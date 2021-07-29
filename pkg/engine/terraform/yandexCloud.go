package terraform

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
)

func CreateYandexProvider(settings *config.DeploySettings, wd string) (yp *YandexProvider, err error) {
	templatesConfig, err := ReadTemplates(wd)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create yandex provider")
	}

	provider := YandexProvider{
		templatesConfig: *templatesConfig,
		settings:        settings,
	}

	yp = &provider

	return yp, nil
}

type YandexProvider struct {
	templatesConfig TemplatesConfig
	settings        *config.DeploySettings
}

// Prepare - подготовить файл конфигурации кластера terraform
func (yp *YandexProvider) Prepare(workingDirectory string) error {
	var templatesInit []ConfigurationUnitParams

	switch yp.settings.Flavor {
	case "small":
		templatesInit = yp.templatesConfig.Yandex.Small
	case "standard":
		templatesInit = yp.templatesConfig.Yandex.Standard
	case "large":
		templatesInit = yp.templatesConfig.Yandex.Large
	case "xlarge":
		templatesInit = yp.templatesConfig.Yandex.Xlarge
	case "xxlarge":
		templatesInit = yp.templatesConfig.Yandex.Xxlarge
	default:
		return merry.Wrap(ErrChooseConfig)
	}

	cpuCount := GetCPUCount(templatesInit)

	ramSize := GetRAMSize(templatesInit)

	diskSize := GetDiskSize(templatesInit)

	platform := GetPlatform(templatesInit)

	err := PrepareYandex(cpuCount, ramSize,
		diskSize, yp.settings.Nodes, platform, workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

func (yp *YandexProvider) getIQNStorage(workersCount int, workingDirectory string) (iqnMap map[string]string, err error) {
	return iqnMap, nil
}

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально)
func (yp *YandexProvider) PerformAdditionalOps(nodes int, provider string, addressMap map[string]map[string]string, workingDirectory string) error {
	iqnMap, err := yp.getIQNStorage(nodes, workingDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	llog.Debugln(iqnMap)

	llog.Infoln("Storages adding for yandex is not used now")
	return nil
}

func (yp *YandexProvider) GetAddressMap(stateFilePath string, nodes int) (mapIPAddresses map[string]map[string]string, err error) {
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
	workerKey := "worker_%v"
	yandexInstanceValue := "instances.%v"
	externalAddress := make(map[string]string)
	internalAddress := make(map[string]string)

	externalAddress["master"] = gjson.Parse(string(data)).
		Get("resources.1").
		Get("instances.0").
		Get("attributes").
		Get("network_interface.0").
		Get("nat_ip_address").Str

	internalAddress["master"] = gjson.Parse(string(data)).
		Get("resources.1").
		Get("instances.0").
		Get("attributes").
		Get("network_interface.0").
		Get("ip_address").Str

	for i := 1; i <= nodes-1; i++ {
		key := fmt.Sprintf(workerKey, i)
		currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
		externalAddress[key] = gjson.Parse(string(data)).
			Get("resources.2").
			Get("instances.0").
			Get("attributes").
			Get(currentInstanceValue).
			Get("network_interface.0").
			Get("nat_ip_address").Str
	}

	for i := 1; i <= nodes-1; i++ {
		key := fmt.Sprintf(workerKey, i)
		currentInstanceValue := fmt.Sprintf(yandexInstanceValue, strconv.Itoa(i-1))
		internalAddress[key] = gjson.Parse(string(data)).
			Get("resources.2").
			Get("instances.0").
			Get("attributes").
			Get(currentInstanceValue).
			Get("network_interface.0").
			Get("ip_address").Str
	}

	mapIPAddresses["external"] = externalAddress
	mapIPAddresses["internal"] = internalAddress

	llog.Debugln("result of getting ip addresses: ", mapIPAddresses)

	return mapIPAddresses, nil
}

func (yp *YandexProvider) IsPrivateKeyExist(workingDirectory string) (bool, error) {
	privateKeyPath := filepath.Join(workingDirectory, "id_rsa")

	publicKeyPath := filepath.Join(workingDirectory, "id_rsa.pub")

	if _, err := os.Stat(privateKeyPath); err != nil {
		if os.IsNotExist(err) {
			return false, merry.Prepend(err, "private key file not found. Create it, please.")
		}
		return false, merry.Prepend(err, "failed to find private key file")
	}

	if _, err := os.Stat(publicKeyPath); err != nil {
		if os.IsNotExist(err) {
			return false, merry.Prepend(err, "public key file not found. Create it, please.")
		}
		return false, merry.Prepend(err, "failed to find public key file")
	}

	return true, nil
}
