package funcs

import (
	"errors"
	cluster2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/cluster"
	config2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/config"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"gitlab.com/picodata/benchmark/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/benchmark/stroppy/internal/model"
	"gitlab.com/picodata/benchmark/stroppy/pkg/statistics"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
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
	FetchSettings() (cluster2.ClusterSettings, error)

	InsertAccount(acc model.Account) error
}

type PopStats struct {
	errors     uint64
	duplicates uint64
}

func Populate(settings *config2.DatabaseSettings) error {
	var (
		err           error
		targetCluster ClusterPopulatable
	)

	switch settings.DatabaseType {
	case cluster2.PostgresType:
		var closeConns func()
		targetCluster, closeConns, err = cluster2.NewPostgresCluster(settings.DBURL)
		if err != nil {
			return merry.Wrap(err)
		}
		defer closeConns()
	case cluster2.FDBType:
		targetCluster, err = cluster2.NewFDBCluster(settings.DBURL)
		if err != nil {
			return merry.Wrap(err)
		}

	default:
		return merry.Errorf("unknown database type for setup")
	}

	stats := PopStats{}

	if err := targetCluster.BootstrapDB(settings.Count, int(settings.Seed)); err != nil {
		return merry.Wrap(err)
	}

	clusterSettings, err := targetCluster.FetchSettings()
	if err != nil {
		return merry.Wrap(err)
	}

	worker := func(id int, n_accounts int, wg *sync.WaitGroup) {
		defer wg.Done()

		var rand fixed_random_source.FixedRandomSource
		rand.Init(clusterSettings.Count, clusterSettings.Seed, settings.BanRangeMultiplier)

		llog.Tracef("Worker %d inserting %d accounts", id, n_accounts)
		for i := 0; i < n_accounts; {
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
				err := targetCluster.InsertAccount(acc)
				if err != nil {
					if errors.Is(err, cluster2.ErrDuplicateKey) {
						atomic.AddUint64(&stats.duplicates, 1)
						break
					}
					atomic.AddUint64(&stats.errors, 1)
					if errors.Is(err, cluster2.ErrTimeoutExceeded) {
						llog.Errorf("Retrying after request error: %v", err)
						time.Sleep(time.Millisecond)
					}
					llog.Fatalf("Fatal error: %+v", err)
				} else {
					i++
					statistics.StatsRequestEnd(cookie)
					break
				}
			}
		}
		llog.Tracef("Worker %d done %d accounts", id, n_accounts)
	}

	llog.Infof("Creating %d accounts using %d workers on %d cores \n",
		settings.Count, settings.Workers,
		runtime.NumCPU())

	var wg sync.WaitGroup

	accountsPerWorker := settings.Count / settings.Workers
	remainder := settings.Count - accountsPerWorker*settings.Workers

	for i := 0; i < settings.Workers; i++ {
		n_accounts := accountsPerWorker
		if i < remainder {
			n_accounts++
		}
		wg.Add(1)
		go worker(i+1, n_accounts, &wg)
	}

	wg.Wait()
	llog.Infof("Done %v accounts, %v errors, %v duplicates",
		settings.Count, stats.errors, stats.duplicates)

	statistics.StatsReportSummary()
	return nil
}
