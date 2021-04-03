package main

import (
	"errors"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/benchmark/stroppy/fixed_random_source"
	"gitlab.com/picodata/benchmark/stroppy/model"
	"gitlab.com/picodata/benchmark/stroppy/store"
)

const maxTxRetries = 10

var maxSleepDuration, _ = time.ParseDuration("1s")

// This interface describe the interaction between general Pay code and
// some db cluster that is capable of performing ACID transactions.
//
// should satisfy PredictableCluster interface
type BuiltinTxTranfer interface {
	GetClusterType() store.DBClusterType
	// provide seed and count of accounts for this cluster.
	FetchSettings() (store.ClusterSettings, error)

	// MakeAtomicTransfer performs transfer operation using db's builtin ACID transactions
	// This methods should not return ErrNoRows - if one of accounts does not exist we should simply proceed further
	MakeAtomicTransfer(t *model.Transfer) error

	PredictableCluster
}

type ClientBuiltinTx struct {
	cluster BuiltinTxTranfer
	// oracle is optional, because it is to hard to implement
	// for large dbs
	oracle   *Oracle
	payStats *PayStats
}

func (c *ClientBuiltinTx) Init(cluster BuiltinTxTranfer, oracle *Oracle, payStats *PayStats) {
	c.cluster = cluster
	c.oracle = oracle
	c.payStats = payStats
}

//nolint:gosec
func (c *ClientBuiltinTx) MakeAtomicTransfer(t *model.Transfer) (bool, error) {
	sleepDuration := time.Millisecond*time.Duration(rand.Intn(10)) + time.Millisecond
	applied := false
	for i := 0; i < maxTxRetries; i++ {
		if err := c.cluster.MakeAtomicTransfer(t); err != nil {
			if errors.Is(err, store.ErrTxRollback) {
				atomic.AddUint64(&c.payStats.retries, 1)

				llog.Tracef("[%v] Retrying transfer after sleeping %v",
					t.Id, sleepDuration)

				time.Sleep(sleepDuration)
				sleepDuration = sleepDuration * 2
				if sleepDuration > maxSleepDuration {
					sleepDuration = maxSleepDuration
				}

				continue
			}
			if errors.Is(err, store.ErrInsufficientFunds) {
				atomic.AddUint64(&c.payStats.insufficient_funds, 1)
				break
			}
			// that means one of accounts was not found
			// and we should proceed to the next transfer
			if errors.Is(err, store.ErrNoRows) {
				atomic.AddUint64(&c.payStats.no_such_account, 1)
				break
			}
			atomic.AddUint64(&c.payStats.errors, 1)
			return applied, merry.Prepend(err, "failed to make a transactional transfer")
		}
		applied = true
		break
	}

	return applied, nil
}

func payWorkerBuiltinTx(
	settings DatabaseSettings,
	n_transfers int,
	zipfian bool,
	dbCluster BuiltinTxTranfer,
	oracle *Oracle,
	payStats *PayStats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	var client ClientBuiltinTx
	var randSource fixed_random_source.FixedRandomSource
	client.Init(dbCluster, oracle, payStats)
	clusterSettings, err := dbCluster.FetchSettings()
	if err != nil {
		llog.Fatalf("Got a fatal error fetching cluster settings: %v", err)
	}

	randSource.Init(clusterSettings.Count, clusterSettings.Seed, settings.banRangeMultiplier)
	for i := 0; i < n_transfers; {
		t := new(model.Transfer)
		t.InitRandomTransfer(&randSource, zipfian)

		cookie := StatsRequestStart()
		if _, err := client.MakeAtomicTransfer(t); err != nil {
			if IsTransientError(err) {
				llog.Tracef("[%v] Transfer failed: %v", t.Id, err)
			} else {
				llog.Errorf("Got a fatal error %v, ending worker", err)
				return
			}
		} else {
			i++
			StatsRequestEnd(cookie)
		}
	}
}

// TO DO: расширить логику, либо убрать err в выходных параметрах
//nolint:unparam
func payBuiltinTx(settings *DatabaseSettings, cluster BuiltinTxTranfer, oracle *Oracle) (*PayStats, error) {
	var wg sync.WaitGroup
	var payStats PayStats

	transfers_per_worker := settings.count / settings.workers
	remainder := settings.count - transfers_per_worker*settings.workers

	// is recovery needed for builtin? Maybe after x retries for Tx
	// TODO: implement recovery

	for i := 0; i < settings.workers; i++ {
		wg.Add(1)
		n_transfers := transfers_per_worker
		if i < remainder {
			n_transfers++
		}
		go payWorkerBuiltinTx(*settings, n_transfers, settings.zipfian, cluster, oracle, &payStats, &wg)
	}

	wg.Wait()
	StatsReportSummary()
	if oracle != nil {
		oracle.FindBrokenAccounts(cluster)
	}

	return &payStats, nil
}
