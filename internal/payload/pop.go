/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package payload

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
	"github.com/pkg/errors"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/state"
	"gitlab.com/picodata/stroppy/pkg/statistics"
	"go.mongodb.org/mongo-driver/mongo"
)

type ClusterPopulatable interface {
	// BootstrapDB creates correspondig tables and truncates them if they already exists.
	// The general DB model is described here:
	// https://docs.google.com/document/d/10tCrLd56ZkPifSlpRF4LPE0yWAc5W2bxN9b5RUhfKss/edit
	//
	// For now data model for PostgreSQL is copied from lighest, but should be adjusted to correspond
	// to planned workload in the future
	BootstrapDB(count uint64, seed int) error
	FetchSettings() (cluster.Settings, error)

	InsertAccount(acc model.Account) error
}

type PopStats struct {
	errors     uint64
	duplicates uint64
}

func (p *BasePayload) Pop(shellState *state.State) error { //nolint //TODO: refactor
	stats := PopStats{}

	err := p.Cluster.BootstrapDB(p.config.Count, int(p.config.Seed))
	if err != nil {
		return merry.Prepend(err, "cluster bootstrap failed")
	}

	var clusterSettings cluster.Settings
	if clusterSettings, err = p.Cluster.FetchSettings(); err != nil {
		return merry.Prepend(err, "cluster settings fetch failed")
	}

	worker := func(id, nAccounts uint64, wg *sync.WaitGroup) { //nolint
		defer wg.Done()

		var rand fixed_random_source.FixedRandomSource
		rand.Init(clusterSettings.Count, clusterSettings.Seed, p.config.BanRangeMultiplier)

		llog.Tracef("Worker %d inserting %d accounts", id, nAccounts)

		for i := uint64(0); i < nAccounts; { //nolint
			cookie := statistics.StatsRequestStart()
			bic, ban := rand.NewBicAndBan()
			balance := rand.NewStartBalance()
			acc := model.Account{ //nolint
				Bic:     bic,
				Ban:     ban,
				Balance: balance,
				Found:   false,
			}
			// Retry loop
			for {
				insertErr := p.Cluster.InsertAccount(acc)
				if insertErr != nil {
					if errors.Is(insertErr, cluster.ErrDuplicateKey) {
						atomic.AddUint64(&stats.duplicates, 1)
						// Duplicate account means we need to re-generate the values and retry
						bic, ban := rand.NewBicAndBan()
						acc = model.Account{ //nolint
							Bic:     bic,
							Ban:     ban,
							Balance: balance,
							Found:   false,
						}

						continue
					}
					atomic.AddUint64(&stats.errors, 1)
					// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
					// description of fdb.error with code 1009 -  "Request for future version". May be because lagging of storages
					// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
					// description of fdb.error with code 1009 -  "Request for future version". May be because lagging of storages
					// description of mongo.error with code 133 - FailedToSatisfyReadPreference (Could not find host matching read preference { mode: "primary" } for set)
					// description of mongo.error with code 64 - waiting for replication timed out
					//  description of mongo.error with code 11602 - InterruptedDueToReplStateChange
					if errors.Is(insertErr, cluster.ErrTimeoutExceeded) ||
						errors.Is(insertErr, cluster.ErrInternalServerError) ||
						errors.Is(insertErr, fdb.Error{
							Code: 1037,
						}) ||
						errors.Is(insertErr, fdb.Error{
							Code: 1009, //nolint
						}) ||
						errors.Is(insertErr, mongo.CommandError{ //nolint
							Code: 133, //nolint
						}) ||
						errors.Is(insertErr, cluster.ErrTxRollback) ||
						mongo.IsNetworkError(insertErr) ||
						// временная мера до стабилизации mongo
						mongo.IsTimeout(insertErr) ||
						strings.Contains(insertErr.Error(), "connection ") ||
						strings.Contains(insertErr.Error(), "socket ") ||
						errors.Is(insertErr, mongo.WriteConcernError{Code: 64}) || //nolint
						errors.Is(insertErr, mongo.WriteConcernError{Code: 11602}) || //nolint
						errors.Is(insertErr, mongo.WriteError{}) { //nolint
						llog.Errorf("Retrying after request error: %v", insertErr)
						// workaround to finish populate test when account insert gets retryable error
						time.Sleep(time.Millisecond)
						continue
					}

					llog.Fatalf("Fatal error: %+v", insertErr)
				} else {
					break
				}
			}
			// Switch to next account generation
			i++

			statistics.StatsRequestEnd(cookie)
		}
		llog.Tracef("Worker %d done %d accounts", id, nAccounts)
	}

	llog.Infof(
		"Creating %d accounts using %d workers on %d cores \n",
		p.config.Count,
		p.config.Workers,
		p.config.Workers,
	)

	var wg sync.WaitGroup

	accountsPerWorker := p.config.Count / p.config.Workers
	remainder := p.config.Count - accountsPerWorker*p.config.Workers

	chaosCommand := fmt.Sprintf("%s-%s", p.config.DBType, p.chaosParameter)
	if err = p.chaos.ExecuteCommand(chaosCommand, shellState); err != nil {
		return errors.Wrap(err, "failed to execute chaos command")
	}

	for i := uint64(0); i < p.config.Workers; i++ { //nolint
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

	return nil
}
