package yandex

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"

	"gitlab.com/picodata/stroppy/pkg/tools"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
)

const (
	yandexPrivateKeyFile = "id_rsa"
	yandexPublicKeyFile  = "id_rsa.pub"
)

func CreateProvider(settings *config.DeploymentSettings, wd string) (yp *Provider, err error) {
	clusterDeploymentDirectory := filepath.Join(wd, "cluster", "provider", "yandex")

	var templatesConfig *provider.ClusterConfigurations
	if templatesConfig, err = provider.LoadClusterTemplate(clusterDeploymentDirectory); err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create yandex provider")
	}

	_provider := Provider{
		templatesConfig:  templatesConfig,
		settings:         settings,
		workingDirectory: wd,
		addressMapLock:   sync.Mutex{},
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

	tfStateData    []byte
	addressMap     map[string]map[string]string
	addressMapLock sync.Mutex
}

// Prepare - подготовить файл конфигурации кластера terraform
func (yp *Provider) Prepare() (err error) {
	var clusterParameters provider.ClusterParameters
	if clusterParameters, err = provider.DispatchTemplate(yp.templatesConfig, yp.settings.Flavor); err != nil {
		return
	}

	err = yp.prepare(&clusterParameters, yp.settings.Nodes, yp.workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return
}

// PerformAdditionalOps - добавить отдельные сетевые диски (для yandex пока неактуально)
func (yp *Provider) PerformAdditionalOps(nodes int) error {
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

func (yp *Provider) SetTerraformStatusData(data []byte) {
	yp.tfStateData = data
}

func (yp *Provider) reparseAddressMap(nodes int) (err error) {
	if yp.tfStateData == nil {
		err = errors.New("tf state data empty")
		return
	}

	workerKey := "worker-%v"
	yandexInstanceValue := "instances.%v"
	externalAddress := make(map[string]string)
	internalAddress := make(map[string]string)

	data := yp.tfStateData
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

	for i := 1; i <= nodes; i++ {
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

	for i := 1; i <= nodes; i++ {
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

	yp.addressMap = make(map[string]map[string]string)
	yp.addressMap["external"] = externalAddress
	yp.addressMap["internal"] = internalAddress
	return
}

func (yp *Provider) GetAddressMap(nodes int) (mapIPAddresses map[string]map[string]string, err error) {
	/* Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
	 * своя пара - внешний (NAT) и внутренний ip.
	 * Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
	 * т.к. использование encoding/json
	 * влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
	 * по переданному тегу json, который можно преобразовать в том числе в строку. */

	defer func() {
		mapIPAddresses = provider.DeepCopyAddressMap(yp.addressMap)
		llog.Debugln("result of getting ip addresses: ", mapIPAddresses)
	}()

	yp.addressMapLock.Lock()
	defer yp.addressMapLock.Lock()

	if yp.addressMap != nil {
		return
	}

	err = yp.reparseAddressMap(nodes)
	return
}

func (yp *Provider) IsPrivateKeyExist(workingDirectory string) bool {
	var isFoundPrivateKey bool
	var isFoundPublicKey bool

	llog.Infoln("checking of private key for yandex provider...")
	isFoundPrivateKey = engine.IsFileExists(workingDirectory, yandexPrivateKeyFile)
	if !isFoundPrivateKey {
		llog.Infoln("checking of private key for yandex provider: unsuccess")
	} else {
		llog.Infoln("checking of private key for yandex provider: success")
	}

	llog.Infoln("checking of public key for yandex provider...")
	if isFoundPublicKey = engine.IsFileExists(workingDirectory, yandexPublicKeyFile); !isFoundPublicKey {
		llog.Infoln("checking of public key for yandex provider: unsuccess")
	}

	if isFoundPrivateKey && isFoundPublicKey {
		llog.Infoln("checking of authtorized keys for yandex provider: success")
		return true
	}

	llog.Errorln("checking of authtorized keys for yandex provider: unsuccess")
	return false
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
