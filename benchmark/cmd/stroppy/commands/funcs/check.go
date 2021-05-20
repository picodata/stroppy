package funcs

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	cluster2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/cluster"
	config2 "gitlab.com/picodata/benchmark/stroppy/pkg/database/config"
	"gopkg.in/inf.v0"
)

type CheckableCluster interface {
	FetchTotal() (*inf.Dec, error)
	CheckBalance() (*inf.Dec, error)
	PersistTotal(total inf.Dec) error
}

func Check(settings *config2.DatabaseSettings, prev *inf.Dec) (*inf.Dec, error) {
	var (
		err         error
		someCluster interface{}
	)

	switch settings.DatabaseType {
	case cluster2.PostgresType:
		var closeConns func()
		someCluster, closeConns, err = cluster2.NewPostgresCluster(settings.DBURL)
		if err != nil {
			return nil, merry.Wrap(err)
		}
		defer closeConns()
	case cluster2.FDBType:
		someCluster, err = cluster2.NewFDBCluster(settings.DBURL)
		if err != nil {
			return nil, merry.Wrap(err)
		}
	default:
		return nil, merry.Errorf("unknown database type for setup")
	}

	targetCluster, ok := someCluster.(CheckableCluster)
	if !ok {
		return nil, merry.Errorf("builtin transactions are not supported for %s cluster",
			settings.DatabaseType)
	}

	// Only persist the balance if it is not persisted yet
	// Only calculate the balance if it's necessary to persist
	// it, or it is necessary for a Check (prev != nil)
	var sum *inf.Dec
	persistBalance := false

	if prev == nil {
		sum, err = targetCluster.FetchTotal()
		if err != nil {
			if err != cluster2.ErrNoRows {
				llog.Fatalf("Failed to fetch the stored total: %v", err)
			}
			sum = nil
			persistBalance = true
		}
	}

	if sum == nil {
		llog.Infof("Calculating the total balance...")
		if sum, err = targetCluster.CheckBalance(); err != nil {
			llog.Fatalf("Failed to calculate the total: %v", err)
		}
	}

	if prev != nil {
		if prev.Cmp(sum) != 0 {
			llog.Fatalf("Check balance mismatch:\nbefore: %v\nafter:  %v", prev, sum)
		}
	}

	if persistBalance {
		// Do not overwrite the total balance if it is already persisted.
		llog.Infof("Persisting the total balance...")
		if err := targetCluster.PersistTotal(*sum); err != nil {
			llog.Fatalf("Failed to persist total balance: error %v, sum: %v", err, sum)
		}
	}

	return sum, nil
}
