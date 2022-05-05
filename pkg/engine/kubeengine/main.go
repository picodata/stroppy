/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"fmt"
	"net/url"
	"path/filepath"

	"gitlab.com/picodata/stroppy/pkg/engine/ssh"

	"k8s.io/client-go/rest"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"k8s.io/client-go/kubernetes"
)

func CreateSystemShell(settings *config.Settings) (sc ssh.Client, err error) {
	kubernetesMasterAddress := settings.TestSettings.KubernetesMasterAddress
	commandClientType := ssh.RemoteClient

	if settings.TestSettings.UseCloudStroppy {
		if kubernetesMasterAddress == "" {
			err = fmt.Errorf("kubernetes master address is empty")

			return
		}
	} else {
		commandClientType = ssh.DummyClient
	}

	sc, err = ssh.CreateClient(settings.WorkingDirectory,
		kubernetesMasterAddress,
		settings.DeploymentSettings.Provider,
		commandClientType)
	if err != nil {
		err = merry.Prependf(err, "setup ssh tunnel to '%s'", kubernetesMasterAddress)
	}

	return
}

func createKubernetesObject(settings *config.Settings,
	terraformAddressMap map[string]map[string]string,
	sshClient ssh.Client,
) (pObj *Engine) {
	pObj = &Engine{
		WorkingDirectory:  settings.WorkingDirectory,
		clusterConfigFile: filepath.Join(settings.WorkingDirectory, "config"),

		AddressMap: terraformAddressMap,
		sc:         sshClient,

		UseLocalSession:      settings.Local,
		isSSHKeyFileOnMaster: false,
	}

	return
}

func CreateEngine(settings *config.Settings,
	terraformAddressMap map[string]map[string]string,
	sshClient ssh.Client,
) (e *Engine, err error) {
	e = createKubernetesObject(settings, terraformAddressMap, sshClient)
	e.sshKeyFileName, e.sshKeyFilePath = e.sc.GetPrivateKeyInfo()

	llog.Infof("kubernetes engine init successfully on directory '%s' and ssh key file '%s'",
		e.WorkingDirectory, e.sshKeyFilePath)

	return
}

type Engine struct {
	WorkingDirectory  string
	clusterConfigFile string

	AddressMap map[string]map[string]string

	sshKeyFileName string
	sshKeyFilePath string
	sc             ssh.Client

	isSSHKeyFileOnMaster bool
	UseLocalSession      bool
}

func (e *Engine) GetClientSet() (clientSet *kubernetes.Clientset, err error) {
	var _config *rest.Config

	if _config, err = e.GetKubeConfig(); err != nil {
		err = merry.Prepend(err, "failed to get kubeconfig for clientSet")

		return
	}

	// clientSet - клиент для обращения к группам сущностей k8s
	if clientSet, err = kubernetes.NewForConfig(_config); err != nil {
		return nil, merry.Prepend(err, "failed to create clientSet")
	}

	return
}

func (e *Engine) GetResourceURL(resource, namespace, name, subresource string) (url *url.URL, err error) {
	var clientSet *kubernetes.Clientset

	if clientSet, err = e.GetClientSet(); err != nil {
		return
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	url = clientSet.CoreV1().RESTClient().Post().
		Resource(resource).
		Namespace(namespace).
		Name(name).
		SubResource(subresource).URL()

	return
}
