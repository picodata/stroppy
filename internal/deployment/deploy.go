/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

import (
	"bufio"
	"os"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"

	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

func createShell(config *config.Settings) (d *shell) {
	d = &shell{
		settings:         config,
		stdinScanner:     bufio.NewScanner(os.Stdin),
		workingDirectory: config.WorkingDirectory,
	}
	return
}

type shell struct {
	tf *terraform.Terraform
	sc engineSsh.Client
	k  *kubernetes.Kubernetes

	cluster   db.Cluster
	settings  *config.Settings
	chaosMesh chaos.Controller
	payload   payload.Payload

	stdinScanner *bufio.Scanner

	workingDirectory string
}

func (sh *shell) gracefulShutdown() (err error) {
	llog.Println("Exiting...")

	sh.k.Shutdown()

	if sh.settings.DestroyOnExit {
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

func (sh *shell) prepareTerraform() (err error) {
	deploymentSettings := sh.settings.DeploymentSettings

	sh.tf = terraform.CreateTerraform(deploymentSettings, sh.workingDirectory, sh.workingDirectory)
	/* отдельный метод, чтобы не смешивать инициализацию terraform, где просто заполняем структуру,
	и провайдера, где читаем файл и его может не быть*/
	if err = sh.tf.InitProvider(); err != nil {
		return merry.Prepend(err, "failed to init provider")
	}

	if ok := sh.tf.Provider.IsPrivateKeyExist(sh.tf.WorkDirectory); !ok {
		return merry.Errorf("failed to check private key exist")
	}
	return
}

func (sh *shell) prepareEngine() (err error) {
	var addressMap map[string]map[string]string
	// Parse terraform.tfstate, get ip_address and nat_address
	if addressMap, err = sh.tf.GetAddressMap(); err != nil {
		return merry.Prepend(err, "failed to get address map")
	}

	// string var (like `remote` or `local`) which will be used to create ssh the client
	commandClientType := engineSsh.RemoteClient
	if sh.settings.Local {
		commandClientType = engineSsh.LocalClient
	}

	// create ssh client
	sh.sc, err = engineSsh.CreateClient(sh.workingDirectory,
		addressMap["external"]["master"],
		sh.settings.DeploymentSettings.Provider,
		commandClientType)
	if err != nil {
		return merry.Prepend(err, "failed to init ssh client")
	}

	sh.k, err = kubernetes.CreateKubernetes(sh.settings, sh.tf.Provider, addressMap, sh.sc)
	if err != nil {
		return merry.Prepend(err, "failed to init kubernetes")
	}

	return
}

func (sh *shell) preparePayload() (err error) {
	sh.cluster, err = db.CreateCluster(
		sh.settings.DatabaseSettings,
		sh.sc,
		sh.k,
		sh.workingDirectory,
	)
	if err != nil {
		return
	}

	if sh.payload, err = payload.CreatePayload(sh.cluster, sh.settings, sh.chaosMesh); err != nil {
		if sh.settings.DatabaseSettings.DBType != cluster.Foundation {
			return merry.Prepend(err, "failed to init payload")
		}

		// \todo: Временное решение, убрать, как будут готовы функции загрузки файлов с подов
		llog.Error(merry.Prepend(err, "failed to init foundation payload"))
		err = nil
	}
	return
}

func (sh *shell) deploy() (err error) {
	llog.Traceln(sh.settings)

	// Build terraform script
	if err = sh.prepareTerraform(); err != nil {
		return
	}

	llog.Traceln(sh.tf)

	// Apply terraform scirpt
	if err = sh.tf.Run(); err != nil {
		return merry.Prepend(err, "terraform run failed")
	}

	// Create and check ssh client
	if err = sh.prepareEngine(); err != nil {
		return
	}

	// Fully functional k8s cluster deploy via ansible
	if err = sh.k.DeployAll(sh.workingDirectory); err != nil {
		return merry.Prepend(err, "failed to start kubernetes")
	}

	// Forvard host ports for grafana
	if err = sh.k.OpenPortForwarding(); err != nil {
		return
	}

	err = sh.tf.Provider.PerformAdditionalOps(sh.settings.DeploymentSettings.Nodes)
	if err != nil {
		return merry.Prepend(err, "failed to add network storages to provider")
	}

	sh.chaosMesh = chaos.CreateController(sh.k.Engine, sh.workingDirectory, sh.settings.UseChaos)
	if err = sh.chaosMesh.Deploy(); err != nil {
		return merry.Prepend(err, "failed to deploy and start chaos")
	}

	if err = sh.preparePayload(); err != nil {
		return
	}
	if err = sh.cluster.Deploy(); err != nil {
		return merry.Prependf(
			err,
			"'%s' database deploy failed",
			sh.settings.DatabaseSettings.DBType,
		)
	}

	if err = sh.payload.Connect(); err != nil {
		// return merry.Prepend(err, "cluster connect")
		// \todo: временно необращаем внимание на эту ошибку

		llog.Errorf("cluster connect: %v", err)
		err = nil
	}

	llog.Infof("'%s' database cluster deployed successfully", sh.settings.DatabaseSettings.DBType)

	llog.Infof(interactiveUsageHelpTemplate, sh.k.MonitoringPort.Port, sh.k.KubernetesPort.Port)
	return
}
