package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strconv"
	"time"

	"gitlab.com/picodata/stroppy/pkg/tools"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const oraclePrivateKeyFile = "private_key.pem"

func CreateOracleProvider(settings *config.DeploymentSettings, wd string) (op *OracleProvider, err error) {
	templatesConfig, err := ReadTemplates(wd)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create yandex provider")
	}

	provider := OracleProvider{
		templatesConfig:  *templatesConfig,
		settings:         settings,
		workingDirectory: wd,
	}

	op = &provider
	return
}

type OracleProvider struct {
	templatesConfig TemplatesConfig
	settings        *config.DeploymentSettings

	workingDirectory string
}

// Prepare - подготовить файл конфигурации кластера terraform
func (op *OracleProvider) Prepare() error {
	var templatesInit []ConfigurationUnitParams

	switch op.settings.Flavor {
	case "small":
		templatesInit = op.templatesConfig.Oracle.Small
	case "standard":
		templatesInit = op.templatesConfig.Oracle.Standard
	case "large":
		templatesInit = op.templatesConfig.Oracle.Large
	case "xlarge":
		templatesInit = op.templatesConfig.Oracle.Xlarge
	case "xxlarge":
		templatesInit = op.templatesConfig.Oracle.Xxlarge
	default:
		return merry.Wrap(ErrChooseConfig)
	}

	cpuCount := GetCPUCount(templatesInit)

	ramSize := GetRAMSize(templatesInit)

	diskSize := GetDiskSize(templatesInit)

	err := PrepareOracle(cpuCount, ramSize,
		diskSize, op.settings.Nodes, op.workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

// getIQNStorage - получить идентификаторы IQN (iSCSI qualified name) для каждой машины кластера
func (op *OracleProvider) getIQNStorage(workersCount int, workingDirectory string) (iqnMap map[string]string, err error) {
	stateFilePath := filepath.Join(workingDirectory, TerraformStateFileName)
	var data []byte

	if data, err = ioutil.ReadFile(stateFilePath); err != nil {
		err = merry.Prepend(err, "failed to read file terraform.tfstate")
		return
	}

	iqnMap = make(map[string]string)
	masterInstance := "instances.0"
	iqnMap["master"] = gjson.Parse(string(data)).Get("resources.9").Get(masterInstance).Get("attributes").Get("iqn").Str
	// для Oracle мы задаем при деплое на одну ноду больше, фактически воркеров nodes-1
	for i := 1; i <= workersCount-1; i++ {
		workerInstance := fmt.Sprintf("instances.%v", i)
		worker := fmt.Sprintf("worker-%v", i)
		iqnMap[worker] = gjson.Parse(string(data)).Get("resources.9").Get(workerInstance).Get("attributes").Get("iqn").Str
	}

	return iqnMap, nil
}

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально)
func (op *OracleProvider) PerformAdditionalOps(nodes int, provider string, addressMap map[string]map[string]string) error {
	iqnMap, err := op.getIQNStorage(nodes, op.workingDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	/*
		В цикле выполняется следующий алгоритм:
		Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
		Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя.
	*/

	for k, address := range addressMap["external"] {
		var newLoginCmd string
		var updateTargetCmd string
		var loginTargetCmd string

		newLoginCmd = NewTargetCmdTemplate
		updateTargetCmd = fmt.Sprintf(UpdateTargetCmdTemplate, iqnMap[k])
		loginTargetCmd = fmt.Sprintf(loginTargetCmdTemplate, iqnMap[k])

		llog.Infof("Adding network storage to %v %v", k, address)

		llog.Infoln("checking additional storage mount ...")
		ok, err := engineSsh.IsExistEntity(address, CheckAdddedDiskCmd,
			"block special", op.workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage mount ")
		}

		if !ok {

			err = tools.Retry("send targets",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, newLoginCmd, provider)
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "send target is failed to worker %v", k)
			}

			err = tools.Retry("add automatic startup for node",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, updateTargetCmd, provider)
					return
				},
				tools.RetryStandardRetryCount,
				tools.RetryStandardWaitingTime)
			if err != nil {
				return merry.Prependf(err, "adding automatic startup for node is failed to worker %v", k)
			}

			err = tools.Retry("target login",
				func() (err error) {
					_, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, loginTargetCmd, provider)
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
		ok, err = engineSsh.IsExistEntity(address, CheckPartedCmd, "primary", op.workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, PartedVolumeCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for additional storage partitioning %v", k)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")
		ok, err = engineSsh.IsExistEntity(address, CheckExistFileSystemCmd, "ext4", op.workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, CreatefileSystemCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for create additional storage file system %v", k)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")
		ok, err = engineSsh.IsExistEntity(address, CheckMountCmd, "/opt/local-path-provisioner", op.workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, AddDirectoryCmdTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for add directory to %v", k)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(op.workingDirectory, address, MountLocalPathTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to mount disk to /opt/local-path-provisioner/ %v", k)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infof("Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success", k)
		}
		llog.Infof("added network storage to %v", k)

	}

	return nil
}

func (op *OracleProvider) GetAddressMap(stateFilePath string, nodes int) (mapIPAddresses map[string]map[string]string, err error) {
	/* Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
	 * своя пара - внешний (NAT) и внутренний ip.
	 * Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
	 * т.к. использование encoding/json
	 * влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
	 * по переданному тегу json, который можно преобразовать в том числе в строку. */

	var data []byte
	if data, err = ioutil.ReadFile(stateFilePath); err != nil {
		err = merry.Prepend(err, "failed to read file terraform.tfstate")
		return
	}

	mapIPAddresses = make(map[string]map[string]string)
	workerKey := "worker-%v"
	oracleInstanceValue := "value.0.%v"
	externalAddress := make(map[string]string)
	internalAddress := make(map[string]string)

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

	mapIPAddresses["external"] = externalAddress
	mapIPAddresses["internal"] = internalAddress

	llog.Debugln("result of getting ip addresses: ", mapIPAddresses)

	return mapIPAddresses, nil
}

func (op *OracleProvider) IsPrivateKeyExist(workingDirectory string) bool {
	var isFoundPrivateKey bool

	llog.Infoln("checking of private key for oracle provider...")
	if isFoundPrivateKey = engine.IsFileExists(workingDirectory, oraclePrivateKeyFile); !isFoundPrivateKey {
		llog.Infoln("checking of private key for oracle provider: unsuccess")
		return false
	}

	llog.Infoln("checking of private key for oracle provider: success")
	return true
}

func (op *OracleProvider) RemoveProviderSpecificFiles() {
	oracleFileToClean := []string{
		instanceFileName,
	}
	tools.RemovePathList(oracleFileToClean, op.workingDirectory)
}