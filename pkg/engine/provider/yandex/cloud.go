/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package yandex

import (
	"fmt"
	"os"
	"path"

	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	"gitlab.com/picodata/stroppy/pkg/tools"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
)

const (
	SSH_DIR       = ".ssh"                                   //nolint
	PRIV_KEY_NAME = "id_rsa"                                 //nolint
	PUB_KEY_NAME  = "id_rsa.pub"                             //nolint
	CAST_ERR      = "Error then casting type into interface" //nolint
)

// Create YandexCloud provider
// TODO: Switch stroppy to work with HCL files instead of yaml #issue96.
func CreateProvider(settings *config.DeploymentSettings, wd string) (yp *Provider, err error) {
	var templatesConfig *provider.ClusterConfigurations

	_provider := Provider{
		templatesConfig:  templatesConfig,
		settings:         settings,
		workingDirectory: wd,
	}

	yp = &_provider

	return
}

type Provider struct {
	templatesConfig *provider.ClusterConfigurations
	settings        *config.DeploymentSettings

	workingDirectory string

	serviceAccountName     string
	vpcSubnetBlockName     string
	vpcInternalNetworkName string

	tfState TfState
}

func (yandexProvider *Provider) GetTfStateScheme() interface{} {
	return yandexProvider.tfState
}

// Prepare - подготовить файл конфигурации кластера terraform
func (yp *Provider) Prepare() (err error) {
	var clusterParameters provider.ClusterParameters
	if clusterParameters, err = provider.DispatchTemplate(yp.templatesConfig, yp.settings.Flavor); err != nil {
		return
	}
	llog.Infoln(clusterParameters)

	err = yp.prepare(&clusterParameters, yp.settings.Nodes, yp.workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return
}

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально)
func (yp *Provider) AddNetworkDisks(nodes int) error {
	iqnMap, err := yp.getIQNStorage(nodes, yp.workingDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	llog.Infoln("Storages adding for yandex is not used now")

	return nil
}

func (yp *Provider) RemoveProviderSpecificFiles() {
	yandexFilesToClear := []string{
		providerFileName,
	}
	tools.RemovePathList(yandexFilesToClear, yp.workingDirectory)
}

func (yandexProvider *Provider) GetInstanceAddress(
	group, name string,
) (*provider.Addresses, error) {
	var (
		resource  *Resource
		workgroup *Instance
		instance  *GroupInstance
		ok        bool
	)

	if resource, ok = yandexProvider.tfState.GetResource(group); !ok {
		return nil, fmt.Errorf("failed to get resource %s", group)
	}

	if workgroup, ok = resource.GetInstance(group); !ok {
		return nil, fmt.Errorf("failed to get instance or group %s", group)
	}

	if len(workgroup.Attributes.GroupInstances) == 0 {
		return &provider.Addresses{
			Internal: workgroup.Attributes.NetworkInterface[0].IpAddress,
			External: workgroup.Attributes.NetworkInterface[0].NatIpAddress,
		}, nil
	}

	if instance, ok = workgroup.Attributes.GetGroupInstance(name); !ok {
		return nil, fmt.Errorf("failed to get instance or group %s", name)
	}

	return &provider.Addresses{
		Internal: instance.NetworkInterface[0].IpAddress,
		External: instance.NetworkInterface[0].NatIpAddress,
	}, nil
}

//nolint:nosnakecase // constant
// Check ssh key files and directory existence
// 1. Check .ssh directory
// 2. Create .ssh directory
// 3. If directory was been created in step 2, try to copy ssh private key from project dir
// 4. If step 4 failed ask next action
//      - Copy key files from user home ~/.ssh
//      - Crete new key files
//      - Abort execution
func (yp *Provider) CheckSSHPrivateKey(workDir string) error {
	var err error

	llog.Infof("Checking if `.ssh` directory exists in the project directory `%s`", workDir)

	if err = engine.IsDirExists(path.Join(workDir, ".ssh")); err != nil {
		llog.Warnf("Directory `%s/.ssh` does not exists. %s", workDir, err)

		// Create ssh config directory
		if err = os.Mkdir(path.Join(workDir, ".ssh"), os.ModePerm); err != nil {
			return merry.Prepend(
				err,
				fmt.Sprintf("Error then creating `%s/.ssh` directory", workDir),
			)
		}

		llog.Infof("Directory `%s/.ssh` successefully created", workDir)
	} else {
		llog.Infof("Directory `%s/.ssh` already exists", workDir)
	}

	llog.Infoln("Checking of private key for yandex provider")

	if engine.IsFileExists(path.Join(workDir, ".ssh"), PRIV_KEY_NAME) {
		llog.Infoln("Checking of private key for yandex provider: success")

		return nil
	}

	llog.Warnf(
		"Private key for yandex provider `%s/.ssh/id_rsa` does not exist",
		workDir,
	)
	llog.Infof(
		"Check if the key exists in the working directory of the project `%s`",
		workDir,
	)
	// if .ssh directory does not contains id_rsa trying to copy id_rsa file
	// from project root dir
	if err = engine.CopyFileContents(
		path.Join(workDir, "id_rsa"),
		path.Join(workDir, ".ssh", "id_rsa"),
		os.FileMode(engine.RW_ROOT_MODE),
	); err == nil {
		llog.Infof("Private key successefully copied from workdir to .ssh dir")

		return nil
	}

	llog.Debugf(
		"Failed to copy %s/id_rsa to %s/.shh/id_rsa: %v",
		workDir,
		workDir,
		err,
	)
	llog.Infoln("Project working directory does not contains private key")

	if err = engine.AskNextAction(workDir); err != nil {
		return merry.Prepend(err, "Error then creating private key file")
	}

	return nil
}

// Check ssh public key and create if his not exists
// Should called only after 'CheckSSHPrivateKey'.
func (*Provider) CheckSSHPublicKey(workDir string) error {
	var err error

	if !engine.IsFileExists(workDir, PUB_KEY_NAME) {
		llog.Infoln("Checking of public key for yandex provider: unsuccess")

		if err = engine.CreatePublicKey(
			path.Join(workDir, SSH_DIR, PRIV_KEY_NAME),
			path.Join(workDir, SSH_DIR, PUB_KEY_NAME),
		); err != nil {
			return merry.Prepend(err, "Error then creating ssh public key")
		}
	} else {
		llog.Infoln("Checking of public key for yandex provider: success")
	}

	return nil
}

func (yp *Provider) Name() string {
	return provider.Yandex
}

func (yp *Provider) GetDeploymentCommands() (firstStep, thirdStep string) {
	firstStep = "./cluster/provider/yandex/deploy_base_components.sh"
	thirdStep = "./cluster/provider/yandex/deploy_monitoring.sh"

	return
}

// --- private methods ---------------

func (yp *Provider) getIQNStorage(_ int, _ string) (_ map[string]string, _ error) {
	return
}
