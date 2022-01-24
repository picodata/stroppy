/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	"errors"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/inf.v0"

	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"

	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	llog "github.com/sirupsen/logrus"
)

const maxTxRetries = 1000

var maxSleepDuration, _ = time.ParseDuration("1s")

type CheckableCluster interface {
	FetchTotal() (*inf.Dec, error)
	CheckBalance() (*inf.Dec, error)
	PersistTotal(total inf.Dec) error
}

// BasicTxTransfer
// This interface describe the interaction between general Pay code and
// some db cluster that is capable of performing ACID transactions.
//
// should satisfy PredictableCluster interface
type BasicTxTransfer interface {
	GetClusterType() cluster.DBClusterType

	// provide seed and count of accounts for this cluster.
	FetchSettings() (cluster.Settings, error)

	// MakeAtomicTransfer performs transfer operation using db's builtin ACID transactions
	// This methods should not return ErrNoRows - if one of accounts does not exist we should simply proceed further
	MakeAtomicTransfer(t *model.Transfer) error

	database.PredictableCluster
}

type ClientBasicTx struct {
	cluster BasicTxTransfer
	// oracle is optional, because it is to hard to implement
	// for large dbs
	oracle   *database.Oracle
	payStats *PayStats
}

func (c *ClientBasicTx) Init(cluster BasicTxTransfer, oracle *database.Oracle, payStats *PayStats) {
	c.cluster = cluster
	c.oracle = oracle
	c.payStats = payStats
}

func (c *ClientBasicTx) MakeAtomicTransfer(t *model.Transfer) (bool, error) {
	sleepDuration := time.Millisecond*time.Duration(rand.Intn(10)) + time.Millisecond
	applied := false

	for i := 0; i < maxTxRetries; i++ {
		if err := c.cluster.MakeAtomicTransfer(t); err != nil {
			// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
			// description of fdb.error with code 1009 -  "Request for future version". May be because lagging of storages
			// description of mongo.error with code 133 - FailedToSatisfyReadPreference (Could not find host matching read preference { mode: "primary" } for set)
			// description of mongo.error with code 64 - waiting for replication timed out
			//  description of mongo.error with code 11602 - InterruptedDueToReplStateChange
			if errors.Is(err, cluster.ErrTimeoutExceeded) || errors.Is(err, fdb.Error{
				Code: 1037,
			}) || errors.Is(err, fdb.Error{
				Code: 1009,
			}) || errors.Is(err, fdb.Error{
				Code: 1007,
			}) || errors.Is(err, cluster.ErrCockroachTxClosed) || errors.Is(err, cluster.ErrCockroachUnexpectedEOF) || errors.Is(err, mongo.CommandError{
				Code: 133,
				// https://gitlab.com/picodata/openway/stroppy/-/issues/57
			}) || errors.Is(err, cluster.ErrTxRollback) || mongo.IsNetworkError(err) ||
				// временная мера до стабилизации mongo
				mongo.IsTimeout(err) || strings.Contains(err.Error(), "connection") || strings.Contains(err.Error(), "socket") ||
				errors.Is(err, mongo.WriteConcernError{Code: 64}) || errors.Is(err, mongo.WriteConcernError{Code: 11602}) {
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
	dbCluster CustomTxTransfer,
	oracle *database.Oracle,
	payStats *PayStats,
	wg *sync.WaitGroup,
) {
	defer wg.Done()

	var client ClientBasicTx
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
func payBuiltinTx(settings *config.DatabaseSettings, cluster CustomTxTransfer, oracle *database.Oracle) (*PayStats, error) {
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
		go payWorkerBuiltinTx(settings, nTransfers, settings.Zipfian, cluster, oracle, &payStats, &wg)
	}

	wg.Wait()
	statistics.StatsReportSummary()
	if oracle != nil {
		oracle.FindBrokenAccounts(cluster)
	}

	return &payStats, nil
}
