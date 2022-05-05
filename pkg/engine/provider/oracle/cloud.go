/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package oracle

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
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
		addressMapLock:   sync.Mutex{},
	}

	op = &_provider

	return
}

type Provider struct {
	templatesConfig *provider.ClusterConfigurations
	settings        *config.DeploymentSettings

	workingDirectory string

	tfStateData    []byte
	addressMap     map[string]map[string]string
	addressMapLock sync.Mutex
}

// Prepare - подготовить файл конфигурации кластера terraform.
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

// getIQNStorage получает идентификаторы IQN (iSCSI qualified name) для каждой машины кластера.
func (op *Provider) getIQNStorage(workersCount int) (iqnMap map[string]string, err error) {
	iqnMap = make(map[string]string)
	masterInstance := "instances.0"

	data := string(op.tfStateData)
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

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально).
func (op *Provider) PerformAdditionalOps(nodes int) error {
	iqnMap, err := op.getIQNStorage(nodes)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	/* В цикле выполняется следующий алгоритм:
	   Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
	   Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя. */

	op.addressMapLock.Lock()
	defer op.addressMapLock.Unlock()

	for k, address := range op.addressMap["external"] {
		var newLoginCmd string

		var updateTargetCmd string

		var loginTargetCmd string

		newLoginCmd = newTargetCmdTemplate
		updateTargetCmd = fmt.Sprintf(updateTargetCmdTemplate, iqnMap[k])
		loginTargetCmd = fmt.Sprintf(loginTargetCmdTemplate, iqnMap[k])

		llog.Infof("Adding network storage to %v %v", k, address)
		llog.Infoln("checking additional storage mount...")

		providerName := provider.Oracle

		ok, err := engineSsh.IsExistEntity(address, checkAddedDiskCmd, "block special", op.workingDirectory, providerName)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage mount")
		}

		if !ok {
			err = tools.Retry("send targets",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, newLoginCmd, providerName)

					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "send target is failed to worker %v", k)
			}

			err = tools.Retry("add automatic startup for node",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, updateTargetCmd, providerName)

					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "adding automatic startup for node is failed to worker %v", k)
			}

			err = tools.Retry("target login",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, loginTargetCmd, providerName)

					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "storage is not logged in target %v", k)
			}

			time.Sleep(5 * time.Second)
			llog.Infoln("mount additional storage: success")
		}

		llog.Infoln("checking the partition of additional storage...")

		ok, err = engineSsh.IsExistEntity(address, checkPartedCmd, "primary", op.workingDirectory, providerName)
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, partedVolumeScript, providerName); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for additional storage partitioning %v", k)

				return merry.Prepend(err, errorMessage)
			}

			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")

		ok, err = engineSsh.IsExistEntity(address, checkExistFileSystemCmd, "ext4", op.workingDirectory, providerName)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, createFileSystemCmd, providerName); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for create additional storage file system %v", k)

				return merry.Prepend(err, errorMessage)
			}

			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")

		ok, err = engineSsh.IsExistEntity(address, checkMountCmd, "/opt/local-path-provisioner", op.workingDirectory, providerName)
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, addDirectoryCmdTemplate, providerName); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for add directory to %v", k)

				return merry.Prepend(err, errorMessage)
			}

			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, mountLocalPathTemplate, providerName); err != nil {
				errorMessage := fmt.Sprintf("failed to mount disk to /opt/local-path-provisioner/ %v", k)

				return merry.Prepend(err, errorMessage)
			}

			llog.Infof("Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success", k)
		}

		llog.Infof("added network storage to %v", k)
	}

	return nil
}

func (op *Provider) reparseAddressMap(nodes int) (err error) {
	// Осторожно, внутренний метод без блокировки
	if op.tfStateData == nil {
		err = errors.New("terraform state data is empty")

		return
	}

	workerKey := "worker-%v"
	oracleInstanceValue := "value.0.%v"
	externalAddress := make(map[string]string)
	internalAddress := make(map[string]string)

	data := op.tfStateData
	externalAddress["master"] = gjson.Parse(string(data)).
		Get("outputs").
		Get("instance_public_ips").
		Get("value.0.0").Str

	internalAddress["master"] = gjson.Parse(string(data)).
		Get("outputs").
		Get("instance_private_ips").
		Get("value.0.0").Str

	for i := 1; i <= nodes-1; i++ {
		key := fmt.Sprintf(workerKey, i)
		currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
		externalAddress[key] = gjson.Parse(string(data)).
			Get("outputs").
			Get("instance_public_ips").
			Get(currentInstanceValue).Str
	}

	for i := 1; i <= nodes-1; i++ {
		key := fmt.Sprintf(workerKey, i)
		currentInstanceValue := fmt.Sprintf(oracleInstanceValue, strconv.Itoa(i))
		internalAddress[key] = gjson.Parse(string(data)).
			Get("outputs").
			Get("instance_private_ips").
			Get(currentInstanceValue).Str
	}

	op.addressMapLock.Lock()
	defer op.addressMapLock.Unlock()

	op.addressMap = make(map[string]map[string]string)
	op.addressMap["external"] = externalAddress
	op.addressMap["internal"] = internalAddress

	return
}

// GetAddressMap Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
// своя пара - внешний (NAT) и внутренний ip.
// Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
// т.к. использование encoding/json
// влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
// по переданному тегу json, который можно преобразовать в том числе в строку.
func (op *Provider) GetAddressMap(nodes int) (mapIPAddresses map[string]map[string]string, err error) {
	defer func() {
		mapIPAddresses = provider.DeepCopyAddressMap(op.addressMap)
		llog.Debugln("result of getting ip addresses: ", mapIPAddresses)
	}()

	op.addressMapLock.Lock()
	defer op.addressMapLock.Unlock()

	if op.addressMap != nil {
		return
	}

	err = op.reparseAddressMap(nodes)

	return
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
	op.tfStateData = data
}

func (op *Provider) Name() string {
	return provider.Oracle
}

func (op *Provider) GetDeploymentCommands() (firstStep, thirdStep string) {
	op.addressMapLock.Lock()
	defer op.addressMapLock.Unlock()

	scriptParameters := "--pod-addresses "
	internalAddressMap := op.addressMap["internal"]

	for _, podAddress := range internalAddressMap {
		scriptParameters += fmt.Sprintf("%s,", podAddress)
	}

	firstStep = fmt.Sprintf("./cluster/provider/oracle/prepare_oracle.sh %v", scriptParameters)
	thirdStep = "./cluster/provider/oracle/deploy_3rdparties.sh"

	return
}
