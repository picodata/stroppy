/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

import (
	"fmt"
	"strings"
	"time"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"

	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/statistics"
)

type Shell interface {
	ReadEvalPrintLoop() error
	RunRemotePayTest() error
	RunRemotePopTest() error
}

func LoadState(settings *config.Settings) (shell Shell, err error) {
	sh := createShell(settings)
	if err = sh.LoadState(); err != nil {
		return
	}

	shell = sh
	return
}

func (sh *shell) LoadState() (err error) {
	if err = sh.prepareTerraform(); err != nil {
		return
	}

	if err = sh.prepareEngine(); err != nil {
		return
	}

	if err = sh.k.OpenPortForwarding(); err != nil {
		return
	}

	sh.chaosMesh = chaos.CreateController(sh.k.Engine, sh.workingDirectory, sh.settings.UseChaos)

	err = sh.prepareDBForTests()
	return
}

// ReadEvalPrintLoop - прочитать стандартный ввод и запустить выбранные команды
func (sh *shell) ReadEvalPrintLoop() (err error) {
out:
	for {
		fmt.Printf("stroppy> ")
	inner:
		for stillAlive, command, params := sh.scanInteractiveCommand(); stillAlive; {
			statistics.StatsInit()

			switch command {
			case "quit", "exit":
				if err = sh.gracefulShutdown(); err != nil {
					return merry.Prepend(err, "Error then stopping stroppy")
				}

				return nil

			case "pop":
				llog.Println("Starting accounts populating")

				if err = sh.executePop(params); err != nil {
					llog.Errorf(
						"'%s' command failed with error '%v' for arguments '%s'",
						command, err, params,
					)

					break out
				} else {
					llog.Println("Populating of accounts in postgres success")
					llog.Println("Enter next command:")

					break inner
				}

			case "pay":
				llog.Println("Starting transfer tests for postgres...")

				if err = sh.executePay(params); err != nil {
					llog.Errorf(
						"'%s' command failed with error '%v' for arguments '%s'",
						command, err, params,
					)

					break out
				} else {
					llog.Println("Transfers test in postgres success")
					llog.Println("Enter next command:")

					break inner
				}

			case "chaos":
				chaosCommand, chaosCommandParams := sh.getCommandAndParams(params)
				switch chaosCommand {
				case "start":
					if err = sh.chaosMesh.ExecuteCommand(chaosCommandParams); err != nil {
						llog.Errorf("chaos command failed: %v", err)
					}

				case "stop":
					sh.chaosMesh.Stop()
				}

			case "\n", "":

			default:
				llog.Warnf("You entered unknown command '%s'. To exit enter quit", command)
			}

			break
		}
	}

	return nil
}

func (sh *shell) RunRemotePayTest() (err error) {
	settings := sh.settings.DatabaseSettings

	var beginTime, endTime int64
	if beginTime, endTime, err = sh.executeRemotePay(settings); err != nil {
		return
	}

	monImagesArchName := fmt.Sprintf("%v_pay_%v_%v_zipfian_%v_%v.tar.gz",
		settings.DBType,
		settings.Count,
		settings.BanRangeMultiplier,
		settings.Zipfian,
		time.Now().Format(dateFormat))

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)
	if err = sh.k.Engine.CollectMonitoringData(beginTime, endTime, sh.k.MonitoringPort.Port, monImagesArchName); err != nil {
		err = merry.Prepend(err, "failed to get monitoring images for pop test")
	}

	return
}

func (sh *shell) RunRemotePopTest() (err error) {
	settings := sh.settings.DatabaseSettings

	var beginTime, endTime int64
	if beginTime, endTime, err = sh.executeRemotePop(settings); err != nil {
		return
	}

	monImagesArchName := fmt.Sprintf("%v_pop_%v_%v_zipfian_%v_%v.tar.gz",
		settings.DBType,
		settings.Count,
		settings.BanRangeMultiplier,
		settings.Zipfian,
		time.Now().Format(dateFormat))

	// таймаут, чтобы не получать пустое место на графиках
	time.Sleep(20 * time.Second)
	if err = sh.k.Engine.CollectMonitoringData(beginTime, endTime, sh.k.MonitoringPort.Port, monImagesArchName); err != nil {
		err = merry.Prepend(err, "failed to get monitoring images for pop test")
	}

	return
}

// --- tool functions -------

func (sh *shell) scanInteractiveCommand() (stillAlive bool, command string, tail string) {
	stillAlive = sh.stdinScanner.Scan()
	text := sh.stdinScanner.Text()

	command, tail = sh.getCommandAndParams(text)
	return
}

func (sh *shell) getCommandAndParams(text string) (command string, params string) {
	cmdArr := strings.SplitN(text, " ", 1)
	cmdAttrLen := len(cmdArr)
	if cmdAttrLen < 1 {
		return
	}
	command = cmdArr[0]

	if cmdAttrLen > 1 {
		params = cmdArr[1]
	}
	return
}
