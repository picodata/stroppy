/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	llog "github.com/sirupsen/logrus"

	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
)

const dateFormat = "02-01-2006_15_04_05"

//nolint:nonamedreturns // should be fixed in future
func (sh *shell) executeRemotePay() (beginTime, endTime int64, err error) {
	llog.Debugf("DBURL: %s", sh.state.Settings.DatabaseSettings.DBURL)

	payTestCommand := []string{
		stroppyBinaryPath,
		"pay",
		"--dir",
		stroppyHomePath,
		"--run-type", "client",
		"--url", sh.state.Settings.DatabaseSettings.DBURL,
		"--check",
		"--count", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.Count),
		"-r", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.BanRangeMultiplier),
		"-w", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.Workers),
		"--dbtype", sh.state.Settings.DatabaseSettings.DBType,
		"--log-level", sh.state.Settings.LogLevel,
	}

	llog.Tracef("Stroppy remote command '%s'", strings.Join(payTestCommand, " "))

	logFileName := fmt.Sprintf(
		"%v_pay_%v_%v_zipfian_%v_%v.log",
		sh.state.Settings.DatabaseSettings.DBType,
		sh.state.Settings.DatabaseSettings.Count,
		sh.state.Settings.DatabaseSettings.BanRangeMultiplier,
		sh.state.Settings.DatabaseSettings.Zipfian,
		time.Now().Format(dateFormat),
	)

	beginTime, endTime, err = sh.k.ExecuteRemoteCommand(
		stroppy.StroppyClientPodName,
		"",
		payTestCommand,
		logFileName,
		&sh.state,
	)
	if err != nil {
		err = merry.Prepend(err, "failed to execute remote transfer test")
	}

	return
}

// executePay - выполнить тест переводов внутри удаленного пода stroppy
func (sh *shell) executePay(shellState *state.State) error {
	var err error

	if err = sh.readDatabaseConfig("pay"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	var beginTime, endTime int64

	if sh.state.Settings.TestSettings.IsController() {
		if beginTime, endTime, err = sh.executeRemotePay(); err != nil {
			return merry.Prepend(err, "failed to executeRemotePay")
		}
	} else {
		beginTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
		if err = sh.payload.Pay(shellState); err != nil {
			return merry.Prepend(err, "failed to execut local pay")
		}
		endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
	}
	llog.Infof("pay test start time: '%d', end time: '%d'", beginTime, endTime)

	monImagesArchName := fmt.Sprintf(
		"%v_pay_%v_%v_zipfian_%v_%v.tar.gz",
		sh.state.Settings.DatabaseSettings.DBType,
		sh.state.Settings.DatabaseSettings.Count,
		sh.state.Settings.DatabaseSettings.BanRangeMultiplier,
		sh.state.Settings.DatabaseSettings.Zipfian,
		time.Now().Format(dateFormat),
	)

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)

	if err = sh.k.Engine.CollectMonitoringData(
		beginTime,
		endTime,
		sh.k.MonitoringPort.Port,
		monImagesArchName,
		&sh.state,
	); err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pay test")
	}

	return nil
}

// executePop - выполнить загрузку счетов в указанную БД внутри удаленного пода stroppy
func (sh *shell) executePop(shellState *state.State) error {
	var err error

	if err = sh.readDatabaseConfig("pop"); err != nil {
		return merry.Prepend(err, "failed to read config")
	}

	llog.Debugf(
		"Stroppy executed on remote host: %v",
		sh.state.Settings.TestSettings.IsController(),
	)

	var beginTime, endTime int64

	if sh.state.Settings.TestSettings.IsController() {
		if beginTime, endTime, err = sh.executeRemotePop(); err != nil {
			return merry.Prepend(err, "failed to executeRemotePop")
		}
	} else {
		beginTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
		if err = sh.payload.Pop(shellState); err != nil {
			return merry.Prepend(err, "failed to execut local Pop")
		}
		endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000
	}

	llog.Infof("Pop test start time: '%d', end time: '%d'", beginTime, endTime)

	monImagesArchName := fmt.Sprintf(
		"%v_pop_%v_%v_zipfian_%v_%v.tar.gz",
		sh.state.Settings.DatabaseSettings.DBType,
		sh.state.Settings.DatabaseSettings.Count,
		sh.state.Settings.DatabaseSettings.BanRangeMultiplier,
		sh.state.Settings.DatabaseSettings.Zipfian,
		time.Now().Format(dateFormat),
	)

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)

	if err = sh.k.Engine.CollectMonitoringData(
		beginTime,
		endTime,
		sh.k.MonitoringPort.Port,
		monImagesArchName,
		&sh.state,
	); err != nil {
		return merry.Prepend(err, "failed to get monitoring images for pop test")
	}

	return nil
}

