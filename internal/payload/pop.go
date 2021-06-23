package payload

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/fixed_random_source"
	"gitlab.com/picodata/stroppy/internal/model"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/statistics"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
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
	FetchSettings() (cluster.ClusterSettings, error)

	InsertAccount(acc model.Account) error
}

type PopStats struct {
	errors     uint64
	duplicates uint64
}

func (p *BasePayload) Pop(_ string) (err error) {
	stats := PopStats{}

	if err = p.cluster.BootstrapDB(p.config.Count, int(p.config.Seed)); err != nil {
		return merry.Prepend(err, "cluster bootstrap failed")
	}

	var clusterSettings cluster.ClusterSettings
	if clusterSettings, err = p.cluster.FetchSettings(); err != nil {
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
				err := p.cluster.InsertAccount(acc)
				if err != nil {
					if errors.Is(err, cluster.ErrDuplicateKey) {
						atomic.AddUint64(&stats.duplicates, 1)
						break
					}
					atomic.AddUint64(&stats.errors, 1)
					// description of fdb.error with code 1037 -  "Storage process does not have recent mutations"
					if errors.Is(err, cluster.ErrTimeoutExceeded) || errors.Is(err, fdb.Error{
						Code: 1037,
					}) {
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

	statistics.StatsReportSummary()
	return
}
