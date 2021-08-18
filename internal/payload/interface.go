package payload

import (
	"sync"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine/db"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gopkg.in/inf.v0"
)

type Payload interface {
	Pay(string) error
	Pop(string) error
	Check(*inf.Dec) (*inf.Dec, error)
	UpdateSettings(*config.DatabaseSettings)
	StartStatisticsCollect(statInterval time.Duration) error
	Connect() error
}

func CreatePayload(cluster db.Cluster, settings *config.Settings, chaos chaos.Controller) (p Payload, err error) {
	bp := &BasePayload{
		cluster:        cluster,
		config:         settings.DatabaseSettings,
		chaos:          chaos,
		chaosParameter: settings.ChaosParameter,
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
	// \todo: Имеем две сущности описывающие кластер базы данных - произвести рефакторинг
	cluster db.Cluster
	Cluster CustomTxTransfer

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

func (p *BasePayload) StartStatisticsCollect(statInterval time.Duration) (err error) {
	if err = p.Cluster.StartStatisticsCollect(); err != nil {
		return merry.Errorf("failed to get statistic for %v cluster: %v", p.config.DBType, err)
	}

	return
}

func (p *BasePayload) Connect() (err error) {
	// \todo: необходим большой рефакторинг
	var c interface{}
	if c, err = p.cluster.Connect(); err != nil {
		return
	}

	p.Cluster = c.(CustomTxTransfer)
	return
}
