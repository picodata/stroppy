package deployment

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

// executePay - выполнить тест переводов внутри удаленного пода stroppy
func (d *Deployment) executePay(_ string) (err error) {
	var settings *config.DatabaseSettings
	if settings, err = d.readDatabaseConfig("pay"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	payTestCommand := []string{
		"./root/stroppy", "pay",
		"--url", fmt.Sprintf("%v", settings.DBURL),
		"--check",
		"--count", fmt.Sprintf("%v", settings.Count),
		"-r", fmt.Sprintf("%v", settings.BanRangeMultiplier),
		"-w", fmt.Sprintf("%v", settings.Workers),
		"--kube-master-addr", d.settings.TestSettings.KubernetesMasterAddress,
	}

	dateFormat := "01-02-2006_15:04:05"
	logFileName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.log",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	beginTime, endTime, err = d.k.ExecuteRemoteTest(payTestCommand, logFileName)
	if err != nil {
		return merry.Prepend(err, "failed to execute remote transfer test")
	}

	monImagesArchName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.tar.gz", settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	//таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)
	if err = d.k.ExecuteGettingMonImages(beginTime, endTime, monImagesArchName); err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pay test")
	}

	return nil
}

// executePop - выполнить загрузку счетов в указанную БД внутри удаленного пода stroppy
func (d *Deployment) executePop(_ string) error {
	settings, err := d.readDatabaseConfig("pop")
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	// d.payload.UpdateSettings(settings)

	popTestCommand := []string{
		"./root/stroppy", "pop",
		"--url", fmt.Sprintf("%v", settings.DBURL),
		"--count", fmt.Sprintf("%v", settings.Count),
		"-r", fmt.Sprintf("%v", settings.BanRangeMultiplier),
		"--kube-master-addr", d.settings.TestSettings.KubernetesMasterAddress,
		"-w", fmt.Sprintf("%v", settings.Workers), ">>", "pop.txt",
	}
	dateFormat := "01-02-2006_15_04_05"
	logFileName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.log",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	beginTime, endTime, err = d.k.ExecuteRemoteTest(popTestCommand, logFileName)
	if err != nil {
		return merry.Prepend(err, "failed to execute remote populate test")
	}

	monImagesArchName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.tar.gz", settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.ZIPFian, time.Now().Format(dateFormat))

	err = d.k.ExecuteGettingMonImages(beginTime, endTime, monImagesArchName)
	if err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pop test")
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
