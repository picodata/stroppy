package deployment

import (
	"io/ioutil"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gopkg.in/inf.v0"
)

// executePay - выполнить тест переводов
func (d *Deployment) executePay(_ string) (err error) {
	var settings *config.DatabaseSettings
	if settings, err = d.readDatabaseConfig("pay"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	var sum *inf.Dec
	if sum, err = d.payload.Check(nil); err != nil {
		llog.Errorf("%v", err)
	}
	llog.Infof("Initial balance: %v", sum)

	if err := d.payload.Pay(""); err != nil {
		llog.Errorf("%v", err)
	}

	if settings.Check {
		balance, err := d.payload.Check(sum)
		if err != nil {
			llog.Errorf("%v", err)
		}
		llog.Infof("Final balance: %v", balance)
	}
	return
}

// executePop - выполнить загрузку счетов в указанную БД
func (d *Deployment) executePop(_ string) error {
	settings, err := d.readDatabaseConfig("pop")
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	d.payload.UpdateSettings(settings)

	if err := d.payload.Pop(""); err != nil {
		llog.Errorf("%v", err)
	}

	var balance *inf.Dec
	if balance, err = d.payload.Check(nil); err != nil {
		llog.Errorf("%v", err)
	}

	llog.Infof("Total balance: %v", balance)
	return nil
}

// readDatabaseConfig
// прочитать конфигурационный файл test_config.json
func (d *Deployment) readDatabaseConfig(cmdType string) (settings *config.DatabaseSettings, err error) {
	var data []byte
	if data, err = ioutil.ReadFile(configFileName); err != nil {
		err = merry.Prepend(err, "failed to read config file")
		return
	}

	settings = config.DatabaseDefaults()
	settings.LogLevel = gjson.Parse(string(data)).Get("log_level").Str
	settings.BanRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.DBType = d.settings.DatabaseSettings.DBType

	switch d.settings.DatabaseSettings.DBType {
	case "postgres":
		settings.DBURL = "postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable"

	case "fdb":
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
