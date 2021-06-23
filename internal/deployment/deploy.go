package deployment

import (
	"bufio"
	"log"
	"os"
	"strings"

	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/statistics"

	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/engine/postgres"
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
	chaosMesh *chaos.Controller
	payload   payload.Payload

	stdinScanner *bufio.Scanner

	workingDirectory string
}

func (d *Deployment) scanInteractiveCommand() (stillAlive bool, command string, tail string) {
	stillAlive = d.stdinScanner.Scan()
	text := d.stdinScanner.Text()

	cmdArr := strings.SplitN(text, " ", 1)
	if len(cmdArr) < 1 {
		return
	}
	command = cmdArr[0]

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
			} else {
				llog.Println("Populating of accounts in postgres success")
				llog.Println("Waiting enter command:")
			}

		case "pay":
			llog.Println("Starting transfer tests for postgres...")

			if err = d.executePay(params); err != nil {
				llog.Errorf("'%s' command failed with error '%v' for arguments '%s'",
					command, err, params)
			} else {
				llog.Println("Transfers test in postgres success")
				llog.Println("Waiting enter command:")
			}

		case "chaos":
			if err = d.chaosMesh.ExecuteCommand(params); err != nil {
				llog.Errorf("chaos command failed: %v", err)
			}

		default:
			llog.Infof("You entered: %v. Expected quit \n", command)
		}
	}

	return
}

func (d *Deployment) Deploy() (err error) {
	llog.Traceln(d.settings)

	deploySettings := d.settings.DeploySettings
	d.tf = terraform.CreateTerraform(deploySettings, d.workingDirectory, d.workingDirectory)
	if err = d.tf.Run(); err != nil {
		return merry.Prepend(err, "terraform run failed")
	}

	var addressMap terraform.MapAddresses
	if addressMap, err = d.tf.GetAddressMap(); err != nil {
		return merry.Prepend(err, "failed to get address map")
	}

	d.sc, err = engineSsh.CreateClient(d.workingDirectory,
		addressMap.MasterExternalIP,
		deploySettings.Provider,
		d.settings.Local)
	if err != nil {
		return merry.Prepend(err, "failed to init ssh client")
	}

	d.k, err = kubernetes.CreateKubernetes(d.settings, addressMap, d.sc)
	if err != nil {
		return merry.Prepend(err, "failed to init kubernetes")
	}

	if d.settings.UseChaos {
		d.chaosMesh = chaos.CreateController(d.k, d.workingDirectory)
		if err = d.chaosMesh.Deploy(); err != nil {
			return merry.Prepend(err, "failed to deploy and start chaos")
		}
	}

	var (
		port        int
		portForward *engineSsh.Result
	)
	if portForward, port, err = d.k.Deploy(); err != nil {
		return merry.Prepend(err, "failed to start kubernetes")
	}
	defer d.k.Stop()

	if d.settings.DatabaseSettings.DBType == "postgres" {
		pg := postgres.CreatePostgresCluster(d.sc, d.k, addressMap, d.workingDirectory)
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

	log.Printf(interactiveUsageHelpTemplate, portForward.Port, port)

	if d.payload, err = payload.CreateBasePayload(d.settings, d.chaosMesh); err != nil {
		return merry.Prepend(err, "failed to init payload")
	}

	if err = d.readCommandFromInput(portForward); err != nil {
		llog.Error(err)
	} else {
		llog.Info("cluster successfully destroyed")
	}

	return
}
