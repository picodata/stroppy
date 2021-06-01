package terraform

import (
	"bufio"
	"errors"
	"io/ioutil"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gopkg.in/yaml.v2"
)

// размер ответа terraform show при незапущенном кластере
const linesNotInitTerraformShow = 13

const templatesFileName = "templates.yml"

var errChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

func CreateTerraform(settings *config.DeploySettings, exeFolder, cfgFolder string) (t *Terraform) {
	t = &Terraform{
		settings:          settings,
		exePath:           filepath.Join(exeFolder, "terraform"),
		templatesFilePath: filepath.Join(cfgFolder, templatesFileName),
		WorkDirectory:     cfgFolder,
	}
	t.stateFilePath = filepath.Join(t.WorkDirectory, "terraform.tfstate")

	return
}

type MapAddresses struct {
	MasterExternalIP   string
	MasterInternalIP   string
	MetricsExternalIP  string
	MetricsInternalIP  string
	IngressExternalIP  string
	IngressInternalIP  string
	PostgresExternalIP string
	PostgresInternalIP string
}

type Terraform struct {
	settings *config.DeploySettings

	exePath           string
	templatesFilePath string
	stateFilePath     string

	addressMap *MapAddresses
	isInit     bool

	WorkDirectory string
}

type TemplatesConfig struct {
	Yandex Configurations
}

func (t *Terraform) GetAddressMap() (addressMap MapAddresses, err error) {
	if t.addressMap == nil {
		var _map *MapAddresses
		if _map, err = t.collectInternalExternalAddressMap(); err != nil {
			return
		}

		t.addressMap = _map
	}

	addressMap = *t.addressMap
	return
}

func (t *Terraform) Init() (err error) {
	var terraformVersionString []byte
	if terraformVersionString, err = exec.Command("terraform", "version").Output(); err != nil {
		log.Printf("Failed to find terraform version")

		if errors.Is(err, exec.ErrNotFound) {
			if err = t.install(); err != nil {
				llog.Fatalf("Failed to install terraform: %v ", err)
				return merry.Prepend(err, "failed to install terraform")
			}
			llog.Infof("Terraform install status: success")
		}
	}

	if strings.Contains(string(terraformVersionString), "version") {
		llog.Infof("Founded version %v", string(terraformVersionString[:17]))
	}

	err = t.init()
	return
}

func (t *Terraform) Run() error {
	templatesConfig, err := t.readTemplates()
	if err != nil {
		return merry.Prepend(err, "failed to read templates.yml")
	}

	// передаем варианты и ключи выбора конфигурации для формирования файла провайдера terraform (пока yandex)
	err = t.prepare(*templatesConfig, t.settings)
	if err != nil {
		return merry.Prepend(err, "failed to prepare terraform")
	}

	err = t.init()
	if err != nil {
		return merry.Prepend(err, "failed to init terraform")
	}

	err = t.apply()
	if err != nil {
		return merry.Prepend(err, "failed to apply terraform")
	}

	return nil
}

// apply - развернуть кластер
func (t *Terraform) apply() (err error) {
	terraformShowCmd := exec.Command("terraform", "show")
	terraformShowCmd.Dir = t.WorkDirectory

	var terraformShowOutput []byte
	if terraformShowOutput, err = terraformShowCmd.CombinedOutput(); err != nil {
		return merry.Prepend(err, "failed to Check terraform applying")
	}

	// при незапущенном кластера terraform возвращает пустую строку длиной 13 символов, либо no state c пробелами до 13
	if len(terraformShowOutput) > linesNotInitTerraformShow {
		llog.Infof("terraform already applied, deploy continue...")
		return
	}

	llog.Infoln("Applying terraform...")
	applyCMD := exec.Command("terraform", "apply", "-auto-approve")
	applyCMD.Dir = t.WorkDirectory

	var result []byte
	if result, err = applyCMD.CombinedOutput(); err != nil {
		return merry.Prependf(err, "terraform apply error, possible output '%s'", string(result))
	}

	log.Printf("Terraform applied")
	return
}

// Destroy - уничтожить кластер
func (t *Terraform) Destroy() error {
	destroyCmd := exec.Command("terraform", "destroy", "-force")
	destroyCmd.Dir = t.WorkDirectory

	stdout, err := destroyCmd.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdout pipe for logging destroy of cluster")
	}

	stdoutReader := bufio.NewReader(stdout)
	go engine.HandleReader(stdoutReader)

	llog.Infoln("Destroying terraform...")
	initCmdResult := destroyCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}

	llog.Infoln("Terraform destroyed")
	return nil
}

func (t *Terraform) readTemplates() (*TemplatesConfig, error) {
	var templatesConfig TemplatesConfig
	data, err := ioutil.ReadFile(t.templatesFilePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yml")
	}

	err = yaml.Unmarshal(data, &templatesConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yml")
	}

	return &templatesConfig, nil
}

