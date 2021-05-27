package funcs

import (
	"runtime"

	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

// IsTransientError is a wrapper to determine if request was
// terminated due to data inconsistency / logical bug
// or it was just a request / tx timeout etc.
func IsTransientError(err error) bool {
	err = merry.Unwrap(err)

	return err == cluster.ErrTimeoutExceeded
}

var nClients uint64

type PayStats struct {
	errors            uint64
	NoSuchAccount     uint64
	InsufficientFunds uint64
	retries           uint64
	recoveries        uint64
}

func Pay(settings *config.DatabaseSettings) error {
	llog.Infof("Establishing connection to the cluster")

	var (
		err           error
		targetCluster interface{}
	)

	switch settings.DatabaseType {
	case cluster.PostgresType:
		var closeConns func()
		targetCluster, closeConns, err = cluster.NewPostgresCluster(settings.DBURL)
		if err != nil {
			return merry.Wrap(err)
		}
		defer closeConns()
	case cluster.FDBType:
		targetCluster, err = cluster.NewFDBCluster(settings.DBURL)
		if err != nil {
			return merry.Wrap(err)
		}

	default:
		return merry.Errorf("unknown database type for setup")
	}

	llog.Infof("Making %d transfers using %d workers on %d cores \n",
		settings.Count, settings.Workers, runtime.NumCPU())

	var oracle *database.Oracle
	if settings.Oracle {
		predictableCluster, ok := targetCluster.(database.PredictableCluster)
		if !ok {
			return merry.Errorf("oracle is not supported for %s cluster", settings.DatabaseType)
		}
		oracle = new(database.Oracle)
		oracle.Init(predictableCluster)
	}

	var payStats *PayStats
	if settings.UseCustomTx {
		customTxCluster, ok := targetCluster.(CustomTxTransfer)
		if !ok {
			return merry.Errorf("custom transactions are not supported for %s cluster", settings.DatabaseType)
		}
		payStats, err = payCustomTx(settings, customTxCluster, oracle)
	} else {
		builtinTxCluster, ok := targetCluster.(BuiltinTxTransfer)
		if !ok {
			return merry.Errorf("builtin transactions are not supported for %s cluster", settings.DatabaseType)
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
		payStats.NoSuchAccount,
		payStats.InsufficientFunds)

	return nil
}
