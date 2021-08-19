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

	sh.chaosMesh = chaos.CreateController(sh.k, sh.workingDirectory, sh.settings.UseChaos)

	err = sh.preparePayload()
	return
}

// ReadEvalPrintLoop - прочитать стандартный ввод и запустить выбранные команды
func (sh *shell) ReadEvalPrintLoop() (err error) {
	for {
		fmt.Printf("stroppy> ")
		for stillAlive, command, params := sh.scanInteractiveCommand(); stillAlive; {
			statistics.StatsInit()

			switch command {
			case "quit", "exit":
				err = sh.gracefulShutdown()
				return

			case "pop":
				llog.Println("Starting accounts populating")

				if err = sh.executePop(params); err != nil {
					llog.Errorf("'%s' command failed with error '%v' for arguments '%s'",
						command, err, params)
					break
				} else {
					llog.Println("Populating of accounts in postgres success")
					llog.Println("Enter next command:")
					break
				}

			case "pay":
				llog.Println("Starting transfer tests for postgres...")

				if err = sh.executePay(params); err != nil {
					llog.Errorf("'%s' command failed with error '%v' for arguments '%s'",
						command, err, params)
					break
				} else {
					llog.Println("Transfers test in postgres success")
					llog.Println("Enter next command:")
					break
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
	if err = sh.k.ExecuteGettingMonImages(beginTime, endTime, monImagesArchName); err != nil {
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
	if err = sh.k.ExecuteGettingMonImages(beginTime, endTime, monImagesArchName); err != nil {
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
