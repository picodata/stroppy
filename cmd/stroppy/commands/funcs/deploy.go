package funcs

import (
	"bufio"
	"io/ioutil"
	"log"
	"os"

	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/engine/postgres"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

const workingDirectory = "benchmark/deploy/"

const configFile = "benchmark/deploy/test_config.json"

var _terraform *terraform.Terraform

// readCommandFromInput - прочитать стандартный ввод и запустить выбранные команды
func readCommandFromInput(portForwardStruct *engine.ClusterTunnel,
	errorExit chan error, successExit chan bool, popChan chan error, payChan chan error) {
	for {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			consoleCmd := sc.Text()
			statistics.StatsInit()
			switch consoleCmd {
			case "quit":
				llog.Println("Exiting...")

				err := portForwardStruct.Command.Process.Kill()
				if err != nil {
					llog.Errorf("failed to kill process port forward %v. \n Repeat...", err.Error())
				}

				err = _terraform.Destroy()
				if err != nil {
					errorExit <- merry.Prepend(err, "failed to destroy terraform")
				} else {
					successExit <- true
				}
			case "postgres pop":
				{
					llog.Println("Starting accounts populating for postgres...")
					err := executePop(consoleCmd, "postgres")
					if err != nil {
						popChan <- err
					} else {
						llog.Println("Populating of accounts in postgres successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "postgres pay":
				{
					llog.Println("Starting transfer tests for postgres...")
					err := executePay(consoleCmd, "postgres")
					if err != nil {
						payChan <- err
					} else {
						llog.Println("Transfers test in postgres successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "fdb pop":
				{
					llog.Println("Starting accounts populating for fdb...")
					err := executePop(consoleCmd, "fdb")
					if err != nil {
						popChan <- err
					} else {
						llog.Println("Populating of accounts in fdb successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "fdb pay":
				{
					llog.Println("Starting transfer tests for fdb...")
					err := executePay(consoleCmd, "fdb")
					if err != nil {
						payChan <- err
					} else {
						llog.Println("Transfers test in fdb successed")
						llog.Println("Waiting enter command:")
					}
				}
			default:
				llog.Infof("You entered: %v. Expected quit \n", consoleCmd)
			}
		}
	}
}

// executePay - выполнить тест переводов
func executePay(cmdType string, databaseType string) error {
	settings, err := readConfig(cmdType, databaseType)
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	sum, err := Check(settings, nil)
	if err != nil {
		llog.Errorf("%v", err)
	}

	llog.Infof("Initial balance: %v", sum)

	if err := Pay(settings); err != nil {
		llog.Errorf("%v", err)
	}

	if settings.Check {
		balance, err := Check(settings, sum)
		if err != nil {
			llog.Errorf("%v", err)
		}
		llog.Infof("Final balance: %v", balance)
	}
	return nil
}

// readConfig
// прочитать конфигурационный файл test_config.json
func readConfig(cmdType string, databaseType string) (*config.DatabaseSettings, error) {
	settings := config.Defaults()
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read config file")
	}

	settings.LogLevel = gjson.Parse(string(data)).Get("log_level").Str
	settings.BanRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.DatabaseType = databaseType
	if databaseType == "postgres" {
		settings.DBURL = "postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable"
	} else if databaseType == "fdb" {
		settings.DBURL = "fdb.cluster"
	}
	if (cmdType == "postgres pop") || (cmdType == "fdb pop") {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.0").Get("pop").Get("count").Int())
	} else if (cmdType == "postgres pay") || (cmdType == "fdb pay") {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("count").Int())
		settings.Check = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("Check").Bool()
		settings.ZIPFian = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("zipfian").Bool()
		settings.Oracle = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("oracle").Bool()
	}

	return settings, nil
}

func Deploy(settings *config.DeploySettings) (err error) {
	llog.Traceln(settings)

	terraformVersion, err := terraform.GetTerraformVersion()
	if err != nil {
		return merry.Prepend(err, "failed to get terraform version")
	}

	_terraform = terraform.CreateTerraform(settings, workingDirectory, workingDirectory, terraformVersion)

	if err = _terraform.Run(); err != nil {
		return merry.Prepend(err, "terraform run failed")
	}

	var addressMap terraform.MapAddresses
	if addressMap, err = _terraform.GetAddressMap(); err != nil {
		return merry.Prepend(err, "failed to get address map")
	}

	privateKeyFile, err := engineSsh.GetPrivateKeyFile(settings.Provider, _terraform.WorkDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get private key for terraform")
	}

	sc, _ := engineSsh.CreateClient(workingDirectory, addressMap.MasterExternalIP, settings.Provider, privateKeyFile)

	k, err := kubernetes.CreateKubernetes(workingDirectory, addressMap, sc, privateKeyFile, settings.Provider)

	var portForward *engine.ClusterTunnel
	var port int
	if portForward, port, err = k.Deploy(); err != nil {
		return merry.Prepend(err, "failed to start kubernetes")
	}
	defer k.Stop()

	if settings.UseChaos {
		chaosMesh := chaos.CreateController(sc, k)
		if err = chaosMesh.Deploy(); err != nil {
			return merry.Prepend(err, "failed to deploy and start chaos")
		}
	}

	if settings.DBType == "postgres" {
		pg := postgres.CreatePostgresCluster(sc, k, addressMap)
		if err = pg.Deploy(); err != nil {
			return merry.Prepend(err, "failed to deploy of postgres")
		}

		statusSet, err := pg.GetStatus()
		if err != nil {
			return merry.Prepend(err, "failed to check deploy of postgres")
		}
		if statusSet.Err != nil {
			return merry.Prepend(err, "deploy of postgres is failed")
		}

		if err = pg.OpenPortForwarding(); err != nil {
			return merry.Prepend(err, "failed to open port-forward channel of postgres")
		}
	}

	log.Printf(
		`Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
	To access Grafana use address localhost:%v.
	To access to kubernetes cluster in cloud use address localhost:%v.
	Enter "quit" to exit stroppy and destroy cluster.
	Enter "postgres pop" to start populating PostgreSQL with accounts.
	Enter "postgres pay" to start transfers test in PostgreSQL.
	Enter "fdb pop" to start populating FoundationDB with accounts.
	Enter "fdb pay" to start transfers test in FoundationDB.
	To use kubectl for access kubernetes cluster in another console 
	execute command for set environment variables KUBECONFIG before using:
	"export KUBECONFIG=$(pwd)/config"`, portForward.LocalPort, port)

	errorExitChan := make(chan error)
	successExitChan := make(chan bool)
	popChan := make(chan error)
	payChan := make(chan error)
	go readCommandFromInput(portForward, errorExitChan, successExitChan, popChan, payChan)
	select {
	case err = <-errorExitChan:
		llog.Errorf("failed to destroy cluster: %v", err)
		return merry.Wrap(err)
	case success := <-successExitChan:
		llog.Infof("destroy cluster %v", success)
		return nil
	}
}