//nolint:nonamedreturns // should be fixed in future
func (sh *shell) executeRemotePop() (beginTime, endTime int64, err error) {
	llog.Debugf("DBURL: %s", sh.state.Settings.DatabaseSettings.DBURL)

	popTestCommand := []string{
		stroppyBinaryPath,
		"pop",
		"--dir",
		stroppyHomePath,
		"--run-type", "client",
		"--url", sh.state.Settings.DatabaseSettings.DBURL,
		"--count", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.Count),
		"-r", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.BanRangeMultiplier),
		"-w", fmt.Sprintf("%v", sh.state.Settings.DatabaseSettings.Workers),
		"--dbtype", sh.state.Settings.DatabaseSettings.DBType,
		"--log-level", sh.state.Settings.LogLevel,
	}

	llog.Tracef("Stroppy remote command '%s'", strings.Join(popTestCommand, " "))

	if sh.state.Settings.DatabaseSettings.Sharded {
		popTestCommand = append(popTestCommand, "sharded")
	}

	logFileName := fmt.Sprintf(
		"%v_pop_%v_%v_zipfian_%v_%v.log",
		sh.state.Settings.DatabaseSettings.DBType,
		sh.state.Settings.DatabaseSettings.Count,
		sh.state.Settings.DatabaseSettings.BanRangeMultiplier,
		sh.state.Settings.DatabaseSettings.Zipfian,
		time.Now().Format(dateFormat),
	)

	if beginTime, endTime, err = sh.k.ExecuteRemoteCommand(
		stroppy.StroppyClientPodName,
		"",
		popTestCommand,
		logFileName,
		&sh.state,
	); err != nil {
		return 0, 0, merry.Prepend(err, "failed to execute remote populate test")
	}

	return beginTime, endTime, nil
}

// readDatabaseConfig
// прочитать конфигурационный файл test_config.json
func (sh *shell) readDatabaseConfig(cmdType string) error {
	var (
		err  error
		data []byte
	)

	llog.Debugf(
		"Expected test config file path %s",
		filepath.Join(sh.workingDirectory, testConfDir, configFileName),
	)

	if data, err = os.ReadFile(path.Join(
		sh.state.Settings.WorkingDirectory,
		testConfDir,
		configFileName,
	)); err != nil {
		return errors.Wrap(err, "failed to read config file")
	}

	sh.state.Settings.DatabaseSettings.BanRangeMultiplier = gjson.Parse(string(data)).
		Get("banRangeMultiplier").
		Float()

	switch sh.state.Settings.DatabaseSettings.DBType {
	case cluster.Postgres:
		sh.state.Settings.DatabaseSettings.DBURL = pgDefaultURI
	case cluster.Foundation:
		sh.state.Settings.DatabaseSettings.DBURL = fdbDefultURI
	case cluster.MongoDB:
		sh.state.Settings.DatabaseSettings.DBURL = mongoDefaultURI
	case cluster.Cockroach:
		sh.state.Settings.DatabaseSettings.DBURL = crDefaultURI
	case cluster.Cartridge:
		sh.state.Settings.DatabaseSettings.DBURL = cartDefaultURI
	case cluster.YandexDB:
		sh.state.Settings.DatabaseSettings.DBURL = ydbDefaultURI
	default:
		return errors.Errorf("unknown db type '%s'", sh.state.Settings.DatabaseSettings.DBType)
	}

	switch cmdType {
	case "pop":
		sh.state.Settings.DatabaseSettings.Count = gjson.Parse(string(data)).
			Get("cmd.0").
			Get("pop").
			Get("count").
			Uint()
	case "pay":
		sh.state.Settings.DatabaseSettings.Count = gjson.Parse(string(data)).
			Get("cmd.1").
			Get("pay").
			Get("count").
			Uint()

		sh.state.Settings.DatabaseSettings.Check = gjson.Parse(string(data)).
			Get("cmd.1").
			Get("pay").
			Get("Check").
			Bool()
		sh.state.Settings.DatabaseSettings.Zipfian = gjson.Parse(string(data)).
			Get("cmd.1").
			Get("pay").
			Get("zipfian").
			Bool()
		sh.state.Settings.DatabaseSettings.Oracle = gjson.Parse(string(data)).
			Get("cmd.1").
			Get("pay").
			Get("oracle").
			Bool()
	}

	return nil
}
