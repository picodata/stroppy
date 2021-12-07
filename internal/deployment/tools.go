/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

const dateFormat = "02-01-2006_15_04_05"

func (sh *shell) executeRemotePay(settings *config.DatabaseSettings) (beginTime, endTime int64, err error) {
	payTestCommand := []string{
		"/root/stroppy", "pay",
		"--dir", "/root",
		"--run-as-pod",
		"--url", fmt.Sprintf("%v", settings.DBURL),
		"--check",
		"--count", fmt.Sprintf("%v", settings.Count),
		"-r", fmt.Sprintf("%v", settings.BanRangeMultiplier),
		"-w", fmt.Sprintf("%v", settings.Workers),
		"--dbtype", sh.settings.DatabaseSettings.DBType,
	}

	logFileName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.log",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.Zipfian, time.Now().Format(dateFormat))

	beginTime, endTime, err = sh.k.ExecuteRemoteCommand(stroppy.PodName, "",
		payTestCommand, logFileName)
	if err != nil {
		err = merry.Prepend(err, "failed to execute remote transfer test")
	}
	return
}

// executePay - выполнить тест переводов внутри удаленного пода stroppy
func (sh *shell) executePay(_ string) (err error) {
	var settings *config.DatabaseSettings
	if settings, err = sh.readDatabaseConfig("pay"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	var beginTime, endTime int64
	if sh.settings.TestSettings.UseCloudStroppy {
		beginTime, endTime, err = sh.executeRemotePay(settings)
		if err != nil {
			return
		}
	} else {
		beginTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
		if err = sh.payload.Pay(""); err != nil {
			return
		}
		endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
	}

	monImagesArchName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.tar.gz",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.Zipfian, time.Now().Format(dateFormat))

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)
	if err = sh.k.Engine.CollectMonitoringData(beginTime, endTime, sh.k.MonitoringPort.Port, monImagesArchName); err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pay test")
	}

	return
}

func (sh *shell) executeRemotePop(settings *config.DatabaseSettings) (beginTime, endTime int64, err error) {
	popTestCommand := []string{
		"/root/stroppy", "pop",
		"--dir", "/root",
		"--run-as-pod",
		"--url", fmt.Sprintf("%v", settings.DBURL),
		"--count", fmt.Sprintf("%v", settings.Count),
		"-r", fmt.Sprintf("%v", settings.BanRangeMultiplier),
		"-w", fmt.Sprintf("%v", settings.Workers),
		"--dbtype", sh.settings.DatabaseSettings.DBType,
	}

	if settings.Sharded {
		popTestCommand = append(popTestCommand, "sharded")
	}

	logFileName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.log",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.Zipfian, time.Now().Format(dateFormat))

	beginTime, endTime, err = sh.k.ExecuteRemoteCommand(stroppy.PodName, "", popTestCommand, logFileName)
	if err != nil {
		err = merry.Prepend(err, "failed to execute remote populate test")
	}
	return
}

// executePop - выполнить загрузку счетов в указанную БД внутри удаленного пода stroppy
func (sh *shell) executePop(_ string) (err error) {
	var settings *config.DatabaseSettings
	if settings, err = sh.readDatabaseConfig("pop"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	// sh.payload.UpdateSettings(settings)

	var beginTime, endTime int64
	if sh.settings.TestSettings.UseCloudStroppy {
		beginTime, endTime, err = sh.executeRemotePop(settings)
		if err != nil {
			return
		}
	} else {
		beginTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
		if err = sh.payload.Pop(""); err != nil {
			return
		}
		endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
	}

	monImagesArchName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.tar.gz",
		settings.DBType, settings.Count, settings.BanRangeMultiplier,
		settings.Zipfian, time.Now().Format(dateFormat))

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)
	if err = sh.k.Engine.CollectMonitoringData(beginTime, endTime, sh.k.MonitoringPort.Port, monImagesArchName); err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pop test")
	}

	return
}

// readDatabaseConfig
// прочитать конфигурационный файл test_config.json
func (sh *shell) readDatabaseConfig(cmdType string) (settings *config.DatabaseSettings, err error) {
	var data []byte
	configFilePath := filepath.Join(sh.workingDirectory, configFileName)
	if data, err = ioutil.ReadFile(configFilePath); err != nil {
		err = merry.Prepend(err, "failed to read config file")
		return
	}

	settings = config.DatabaseDefaults()
	settings.BanRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.DBType = sh.settings.DatabaseSettings.DBType

	switch sh.settings.DatabaseSettings.DBType {
	case cluster.Postgres:
		settings.DBURL = "postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable"

	case cluster.Foundation:
		settings.DBURL = "fdb.cluster"
	case cluster.MongoDB:
		settings.DBURL = "mongodb://stroppy:stroppy@sample-cluster-name-mongos.default.svc.cluster.local/admin?ssl=false"
	default:
		err = merry.Errorf("unknown db type '%s'", sh.settings.DatabaseSettings.DBType)
		return
	}

	if cmdType == "pop" {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.0").Get("pop").Get("count").Int())
	} else if cmdType == "pay" {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("count").Int())
		settings.Check = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("Check").Bool()
		settings.Zipfian = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("zipfian").Bool()
		settings.Oracle = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("oracle").Bool()
	}

	return
}
