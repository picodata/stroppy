package main

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/inf.v0"

	"gitlab.com/picodata/benchmark/stroppy/store"
)

type CheckableCluster interface {
	FetchTotal() (*inf.Dec, error)
	CheckBalance() (*inf.Dec, error)
	PersistTotal(total inf.Dec) error
}

func check(settings *Settings, prev *inf.Dec) (*inf.Dec, error) {
	var err error
	var someCluster interface{}
	switch settings.databaseType {
	case "postgres":
		someCluster, err = store.NewPostgresCluster(settings.dbURL)
		if err != nil {
			return nil, merry.Wrap(err)
		}
	default:
		return nil, merry.Errorf("unknown database type for setup")
	}

	cluster, ok := someCluster.(CheckableCluster)
	if !ok {
		return nil, merry.Errorf("builtin transactions are not supported for %s cluster", settings.databaseType)
	}

	// Only persist the balance if it is not persisted yet
	// Only calculate the balance if it's necessary to persist
	// it, or it is necessary for a check (prev != nil)
	var sum *inf.Dec
	persistBalance := false

	if prev == nil {
		sum, err = cluster.FetchTotal()
		if err != nil {
			if err != store.ErrNoRows {
				llog.Fatalf("Failed to fetch the stored total: %v", err)
			}
			sum = nil
			persistBalance = true
		}
	}
	if sum == nil {
		llog.Infof("Calculating the total balance...")
		sum, err = cluster.CheckBalance()
		if err != nil {
			if err != nil {
				llog.Fatalf("Failed to calculate the total: %v", err)
			}
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
		if err := cluster.PersistTotal(*sum); err != nil {
			llog.Fatalf("Failed to persist total balance: error %v, sum: %v", err, sum)
		}
	}

	return sum, nil
}
