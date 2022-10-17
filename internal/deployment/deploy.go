/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

import (
	"bufio"
	"fmt"
	"os"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"

	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func createShell(config *config.Settings) (d *shell) {
	d = &shell{
		state:            state.State{Settings: config}, //nolint
		stdinScanner:     bufio.NewScanner(os.Stdin),
		workingDirectory: config.WorkingDirectory,
	}

	return
}

type shell struct {
	state state.State

	tf *terraform.Terraform
	sc engineSsh.Client
	k  *kubernetes.Kubernetes

	cluster   db.Cluster
	chaosMesh chaos.Controller
	payload   payload.Payload

	stdinScanner *bufio.Scanner

	workingDirectory string
}

func (sh *shell) gracefulShutdown() (err error) {
	llog.Println("Exiting...")

	sh.k.Shutdown()

	if sh.state.Settings.DestroyOnExit {
		if err = sh.tf.Destroy(); err != nil {
			return merry.Prepend(err, "failed to destroy terraform")
		}
	}

	return
}

func (sh *shell) Shutdown() (err error) {
	err = sh.gracefulShutdown()

	return
}

func Deploy(settings *config.Settings) (shell Shell, err error) {
	sh := createShell(settings)
	if err = sh.deploy(); err != nil {
		return
	}

	shell = sh

	return
}

func (sh *shell) prepareTerraform() error {
	var err error

	deploymentSettings := sh.state.Settings.DeploymentSettings

	sh.tf = terraform.CreateTerraform(deploymentSettings, sh.workingDirectory, sh.workingDirectory)
	/* отдельный метод, чтобы не смешивать инициализацию terraform, где просто заполняем структуру,
	и провайдера, где читаем файл и его может не быть*/
	if err = sh.tf.InitProvider(); err != nil {
		return merry.Prepend(err, "failed to init provider")
	}

	if err = sh.tf.Provider.CheckSSHPrivateKey(sh.tf.WorkDirectory); err != nil {
		return merry.Prepend(err, "Error then checking ssh private key")
	}

	if err = sh.tf.Provider.CheckSSHPublicKey(sh.tf.WorkDirectory); err != nil {
		return merry.Prepend(err, "Error then checking ssh public key")
	}

	return nil
}

func (sh *shell) prepareEngine() error {
	var err error

	instanceAddresses := sh.tf.Provider.GetInstancesAddresses()

	sh.state.NodesInfo = state.NodesInfo{
		MastersCnt: instanceAddresses.MastersCnt(sh.state.Settings.DeploymentSettings.AllMasters),
		WorkersCnt: instanceAddresses.WorkersCnt(sh.state.Settings.DeploymentSettings.AllWorkers),
		IPs: state.IPs{
			FirstMasterIP: instanceAddresses.GetFirstMaster(),
			FirstWokerIP:  instanceAddresses.GetFirstWorker(),
		},
		NodesParams: sh.tf.Provider.GetNodesInfo(),
	}

	sh.state.InstanceAddresses = instanceAddresses
	sh.state.Subnet = sh.tf.Provider.GetSubnet()

	if sh.state.Settings.DatabaseSettings.Workers == 0 {
		llog.Debugln("Number of workers defined for db test is: 0")

		//nolint
		databaseTestWorkers := sh.state.NodesInfo.GetFirstMaster().Resources.CPU * uint64(4)
		sh.state.Settings.DatabaseSettings.Workers = databaseTestWorkers

		llog.Debugf(
			"Set the number of workers defined for db test to: %d\n", databaseTestWorkers,
		)
	}

	// string var (like `remote` or `local`) which will be used to create ssh the client
	commandClientType := engineSsh.RemoteClient
	if sh.state.Settings.TestSettings.IsLocal() {
		commandClientType = engineSsh.LocalClient
	}

	// create ssh client
	sh.sc, err = engineSsh.CreateClient(
		sh.workingDirectory,
		sh.state.InstanceAddresses.GetFirstMaster().External,
		sh.state.Settings.DeploymentSettings.Provider,
		commandClientType)
	if err != nil {
		return merry.Prepend(err, "failed to init ssh client")
	}

	if sh.k, err = kubernetes.CreateKubernetes(sh.sc, &sh.state); err != nil {
		return merry.Prepend(err, "failed to init kubernetes")
	}

	return nil
}

func (sh *shell) prepareDBForTests() error {
	var err error

	llog.Infoln("Prepating database payload")

	if sh.cluster, err = db.CreateCluster(
		sh.sc,
		sh.k,
		&sh.state,
	); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf(
				"Error then creating '%s' cluster",
				sh.state.Settings.DatabaseSettings.DBType,
			),
		)
	}

	if sh.payload, err = payload.CreatePayload(
		sh.cluster,
		sh.state.Settings,
		sh.chaosMesh,
	); err != nil {
		if sh.state.Settings.DatabaseSettings.DBType != cluster.Foundation {
			return merry.Prepend(err, "failed to init payload")
		}

		// \todo: Временное решение, убрать, как будут готовы функции загрузки файлов с подов
		llog.Error(merry.Prepend(err, "failed to init foundation payload"))
	}

	return nil
}

