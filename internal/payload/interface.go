package payload

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ansel1/merry"
	"github.com/apple/foundationdb/bindings/go/src/fdb"
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
	GetStatistics() error
}

const statJsonFileTemplate = "status_json_%v.json"

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

// GetStatistics - получить статистику работу БД в процессе теста
// Внимание! Пока поддерживается только получение status json для fdb
func (p *BasePayload) GetStatistics() error {

	switch p.config.DBType {
	case "fdb":
		stopChan := make(chan bool)
		errChan := make(chan error)

		if p.config.DBType == "fdb" {
			llog.Debugln("starting of statistic goroutine...")
			go p.getStatistics(stopChan, errChan)
		}

		errorCheck := <-errChan

		if errorCheck != nil {
			return merry.Prepend(errorCheck, "failed to get statistic")
		}

	case "postges":
		llog.Debugln("statictis for postgres not supported")
	}

	return nil
}

func (p *BasePayload) getStatistics(stopChan chan bool, errChan chan error) {
	var once sync.Once
	var resultMap map[string]interface{}
	var jsonResult []byte

	const dateFormat = "02-01-2006_15:04:05"

	statFileName := fmt.Sprintf(statJsonFileTemplate, time.Now().Format(dateFormat))
	llog.Debugln("Opening statistic file...")
	statFile, err := os.OpenFile(statFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		errChan <- merry.Prepend(err, "failed to open statistic file")
	}

	defer statFile.Close()

	llog.Debugln("Opening statistic file: success")

	var FDBPool fdb.Database

	if FDBPool, err = fdb.OpenDatabase(p.config.DBURL); err != nil {
		errChan <- merry.Prepend(err, "failed to open connect to fdb to get statictics")
	}

	// если ошибки нет, то отправляем nil, чтобы продолжить работу
	onceBody := func() {
		errChan <- nil
	}

	for {
		data, err := FDBPool.ReadTransact(func(tx fdb.ReadTransaction) (interface{}, error) {
			status, err := tx.Get(fdb.Key("\xFF\xFF/status/json")).Get()
			if err != nil {
				return nil, err
			}
			return status, nil
		})

		if err != nil {
			errChan <- merry.Prepend(err, "failed to get status json from db")
		}

		result, ok := data.([]byte)
		if !ok {
			errChan <- merry.Errorf("status data type is not supported, value: %v", result)
		}

		if err = json.Unmarshal(result, &resultMap); err != nil {
			errChan <- merry.Prepend(err, "failed to unmarchal status json")
		}

		separateString := fmt.Sprintf("\n %v \n", time.Now().Format(dateFormat))
		if _, err = statFile.Write([]byte(separateString)); err != nil {
			errChan <- merry.Prepend(err, "failed to write separate string to statistic file")
		}

		if jsonResult, err = json.MarshalIndent(resultMap, "", "    "); err != nil {
			errChan <- merry.Prepend(err, "failed to marshal data")
		}

		if _, err = statFile.Write(jsonResult); err != nil {
			errChan <- merry.Prepend(err, "failed to write data to statistic file")
		}

		once.Do(onceBody)

		time.Sleep(30 * time.Second)
	}
}
