package payload

import (
	"sync"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gopkg.in/inf.v0"
)

type Payload interface {
	Pay(string) error
	Pop(string) error
	Check(*inf.Dec) (*inf.Dec, error)
	UpdateSettings(*config.DatabaseSettings)
}

func CreateBasePayload(settings *config.Settings, chaos chaos.Controller) (p Payload, err error) {
	bp := &BasePayload{
		config:         settings.DatabaseSettings,
		chaos:          chaos,
		chaosParameter: settings.ChaosParameter,
	}

	switch bp.config.DBType {
	case cluster.Postgres:
		// для возможности подключиться к БД в кластере с локальной машины
		if bp.config.DBURL == "" {
			bp.config.DBURL = "postgres://stroppy:stroppy@localhost:6432/stroppy?sslmode=disable"
			llog.Infoln("changed DBURL on", bp.config.DBURL)
		}
		bp.Cluster, bp.closeConns, err = cluster.NewPostgresCluster(bp.config.DBURL)
		if err != nil {
			return
		}

	case cluster.Foundation:
		if bp.config.DBURL == "" {
			bp.config.DBURL = "fdb.cluster"
		}
		bp.Cluster, err = cluster.NewFoundationCluster(bp.config.DBURL)
		if err != nil {
			return
		}

	default:
		err = merry.Errorf("unknown database type for setup")
		return
	}

	if bp.config.Oracle {
		predictableCluster, ok := bp.Cluster.(database.PredictableCluster)
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
	llog.Infof("payload object constructed for database '%s', url '%s'",
		bp.config.DBType, bp.config.DBURL)

	p = bp

	return
}

type BasePayload struct {
	Cluster    CustomTxTransfer
	closeConns func()

	config     *config.DatabaseSettings
	configLock sync.Mutex

	chaos          chaos.Controller
	chaosParameter string

	oracle  *database.Oracle
	payFunc func(settings *config.DatabaseSettings, cluster CustomTxTransfer, oracle *database.Oracle) (*PayStats, error)
}

func (p *BasePayload) UpdateSettings(newConfig *config.DatabaseSettings) {
	p.configLock.Lock()
	defer p.configLock.Unlock()

	unpConfig := *newConfig
	p.config = &unpConfig
}
