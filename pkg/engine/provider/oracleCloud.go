package provider

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
)

func CreateOracleProvider(settings *config.DeploySettings) (op *OracleProvider, err error) {
	templatesConfig, err := provider.ReadTemplates()
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create provider")
	}

	op.templatesConfig = *templatesConfig

	op.settings = settings

	return op, nil
}

type OracleProvider struct {
	templatesConfig provider.TemplatesConfig
	settings        *config.DeploySettings
}

func (op *OracleProvider) Prepare(workingDirectory string) error {
	var templatesInit []provider.ConfigurationUnitParams

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
		return merry.Wrap(provider.ErrChooseConfig)
	}

	cpuCount := provider.GetCPUCount(templatesInit)

	ramSize := provider.GetRAMSize(templatesInit)

	diskSize := provider.GetDiskSize(templatesInit)

	err := provider.PrepareOracle(cpuCount, ramSize,
		diskSize, op.settings.Nodes, workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

func getIQNStorage(workersCount int, workingDirectory string) (iqnMap map[string]string, err error) {

	stateFilePath := filepath.Join(workingDirectory, terraformStateFileName)
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

func PerformAdditionalOps(nodes int, provider string, addressMap terraform.MapAddresses, workingDirectory string) error {

	iqnMap, err := getIQNStorage(nodes)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	var addressArray []string
	//временное решение до перехода на поддержку динамического кол-ва нод
	addressArray = append(addressArray, addressMap.MasterExternalIP, addressMap.MetricsExternalIP,
		addressMap.IngressExternalIP, addressMap.DatabaseExternalIP)
	/*
		В цикле выполняется следующий алгоритм:
		Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
		Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя.
	*/
	for i := range addressArray {

		var targetLoginCmd string

		worker := fmt.Sprintf("worker-%v", i)
		// заполняем шаблон для воркера или мастера
		targetLoginCmd = fmt.Sprintf(targetLoginCmdTemplate, iqnMap[worker], iqnMap[worker], iqnMap[worker])

		if i == 0 {

			worker = "master"

			targetLoginCmd = fmt.Sprintf(targetLoginCmdTemplate, iqnMap["master"], iqnMap["master"], iqnMap["master"])

		}

		llog.Infof("Adding network storage to %v %v", worker, addressArray[i])

		llog.Infoln("checking additional storage mount ...")
		ok, err := engineSsh.IsExistEntity(addressArray[i], checkAdddedDiskCmd, "block special", workingDirectory, provider)
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
		ok, err = engineSsh.IsExistEntity(addressArray[i], checkPartedCmd, "primary", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], partedVolumeCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for additional storage partitioning %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")
		ok, err = engineSsh.IsExistEntity(addressArray[i], checkExistFileSystemCmd, "ext4", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], createfileSystemCmd, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for create additional storage file system %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")
		ok, err = engineSsh.IsExistEntity(addressArray[i], checkMountCmd, "/opt/local-path-provisioner", workingDirectory, provider)
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], addDirectoryCmdTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for add directory to %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(workingDirectory, addressArray[i], mountLocalPathTemplate, provider); err != nil {
				errorMessage := fmt.Sprintf("failed to mount disk to /opt/local-path-provisioner/ %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infof("Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success", worker)
		}
		llog.Infof("added network storage to %v", worker)

	}

	return nil
}
