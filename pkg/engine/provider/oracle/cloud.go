/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package oracle

import (
	"fmt"
	"path/filepath"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"

	"gitlab.com/picodata/stroppy/pkg/tools"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
)

const oraclePrivateKeyFile = "private_key.pem"

func (oracleProvider *Provider) GetTfStateScheme() interface{} {
	return &oracleProvider.tfState
}

func CreateProvider(settings *config.DeploymentSettings, wd string) (op *Provider, err error) {
	clusterDeploymentDirectory := filepath.Join(wd, "cluster", "provider", "oracle")

	var templatesConfig *provider.ClusterConfigurations
	if templatesConfig, err = provider.LoadClusterTemplate(clusterDeploymentDirectory); err != nil {
		err = merry.Prepend(err, "failed to read templates for create yandex provider")
		return
	}

	_provider := Provider{
		templatesConfig:  templatesConfig,
		settings:         settings,
		workingDirectory: wd,
	}

	op = &_provider
	return
}

type Provider struct {
	templatesConfig *provider.ClusterConfigurations
	settings        *config.DeploymentSettings

	workingDirectory string

	tfState TfState
}

// Prepare - подготовить файл конфигурации кластера terraform
func (op *Provider) Prepare() (err error) {
	var template provider.ClusterParameters
	if template, err = provider.DispatchTemplate(op.templatesConfig, op.settings.Flavor); err != nil {
		return
	}

	if err = op.prepare(&template, op.settings.Nodes, op.workingDirectory); err != nil {
		return merry.Wrap(err)
	}

	return
}

// getIQNStorage получает идентификаторы IQN (iSCSI qualified name) для каждой машины кластера
func (op *Provider) getIQNStorage(workersCount int) (iqnMap map[string]string, err error) {
	iqnMap = make(map[string]string)
	masterInstance := "instances.0"

	data := string(op.tfState.Data)
	iqnMap["master"] = gjson.Parse(data).
		Get("resources.9").
		Get(masterInstance).
		Get("attributes").
		Get("iqn").Str

	// для Oracle мы задаем при деплое на одну ноду больше, фактически воркеров nodes-1
	for i := 1; i <= workersCount-1; i++ {
		workerInstance := fmt.Sprintf("instances.%v", i)
		worker := fmt.Sprintf("worker-%v", i)
		iqnMap[worker] = gjson.Parse(data).
			Get("resources.9").
			Get(workerInstance).
			Get("attributes").
			Get("iqn").Str
	}

	return
}

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально)
func (op *Provider) AddNetworkDisks(nodes int) error {
	iqnMap, err := op.getIQNStorage(nodes)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	/* В цикле выполняется следующий алгоритм:
	   Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
	   Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя. */

	for index, address := range op.tfState.Outputs.InstancePublicIps.Value[0] {
		var (
			newLoginCmd     string
			updateTargetCmd string
			loginTargetCmd  string
			key             string
		)

		if index == 0 {
			key = "master"
		} else {
			key = fmt.Sprintf("worker-%d", index)
		}

		newLoginCmd = newTargetCmdTemplate
		updateTargetCmd = fmt.Sprintf(updateTargetCmdTemplate, iqnMap[key])
		loginTargetCmd = fmt.Sprintf(loginTargetCmdTemplate, iqnMap[key])

		llog.Infof("Adding network storage to %v %v", key, address)

		llog.Infoln("checking additional storage mount...")
		providerName := provider.Oracle
		ok, err := engineSsh.IsExistEntity(address, checkAddedDiskCmd,
			"block special", op.workingDirectory, providerName)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage mount")
		}

		if !ok {

			err = tools.Retry("send targets",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(
						op.workingDirectory,
						address,
						newLoginCmd,
						providerName,
					)
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "send target is failed to worker %v", key)
			}

			err = tools.Retry("add automatic startup for node",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(
						op.workingDirectory,
						address,
						updateTargetCmd,
						providerName,
					)
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(
					err,
					"adding automatic startup for node is failed to worker %v",
					key,
				)
			}

			err = tools.Retry("target login",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(
						op.workingDirectory,
						address,
						loginTargetCmd,
						providerName,
					)
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "storage is not logged in target %v", key)
			}

			time.Sleep(5 * time.Second)
			llog.Infoln("mount additional storage: success")
		}

		llog.Infoln("checking the partition of additional storage...")
		ok, err = engineSsh.IsExistEntity(
			address,
			checkPartedCmd,
			"primary",
			op.workingDirectory,
			providerName,
		)
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(
				op.workingDirectory,
				address,
				partedVolumeScript,
				providerName,
			); err != nil {
				errorMessage := fmt.Sprintf(
					"failed to execute commands for additional storage partitioning %v",
					key,
				)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")
		ok, err = engineSsh.IsExistEntity(
			address,
			checkExistFileSystemCmd,
			"ext4",
			op.workingDirectory,
			providerName,
		)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(
				op.workingDirectory,
				address,
				createFileSystemCmd,
				providerName,
			); err != nil {
				errorMessage := fmt.Sprintf(
					"failed to execute commands for create additional storage file system %v",
					key,
				)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")
		ok, err = engineSsh.IsExistEntity(
			address,
			checkMountCmd,
			"/opt/local-path-provisioner",
			op.workingDirectory,
			providerName,
		)
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(
				op.workingDirectory,
				address,
				addDirectoryCmdTemplate,
				providerName,
			); err != nil {
				errorMessage := fmt.Sprintf(
					"failed to execute commands for add directory to %v",
					key,
				)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(
				op.workingDirectory,
				address,
				mountLocalPathTemplate,
				providerName,
			); err != nil {
				errorMessage := fmt.Sprintf(
					"failed to mount disk to /opt/local-path-provisioner/ %v",
					key,
				)
				return merry.Prepend(err, errorMessage)
			}

			llog.Infof(
				"Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success",
				key,
			)
		}

		llog.Infof("added network storage to %v", key)

	}

	return nil
}

