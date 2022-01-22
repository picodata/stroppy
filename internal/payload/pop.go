/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/statistics"
	"go.mongodb.org/mongo-driver/mongo"
	"gopkg.in/inf.v0"
)

type ClusterPopulatable interface {
	// BootstrapDB creates correspondig tables and truncates them if they already exists.
	// The general DB model is described here:
	// https://docs.google.com/document/d/10tCrLd56ZkPifSlpRF4LPE0yWAc5W2bxN9b5RUhfKss/edit
	//
	// For now data model for PostgreSQL is copied from lighest, but should be adjusted to correspond
	// to planned workload in the future
	BootstrapDB(count int, seed int) error
	FetchSettings() (cluster.Settings, error)

	InsertAccount(acc model.Account) error
}

type PopStats struct {
	errors     uint64
	duplicates uint64
}

func (p *BasePayload) Pop(_ string) (err error) {
	stats := PopStats{}

	if err = p.Cluster.BootstrapDB(p.config.Count, int(p.config.Seed)); err != nil {
		return merry.Prepend(err, "cluster bootstrap failed")
	}

	var clusterSettings cluster.Settings
	if clusterSettings, err = p.Cluster.FetchSettings(); err != nil {
		return merry.Prepend(err, "cluster settings fetch failed")
	}

	worker := func(id, nAccounts int, wg *sync.WaitGroup) {
		defer wg.Done()

		var rand fixed_random_source.FixedRandomSource
		rand.Init(clusterSettings.Count, clusterSettings.Seed, p.config.BanRangeMultiplier)

		llog.Tracef("Worker %d inserting %d accounts", id, nAccounts)
		for i := 0; i < nAccounts; {
			cookie := statistics.StatsRequestStart()
			bic, ban := rand.NewBicAndBan()
			balance := rand.NewStartBalance()
			acc := model.Account{
				Bic:           bic,
				Ban:           ban,
				Balance:       balance,
				PendingAmount: &inf.Dec{},
				Found:         false,
			}

			llog.Tracef("Inserting account %v:%v - %v", bic, ban, balance)
			for {
				err := p.Cluster.InsertAccount(acc)
				if err != nil {
					if errors.Is(err, cluster.ErrDuplicateKey) {
						atomic.AddUint64(&stats.duplicates, 1)
						break
					}
					atomic.AddUint64(&stats.errors, 1)
					// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
					// description of fdb.error with code 1009 -  "Request for future version". May be because lagging of storages
					// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
					// description of fdb.error with code 1009 -  "Request for future version". May be because lagging of storages
					// description of mongo.error with code 133 - FailedToSatisfyReadPreference (Could not find host matching read preference { mode: "primary" } for set)
					// description of mongo.error with code 64 - waiting for replication timed out
					//  description of mongo.error with code 11602 - InterruptedDueToReplStateChange
					if errors.Is(err, cluster.ErrTimeoutExceeded) || errors.Is(err, cluster.ErrInternalServerError) ||
						errors.Is(err, fdb.Error{
							Code: 1037,
						}) || errors.Is(err, fdb.Error{
						Code: 1009,
					}) || errors.Is(err, mongo.CommandError{
						Code: 133,
						// https://gitlab.com/picodata/openway/stroppy/-/issues/57
					}) || errors.Is(err, cluster.ErrTxRollback) || mongo.IsNetworkError(err) ||
						// временная мера до стабилизации mongo
						mongo.IsTimeout(err) || strings.Contains(err.Error(), "connection ") || strings.Contains(err.Error(), "socket ") ||
						errors.Is(err, mongo.WriteConcernError{Code: 64}) || errors.Is(err, mongo.WriteConcernError{Code: 11602}) ||
						errors.Is(err, mongo.WriteError{}) {
						llog.Errorf("Retrying after request error: %v", err)
						// workaround to finish populate test when account insert gets retryable error
						time.Sleep(time.Millisecond)
						continue
					}
					llog.Fatalf("Fatal error: %+v", err)
				} else {
					i++
					statistics.StatsRequestEnd(cookie)
					break
				}
			}
		}
		llog.Tracef("Worker %d done %d accounts", id, nAccounts)
	}

	llog.Infof("Creating %d accounts using %d workers on %d cores \n",
		p.config.Count, p.config.Workers,
		runtime.NumCPU())

	var wg sync.WaitGroup

	accountsPerWorker := p.config.Count / p.config.Workers
	remainder := p.config.Count - accountsPerWorker*p.config.Workers

	chaosCommand := fmt.Sprintf("%s-%s", p.config.DBType, p.chaosParameter)
	if err = p.chaos.ExecuteCommand(chaosCommand); err != nil {
		llog.Errorf("failed to execute chaos command: %v", err)
	}

	for i := 0; i < p.config.Workers; i++ {
		nAccounts := accountsPerWorker
		if i < remainder {
			nAccounts++
		}
		wg.Add(1)
		go worker(i+1, nAccounts, &wg)
	}

	wg.Wait()
	llog.Infof("Done %v accounts, %v errors, %v duplicates",
		p.config.Count, stats.errors, stats.duplicates)

	p.chaos.Stop()
	statistics.StatsReportSummary()
	return
}
