/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"fmt"
	"net/url"
	"path/filepath"

	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
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

// Create kubernetes object based on kubeengine.Engine
// engine prepared to run on local machine.
func createKubernetesObject(
	shellState *state.State,
	sshClient ssh.Client,
) (pObj *Engine) {
	pObj = &Engine{
		clusterConfigFile:    filepath.Join(shellState.Settings.WorkingDirectory, "config"),
		sc:                   sshClient,
		UseLocalSession:      shellState.Settings.Local,
		isSshKeyFileOnMaster: false,
	}
	return
}

func CreateEngine(
	sshClient ssh.Client,
	shellState *state.State,
) (e *Engine, err error) {
	e = createKubernetesObject(shellState, sshClient)
	e.sshKeyFileName, e.sshKeyFilePath = e.sc.GetPrivateKeyInfo()

	llog.Infof(
		"kubernetes engine init successfully on directory '%s' and ssh key file '%s'",
		shellState.Settings.WorkingDirectory,
		e.sshKeyFilePath,
	)
	return
}

type Engine struct {
	clusterConfigFile string

	sshKeyFileName string
	sshKeyFilePath string
	sc             ssh.Client

	isSshKeyFileOnMaster bool
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

func (e *Engine) GetResourceURL(
	resource, namespace, name, subresource string,
) (*url.URL, error) {
	var (
		resourceURL *url.URL
		clientSet   *kubernetes.Clientset
		err         error
	)

	if clientSet, err = e.GetClientSet(); err != nil {
		return nil, merry.Prepend(err, "failed to get client set")
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	resourceURL = clientSet.CoreV1().RESTClient().Post().
		Resource(resource).
		Namespace(namespace).
		Name(name).
		SubResource(subresource).URL()

	return resourceURL, nil
}

func (e *Engine) SetClusterConfigFile(path string) {
	e.clusterConfigFile = path
}
