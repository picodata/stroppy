package payload

import (
	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gopkg.in/inf.v0"
)

type Payload interface {
	Pay(*config.DatabaseSettings, string) error
	Pop(*config.DatabaseSettings, string) error
	Check(*inf.Dec) (*inf.Dec, error)
}

func CreateBasePayload(dbConfig *config.DatabaseSettings) (p Payload, err error) {
	bp := &BasePayload{
		config: dbConfig,
	}

	switch dbConfig.DBType {
	case cluster.PostgresType:
		var closeConns func()
		bp.cluster, closeConns, err = cluster.NewPostgresCluster(dbConfig.DBURL)
		if err != nil {
			return
		}
		defer closeConns()
	case cluster.FDBType:
		bp.cluster, err = cluster.NewFDBCluster(dbConfig.DBURL)
		if err != nil {
			return
		}

	default:
		err = merry.Errorf("unknown database type for setup")
		return
	}

	if bp.config.Oracle {
		predictableCluster, ok := bp.cluster.(database.PredictableCluster)
		if !ok {
			err = merry.Errorf("oracle is not supported for %s cluster", bp.config.DBType)
			return
		}
		bp.oracle = new(database.Oracle)
		bp.oracle.Init(predictableCluster)
	}

	if bp.config.UseCustomTx {
		bp.payFunc = payCustomTx
	} else {
		bp.payFunc = payBuiltinTx
	}

	p = bp
	return
}

type BasePayload struct {
	cluster CustomTxTransfer
	config  *config.DatabaseSettings

	oracle  *database.Oracle
	payFunc func(settings *config.DatabaseSettings, cluster CustomTxTransfer, oracle *database.Oracle) (*PayStats, error)
}
