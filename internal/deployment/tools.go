package deployment

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

// executePay - выполнить тест переводов внутри удаленного пода stroppy
func (d *Deployment) executePay(_ string) (err error) {
	var settings *config.DatabaseSettings
	if settings, err = d.readDatabaseConfig("pay"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	payTestCmdTemplate := []string{"./stroppy", "pay", "--url", fmt.Sprintf("%v", settings.DBURL), "--check", "--count", fmt.Sprintf("%v", settings.Count), "-r",
		fmt.Sprintf("%v", settings.BanRangeMultiplier), "-w", fmt.Sprintf("%v", settings.Workers), ">>", "pay.txt"}

	dateFormat := "01-02-2006_15:04:05"
	logFileName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.log", settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	err = d.k.ExecuteRemoteTest(payTestCmdTemplate, logFileName)
	if err != nil {
		return merry.Prepend(err, "failed to execute remote transfer test")
	}

	return
}

// executePop - выполнить загрузку счетов в указанную БД внутри удаленного пода stroppy
func (d *Deployment) executePop(_ string) error {
	settings, err := d.readDatabaseConfig("pop")
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	//d.payload.UpdateSettings(settings)

	popTestCmdTemplate := []string{"./stroppy", "pop", "--url", fmt.Sprintf("%v", settings.DBURL), "--count", fmt.Sprintf("%v", settings.Count), "-r",
		fmt.Sprintf("%v", settings.BanRangeMultiplier), "-w", fmt.Sprintf("%v", settings.Workers), ">>", "pop.txt"}
	dateFormat := "01-02-2006_15:04:05"
	logFileName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.log", settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	err = d.k.ExecuteRemoteTest(popTestCmdTemplate, logFileName)
	if err != nil {
		return merry.Prepend(err, "failed to execute remote populate test")
	}

	return nil
}

// readDatabaseConfig
// прочитать конфигурационный файл test_config.json
func (d *Deployment) readDatabaseConfig(cmdType string) (settings *config.DatabaseSettings, err error) {
	var data []byte
	configFilePath := filepath.Join(d.workingDirectory, configFileName)
	if data, err = ioutil.ReadFile(configFilePath); err != nil {
		err = merry.Prepend(err, "failed to read config file")
		return
	}

	settings = config.DatabaseDefaults()
	settings.LogLevel = gjson.Parse(string(data)).Get("log_level").Str
	settings.BanRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.DBType = d.settings.DatabaseSettings.DBType

	switch d.settings.DatabaseSettings.DBType {
	case cluster.Postgres:
		settings.DBURL = "postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable"

	case cluster.Foundation:
		settings.DBURL = "fdb.cluster"

	default:
		err = merry.Errorf("unknown db type '%s'", d.settings.DatabaseSettings.DBType)
		return
	}

	if cmdType == "pop" {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.0").Get("pop").Get("count").Int())
	} else if cmdType == "pay" {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("count").Int())
		settings.Check = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("Check").Bool()
		settings.ZIPFian = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("zipfian").Bool()
		settings.Oracle = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("oracle").Bool()
	}

	return
}
