package main

import (
	"runtime"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"

	"gitlab.com/picodata/benchmark/stroppy/store"
)

// IsTransientError is a wrapper to determine if request was
// terminated due to data inconsistency / logical bug
// or it was just a request / tx timeout etc.
func IsTransientError(err error) bool {
	err = merry.Unwrap(err)

	return err == store.ErrTimeoutExceeded
}

var nClients uint64

type PayStats struct {
	errors             uint64
	no_such_account    uint64
	insufficient_funds uint64
	retries            uint64
	recoveries         uint64
}

func pay(settings *Settings) error {
	llog.Infof("Establishing connection to the cluster")
	var err error
	var cluster interface{}
	switch settings.databaseType {
	case "postgres":
		var closeConns func()
		cluster, closeConns, err = store.NewPostgresCluster(settings.dbURL)
		if err != nil {
			return merry.Wrap(err)
		}
		defer closeConns()
	default:
		return merry.Errorf("unknown database type for setup")
	}

	llog.Infof("Making %d transfers using %d workers on %d cores \n",
		settings.count, settings.workers, runtime.NumCPU())

	var oracle *Oracle
	if settings.oracle {
		predictableCluster, ok := cluster.(PredictableCluster)
		if !ok {
			return merry.Errorf("oracle is not supported for %s cluster", settings.databaseType)
		}
		oracle = new(Oracle)
		oracle.Init(predictableCluster)
	}

	var payStats *PayStats
	if settings.useCustomTx {
		customTxCluster, ok := cluster.(CustomTxTransfer)
		if !ok {
			return merry.Errorf("custom transactions are not supported for %s cluster", settings.databaseType)
		}
		payStats, err = payCustomTx(settings, customTxCluster, oracle)
	} else {
		builtinTxCluster, ok := cluster.(BuiltinTxTranfer)
		if !ok {
			return merry.Errorf("builtin transactions are not supported for %s cluster", settings.databaseType)
		}
		payStats, err = payBuiltinTx(settings, builtinTxCluster, oracle)
	}
	if err != nil {
		llog.Fatal(err)
	}

	llog.Infof("Errors: %v, Retries: %v, Recoveries: %v, Not found: %v, Overdraft: %v\n",
		payStats.errors,
		payStats.retries,
		payStats.recoveries,
		payStats.no_such_account,
		payStats.insufficient_funds)

	return nil
}