// prepare
// заполнить конфиг провайдера (for example yandex_compute_instance_group.tf)
func (t *Terraform) prepare(templatesConfig TemplatesConfig, settings *config.DeploySettings) error {
	var templatesInit []ConfigurationUnitParams

	flavor := settings.Flavor
	switch flavor {
	case "small":
		templatesInit = templatesConfig.Yandex.Small
	case "standard":
		templatesInit = templatesConfig.Yandex.Standard
	case "large":
		templatesInit = templatesConfig.Yandex.Large
	case "xlarge":
		templatesInit = templatesConfig.Yandex.Xlarge
	case "xxlarge":
		templatesInit = templatesConfig.Yandex.Xxlarge
	default:
		return merry.Wrap(errChooseConfig)
	}

	err := prepareConfig(templatesInit[2].CPU,
		templatesInit[3].RAM,
		templatesInit[4].Disk,
		templatesInit[1].Platform,
		settings.Nodes)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

func (t *Terraform) collectInternalExternalAddressMap() (*MapAddresses, error) {
	/*
		Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
		 своя пара - внешний (NAT) и внутренний ip.
		 Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
		  т.к. использование encoding/json
		влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
		по переданному тегу json, который можно преобразовать в том числе в строку.
	*/

	if !t.isInit {
		return nil, errors.New("terraform not init")
	}

	var mapIP MapAddresses
	data, err := ioutil.ReadFile(t.stateFilePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read file terraform.tfstate")
	}

	masterExternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterExternalIP := masterExternalIPArray.Get("attributes").Get("network_interface.0").Get("nat_ip_address")
	mapIP.MasterExternalIP = masterExternalIP.Str

	masterInternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterInternalIP := masterInternalIPArray.Get("attributes").Get("network_interface.0").Get("ip_address")
	mapIP.MasterInternalIP = masterInternalIP.Str

	metricsExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsExternalIP := metricsExternalIPArray.Get("instances.0").Get("network_interface.0").Get("nat_ip_address")
	mapIP.MetricsExternalIP = metricsExternalIP.Str

	metricsInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsInternalIP := metricsInternalIPArray.Get("instances.0").Get("network_interface.0").Get("ip_address")
	mapIP.MetricsInternalIP = metricsInternalIP.Str

	ingressExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressExternalIP := ingressExternalIPArray.Get("instances.1").Get("network_interface.0").Get("nat_ip_address")
	mapIP.IngressExternalIP = ingressExternalIP.Str

	ingressInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressInternalIP := ingressInternalIPArray.Get("instances.1").Get("network_interface.0").Get("ip_address")
	mapIP.IngressInternalIP = ingressInternalIP.Str

	postgresExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresExternalIP := postgresExternalIPArray.Get("instances.2").Get("network_interface.0").Get("nat_ip_address")
	mapIP.PostgresExternalIP = postgresExternalIP.Str

	postgresInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresInternalIP := postgresInternalIPArray.Get("instances.2").Get("network_interface.0").Get("ip_address")
	mapIP.PostgresInternalIP = postgresInternalIP.Str

	return &mapIP, nil
}

// install
// установить terraform, если не установлен
func (t *Terraform) install() error {
	llog.Infoln("Preparing the installation terraform...")

	downloadArchiveCmd := exec.Command("curl", "-O",
		"https://releases.hashicorp.com/terraform/0.14.7/terraform_0.14.7_linux_amd64.zip")
	downloadArchiveCmd.Dir = t.WorkDirectory
	err := downloadArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to download archive of terraform")
	}

	unzipArchiveCmd := exec.Command("unzip", "terraform_0.14.7_linux_amd64.zip")
	llog.Infoln(unzipArchiveCmd.String())
	unzipArchiveCmd.Dir = t.WorkDirectory
	err = unzipArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to unzip archive of terraform")
	}

	rmArchiveCmd := exec.Command("rm", "terraform_0.14.7_linux_amd64.zip")
	rmArchiveCmd.Dir = t.WorkDirectory
	err = rmArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to remove archive of terraform")
	}

	installCmd := exec.Command("sudo", "install", "terraform", "/usr/bin/terraform")
	llog.Infoln(installCmd.String())
	installCmd.Dir = t.WorkDirectory
	err = installCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to install terraform")
	}

	tabCompleteCmd := exec.Command("terraform", "-install-autocomplete")
	llog.Infoln(tabCompleteCmd.String())
	tabCompleteCmd.Dir = t.WorkDirectory
	err = tabCompleteCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to add Tab complete to terraform")
	}

	return nil
}

// init - подготовить среду для развертывания
func (t *Terraform) init() error {
	llog.Infoln("Initializating terraform...")

	initCmd := exec.Command("terraform", "init")
	initCmd.Dir = t.WorkDirectory
	initCmdResult := initCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}

	t.isInit = true
	llog.Infoln("Terraform initialized")
	return nil
}
