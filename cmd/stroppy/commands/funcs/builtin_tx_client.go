package funcs

import (
	"errors"
	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

const maxTxRetries = 10

var maxSleepDuration, _ = time.ParseDuration("1s")

// BuiltinTxTransfer
// This interface describe the interaction between general Pay code and
// some db cluster that is capable of performing ACID transactions.
//
// should satisfy PredictableCluster interface
type BuiltinTxTransfer interface {
	GetClusterType() cluster.DBClusterType
	// provide seed and count of accounts for this cluster.
	FetchSettings() (cluster.ClusterSettings, error)

	// MakeAtomicTransfer performs transfer operation using db's builtin ACID transactions
	// This methods should not return ErrNoRows - if one of accounts does not exist we should simply proceed further
	MakeAtomicTransfer(t *model.Transfer) error

	database.PredictableCluster
}

type ClientBuiltinTx struct {
	cluster BuiltinTxTransfer
	// oracle is optional, because it is to hard to implement
	// for large dbs
	oracle   *database.Oracle
	payStats *PayStats
}

func (c *ClientBuiltinTx) Init(cluster BuiltinTxTransfer, oracle *database.Oracle, payStats *PayStats) {
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
			if errors.Is(err, cluster.ErrTxRollback) {
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
			if errors.Is(err, cluster.ErrInsufficientFunds) {
				atomic.AddUint64(&c.payStats.InsufficientFunds, 1)
				break
			}
			// that means one of accounts was not found
			// and we should proceed to the next transfer
			if errors.Is(err, cluster.ErrNoRows) {
				atomic.AddUint64(&c.payStats.NoSuchAccount, 1)
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
	settings *config.DatabaseSettings,
	nTransfers int,
	zipfian bool,
	dbCluster BuiltinTxTransfer,
	oracle *database.Oracle,
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

	randSource.Init(clusterSettings.Count, clusterSettings.Seed, settings.BanRangeMultiplier)
	for i := 0; i < nTransfers; {
		t := new(model.Transfer)
		t.InitRandomTransfer(&randSource, zipfian)

		cookie := statistics.StatsRequestStart()
		if _, err := client.MakeAtomicTransfer(t); err != nil {
			if IsTransientError(err) {
				llog.Tracef("[%v] Transfer failed: %v", t.Id, err)
			} else {
				llog.Errorf("Got a fatal error %v, ending worker", err)
				return
			}
		} else {
			i++
			statistics.StatsRequestEnd(cookie)
		}
	}
}

// TODO: расширить логику, либо убрать err в выходных параметрах
//nolint:unparam
func payBuiltinTx(settings *config.DatabaseSettings, cluster BuiltinTxTransfer, oracle *database.Oracle) (*PayStats, error) {
	var (
		wg       sync.WaitGroup
		payStats PayStats
	)

	transfersPerWorker := settings.Count / settings.Workers
	remainder := settings.Count - transfersPerWorker*settings.Workers

	// is recovery needed for builtin? Maybe after x retries for Tx
	// TODO: implement recovery

	for i := 0; i < settings.Workers; i++ {
		wg.Add(1)
		nTransfers := transfersPerWorker
		if i < remainder {
			nTransfers++
		}
		go payWorkerBuiltinTx(settings, nTransfers, settings.ZIPFian, cluster, oracle, &payStats, &wg)
	}

	wg.Wait()
	statistics.StatsReportSummary()
	if oracle != nil {
		oracle.FindBrokenAccounts(cluster)
	}

	return &payStats, nil
}
