package deployment

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func CreateDeployment(config *config.Settings) (d *Deployment) {
	d = &Deployment{
		settings:         config,
		stdinScanner:     bufio.NewScanner(os.Stdin),
		workingDirectory: config.WorkingDirectory,
	}
	return
}

type Deployment struct {
	tf *terraform.Terraform
	sc engineSsh.Client
	k  *kubernetes.Kubernetes

	settings  *config.Settings
	chaosMesh chaos.Controller
	payload   payload.Payload

	stdinScanner *bufio.Scanner

	workingDirectory string
}

func (d *Deployment) scanInteractiveCommand() (stillAlive bool, command string, tail string) {
	stillAlive = d.stdinScanner.Scan()
	text := d.stdinScanner.Text()

	command, tail = d.getCommandAndParams(text)
	return
}

func (d *Deployment) getCommandAndParams(text string) (command string, params string) {
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

func (d *Deployment) gracefulShutdown(portForwarding *engineSsh.Result) (err error) {
	llog.Println("Exiting...")

	portForwarding.Tunnel.Close()

	if err = d.tf.Destroy(); err != nil {
		return merry.Prepend(err, "failed to destroy terraform")
	}

	return
}

func (d *Deployment) Shutdown() (err error) {
	// \todo: Fix port forwarding
	err = d.gracefulShutdown(nil)
	return
}

// readCommandFromInput - прочитать стандартный ввод и запустить выбранные команды
func (d *Deployment) readCommandFromInput(portForwarding *engineSsh.Result) (err error) {
	for {
		fmt.Printf("stroppy> ")
		for stillAlive, command, params := d.scanInteractiveCommand(); stillAlive; {
			statistics.StatsInit()

			switch command {
			case "quit":
				err = d.gracefulShutdown(portForwarding)
				return

			case "pop":
				llog.Println("Starting accounts populating for postgres...")

				if err = d.executePop(params); err != nil {
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

				if err = d.executePay(params); err != nil {
					llog.Errorf("'%s' command failed with error '%v' for arguments '%s'",
						command, err, params)
					break
				} else {
					llog.Println("Transfers test in postgres success")
					llog.Println("Enter next command:")
					break
				}

			case "chaos":
				chaosCommand, chaosCommandParams := d.getCommandAndParams(params)
				switch chaosCommand {
				case "start":
					if err = d.chaosMesh.ExecuteCommand(chaosCommandParams); err != nil {
						llog.Errorf("chaos command failed: %v", err)
					}

				case "stop":
					d.chaosMesh.Stop()
				}

			case "\n":

			default:
				llog.Warnf("You entered unknown command '%s'. To exit enter quit", command)
			}
			break
		}
	}
}

func (d *Deployment) Deploy() (err error) {
	llog.Traceln(d.settings)

	deploySettings := d.settings.DeploySettings
	d.tf = terraform.CreateTerraform(deploySettings, d.workingDirectory, d.workingDirectory)

	var provider terraform.Provider
	if provider, err = d.tf.Run(); err != nil {
		return merry.Prepend(err, "terraform run failed")
	}

	var addressMap terraform.MapAddresses
	if addressMap, err = d.tf.GetAddressMap(); err != nil {
		return merry.Prepend(err, "failed to get address map")
	}

	commandClientType := engineSsh.RemoteClient
	if d.settings.Local {
		commandClientType = engineSsh.LocalClient
	}
	d.sc, err = engineSsh.CreateClient(d.workingDirectory,
		addressMap.MasterExternalIP,
		deploySettings.Provider,
		commandClientType)
	if err != nil {
		return merry.Prepend(err, "failed to init ssh client")
	}

	d.k, err = kubernetes.CreateKubernetes(d.settings, addressMap, d.sc)
	if err != nil {
		return merry.Prepend(err, "failed to init kubernetes")
	}

	var (
		port        int
		portForward *engineSsh.Result
	)
	portForward, port, err = d.k.Deploy()
	if err != nil {
		return merry.Prepend(err, "failed to start kubernetes")
	}

	defer d.k.Stop()

	if err = provider.PerformAdditionalOps(d.settings.DeploySettings.Nodes, d.settings.DeploySettings.Provider, addressMap, d.workingDirectory); err != nil {
		return merry.Prepend(err, "failed to add network storages to provider")
	}

	d.chaosMesh = chaos.CreateController(d.k, d.workingDirectory, d.settings.UseChaos)
	if err = d.chaosMesh.Deploy(); err != nil {
		return merry.Prepend(err, "failed to deploy and start chaos")
	}

	dbtype := d.settings.DatabaseSettings.DBType

	var _cluster db.Cluster
	switch dbtype {
	default:
		return merry.Errorf("unknown database type '%s'", dbtype)

	case cluster.Postgres:
		_cluster = db.CreatePostgresCluster(d.sc, d.k, d.workingDirectory)

	case cluster.Foundation:
		_cluster = db.CreateFoundationCluster(d.sc, d.k, d.workingDirectory)
	}
	if err = _cluster.Deploy(); err != nil {
		return merry.Prependf(err, "'%s' database deploy failed", dbtype)
	}
	llog.Infof("'%s' database deployed successfully\n", dbtype)

	log.Printf(interactiveUsageHelpTemplate, portForward.Port, port)

	if d.payload, err = payload.CreateBasePayload(d.settings, d.chaosMesh); err != nil {
		if dbtype != cluster.Foundation {
			return merry.Prepend(err, "failed to init payload")
		}
		llog.Error(merry.Prepend(err, "failed to init foundation payload"))
	}

	if err = d.readCommandFromInput(portForward); err != nil {
		llog.Error(err)
	} else {
		llog.Info("cluster successfully destroyed")
	}

	return
}