func (sh *shell) deploy() error {
	var err error

	// Build terraform script
	if err = sh.prepareTerraform(); err != nil {
		return merry.Prepend(err, "Error then preparing terraform")
	}

	// Apply terraform scirpt
	if err = sh.tf.Run(); err != nil {
		return merry.Prepend(err, "Terraform run failed")
	}

	// Create and check ssh client
	if err = sh.prepareEngine(); err != nil {
		return merry.Prepend(err, "Error then stroppy ssh engine")
	}

	// Fully functional k8s cluster deploy via ansible
	// 1. Deploy monitoring via grafana stack
	// 2. Deploy kubernetes cluster
	// 3. Deploy stroppy pod
	if err = sh.k.DeployK8SWithInfrastructure(&sh.state); err != nil {
		return merry.Prepend(err, "Failed to deploy kubernetes and infrastructure")
	}

	if err = sh.tf.Provider.AddNetworkDisks(len(sh.state.NodesInfo.NodesParams)); err != nil {
		return merry.Prepend(err, "Failed to add network storages to provider")
	}

	sh.chaosMesh = chaos.CreateController(
		sh.k.Engine,
		&sh.state,
	)
	if err = sh.chaosMesh.Deploy(&sh.state); err != nil {
		return merry.Prepend(err, "Failed to deploy and start chaos")
	}

	if err = sh.prepareDBForTests(); err != nil {
		return merry.Prepend(err, "Error then preparing database")
	}

	// Deploy database cluster
	if err = sh.cluster.Deploy(sh.k, &sh.state); err != nil {
		return merry.Prependf(
			err,
			"'%s' database deploy failed",
			sh.state.Settings.DatabaseSettings.DBType,
		)
	}

	// Start port forwarding
	if err = sh.k.OpenPortForwarding(&sh.state); err != nil {
		return merry.Prepend(err, "failed to open port forwarding")
	}

	if err = sh.payload.Connect(); err != nil {
		// return merry.Prepend(err, "cluster connect")
		// \todo: временно необращаем внимание на эту ошибку
		if sh.state.Settings.DatabaseSettings.DBType == "ydb" {
			llog.Debugln("Connection from remote stroppy client not implemented yet for YDB")
		} else {
			llog.Errorf("cluster connect: %v", err)
		}
		err = nil
	}

	llog.Infof(
		"Databale cluster of '%s' deployed successfully",
		sh.state.Settings.DatabaseSettings.DBType,
	)
	llog.Infof(
		interactiveUsageHelpTemplate,
		sh.state.Settings.DeploymentSettings.GrUser,
		sh.state.Settings.DeploymentSettings.GrPassword,
		sh.state.NodesInfo.IPs.FirstMasterIP.External,
		sh.state.Settings.DeploymentSettings.GrPort,
		sh.state.NodesInfo.IPs.FirstMasterIP.External,
		sh.state.Settings.DeploymentSettings.PromPort,
	)

	return nil
}
