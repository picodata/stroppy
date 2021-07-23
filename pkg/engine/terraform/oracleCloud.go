package terraform

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func CreateOracleProvider(settings *config.DeploySettings, wd string) (op *OracleProvider, err error) {
	templatesConfig, err := ReadTemplates(wd)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create yandex provider")
	}

	provider := OracleProvider{
		templatesConfig: *templatesConfig,
		settings:        settings,
	}

	op = &provider

	return op, nil
}

type OracleProvider struct {
	templatesConfig TemplatesConfig
	settings        *config.DeploySettings
}

// Prepare - подготовить файл конфигурации кластера terraform
func (op *OracleProvider) Prepare(workingDirectory string) error {
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
		diskSize, op.settings.Nodes, workingDirectory)
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
func (op *OracleProvider) PerformAdditionalOps(nodes int, provider string, addressMap map[string]map[string]string, workingDirectory string) error {
	iqnMap, err := op.getIQNStorage(nodes, workingDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	var addressArray []string

	for _, address := range addressMap["external"] {
		addressArray = append(addressArray, address)
	}

	/*
		В цикле выполняется следующий алгоритм:
		Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
		Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя.
	*/

	llog.Debugln(addressArray)

	for i := range addressArray {

		var targetLoginCmd string

		worker := fmt.Sprintf("worker-%v", i)
		// заполняем шаблон для воркера или мастера
		targetLoginCmd = fmt.Sprintf(TargetLoginCmdTemplate, iqnMap[worker], iqnMap[worker], iqnMap[worker])

		if i == 0 {

			worker = "master"

			targetLoginCmd = fmt.Sprintf(TargetLoginCmdTemplate, iqnMap["master"], iqnMap["master"], iqnMap["master"])

		}

		llog.Infof("Adding network storage to %v %v", worker, addressArray[i])

		llog.Infoln("checking additional storage mount ...")
		ok, err := engineSsh.IsExistEntity(addressArray[i], CheckAdddedDiskCmd, "block special", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage mount ")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], targetLoginCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("additional storage is not mounted to %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("mount additional storage: success")
		}

		llog.Infoln("checking the partition of additional storage...")
		ok, err = engineSsh.IsExistEntity(addressArray[i], CheckPartedCmd, "primary", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], PartedVolumeCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for additional storage partitioning %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")
		ok, err = engineSsh.IsExistEntity(addressArray[i], CheckExistFileSystemCmd, "ext4", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], CreatefileSystemCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for create additional storage file system %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")
		ok, err = engineSsh.IsExistEntity(addressArray[i], CheckMountCmd, "/opt/local-path-provisioner", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], AddDirectoryCmdTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for add directory to %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], MountLocalPathTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to mount disk to /opt/local-path-provisioner/ %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infof("Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success", worker)
		}
		llog.Infof("added network storage to %v", worker)

	}

	return nil
}