// TODO: should be refactored in future after tests on oracle cloud.
func (oracleProvider *Provider) GetInstancesAddresses() *provider.InstanceAddresses {
	instanceAddresses := provider.InstanceAddresses{
		Masters: make(map[string]provider.AddrPair),
		Workers: make(map[string]provider.AddrPair),
	}

	for index, ipAddr := range oracleProvider.tfState.Outputs.InstancePrivateIps.Value[0] {
		switch {
		case index == 0:
			name := fmt.Sprintf("master-%d", index)
			instanceAddresses.Masters[name] = provider.AddrPair{
				Internal: ipAddr,
				External: oracleProvider.tfState.Outputs.InstancePublicIps.Value[0][index],
			}
		case index >= 1:
			name := fmt.Sprintf("worker-%d", index)
			instanceAddresses.Workers[name] = provider.AddrPair{
				Internal: ipAddr,
				External: oracleProvider.tfState.Outputs.InstancePublicIps.Value[0][index],
			}
		}
	}

	return &instanceAddresses
}

// TODO: should be refactored in future after tests on oracle cloud.
func (oracleProvider *Provider) GetSubnet() string {
	panic("unimplemented!")
}

// TODO: should be refactored in future after tests on oracle cloud.
func (oracleProvider *Provider) GetNodesInfo() map[string]*provider.NodeParams {
	nodes := make(map[string]*provider.NodeParams)

	for index, fqdn := range oracleProvider.tfState.Outputs.InstancePublicIps.Value[0] {
		node := provider.NodeParams{
			Index:      0,
			InstanceID: "",
			Fqdn:       fqdn,
			Resources: provider.Resources{
				CPU:           0,
				Memory:        0,
				BootDisk:      0,
				SecondaryDisk: 0,
			},
		}
		nodes[fmt.Sprintf("node-%d", index)] = &node
	}

	return nodes
}

func (oracleProvider *Provider) WaitNodes() error {
	return nil
}

func (op *Provider) IsPrivateKeyExist(workingDirectory string) bool {
	var isFoundPrivateKey bool

	llog.Infoln("checking of private key for oracle provider...")
	if isFoundPrivateKey = engine.IsFileExists(workingDirectory, oraclePrivateKeyFile); !isFoundPrivateKey {
		llog.Infoln("checking of private key for oracle provider: unsuccess")
		return false
	}

	llog.Infoln("checking of private key for oracle provider: success")
	return true
}

func (op *Provider) RemoveProviderSpecificFiles() {
	oracleFileToClean := []string{
		instanceFileName,
	}
	tools.RemovePathList(oracleFileToClean, op.workingDirectory)
}

func (op *Provider) SetTerraformStatusData(data []byte) {
	op.tfState.Data = data
}

func (op *Provider) Name() string {
	return provider.Oracle
}

func (op *Provider) GetDeploymentCommands() (firstStep, thirdStep string) {
	scriptParameters := "--pod-addresses "
	internalAddressMap := op.tfState.Outputs.InstancePrivateIps.Value
	for _, podAddress := range internalAddressMap {
		scriptParameters += fmt.Sprintf("%s,", podAddress)
	}

	firstStep = fmt.Sprintf("./cluster/provider/oracle/prepare_oracle.sh %v", scriptParameters)
	thirdStep = "./cluster/provider/oracle/deploy_3rdparties.sh"

	return
}

func (oracleProvider *Provider) CheckSSHPrivateKey(workDir string) error {
	return nil
}

func (oracleProvider *Provider) CheckSSHPublicKey(workDir string) error {
	return nil
}
