package main

import (
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/benchmark/stroppy/fixed_random_source"
	"gitlab.com/picodata/benchmark/stroppy/model"
	"gitlab.com/picodata/benchmark/stroppy/store"
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
	FetchSettings() (store.ClusterSettings, error)

	InsertAccount(acc model.Account) error
}

type PopStats struct {
	errors     uint64
	duplicates uint64
}

func populate(settings *DatabaseSettings) error {
	var cluster ClusterPopulatable
	var err error
	switch settings.databaseType {
	case store.PostgresType:
		var closeConns func()
		cluster, closeConns, err = store.NewPostgresCluster(settings.dbURL)
		if err != nil {
			return merry.Wrap(err)
		}
		defer closeConns()
	case store.FDBType:
		cluster, err = store.NewFDBCluster(settings.dbURL)
		if err != nil {
			return merry.Wrap(err)
		}

	default:
		return merry.Errorf("unknown database type for setup")
	}

	stats := PopStats{}

	if err := cluster.BootstrapDB(settings.count, int(settings.seed)); err != nil {
		return merry.Wrap(err)
	}

	clusterSettings, err := cluster.FetchSettings()
	if err != nil {
		return merry.Wrap(err)
	}

	worker := func(id int, n_accounts int, wg *sync.WaitGroup) {
		defer wg.Done()

		var rand fixed_random_source.FixedRandomSource
		rand.Init(clusterSettings.Count, clusterSettings.Seed, settings.banRangeMultiplier)

		llog.Tracef("Worker %d inserting %d accounts", id, n_accounts)
		for i := 0; i < n_accounts; {
			cookie := StatsRequestStart()
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
				err := cluster.InsertAccount(acc)
				if err != nil {
					if errors.Is(err, store.ErrDuplicateKey) {
						atomic.AddUint64(&stats.duplicates, 1)
						break
					}
					atomic.AddUint64(&stats.errors, 1)
					if errors.Is(err, store.ErrTimeoutExceeded) {
						llog.Errorf("Retrying after request error: %v", err)
						time.Sleep(time.Millisecond)
					}
					llog.Fatalf("Fatal error: %+v", err)
				} else {
					i++
					StatsRequestEnd(cookie)
					break
				}
			}
		}
		llog.Tracef("Worker %d done %d accounts", id, n_accounts)
	}

	llog.Infof("Creating %d accounts using %d workers on %d cores \n",
		settings.count, settings.workers,
		runtime.NumCPU())

	var wg sync.WaitGroup

	accounts_per_worker := settings.count / settings.workers
	remainder := settings.count - accounts_per_worker*settings.workers

	for i := 0; i < settings.workers; i++ {
		n_accounts := accounts_per_worker
		if i < remainder {
			n_accounts++
		}
		wg.Add(1)
		go worker(i+1, n_accounts, &wg)
	}

	wg.Wait()
	llog.Infof("Done %v accounts, %v errors, %v duplicates",
		settings.count, stats.errors, stats.duplicates)

	StatsReportSummary()
	return nil
}
