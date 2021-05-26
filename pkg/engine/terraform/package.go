package terraform

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"io/ioutil"
	"log"
	"os/exec"
	"path/filepath"
)

var errChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

func CreateTerraform(exeFolder, cfgFolder string) (t *Terraform) {
	t = &Terraform{
		exePath:           filepath.Join(exeFolder, "terraform"),
		templatesFilePath: filepath.Join(cfgFolder),
		WorkDirectory:     cfgFolder,
	}
	return
}

type MapAddresses struct {
	masterExternalIP   string
	masterInternalIP   string
	metricsExternalIP  string
	metricsInternalIP  string
	ingressExternalIP  string
	ingressInternalIP  string
	postgresExternalIP string
	postgresInternalIP string
}

type Terraform struct {
	exePath           string
	templatesFilePath string
	WorkDirectory     string
}

type TemplatesConfig struct {
	Yandex Configurations
}

// terraformApply - развернуть кластер
func (t *Terraform) terraformApply() error {
	checkLaunchTerraform := exec.Command("terraform", "show")
	checkLaunchTerraform.Dir = t.WorkDirectory

	checkLaunchTerraformResult, err := checkLaunchTerraform.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to Check terraform applying")
	}

	// при незапущенном кластера terraform возвращает пустую строку длиной 13 символов, либо no state c пробелами до 13
	if len(checkLaunchTerraformResult) > linesNotInitTerraformShow {
		llog.Infof("terraform already applied, deploy continue...")
		return nil
	}

	llog.Infoln("Applying terraform...")
	applyCMD := exec.Command("terraform", "apply", "-auto-approve")
	applyCMD.Dir = t.WorkDirectory
	result, err := applyCMD.CombinedOutput()
	if err != nil {
		llog.Errorln(string(result))
		return merry.Wrap(err)
	}

	log.Printf("Terraform applied")
	return nil
}

// terraformDestroy - уничтожить кластер
func (t *Terraform) terraformDestroy() error {
	destroyCmd := exec.Command("terraform", "destroy", "-force")
	destroyCmd.Dir = t.WorkDirectory

	stdout, err := destroyCmd.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdout pipe for logging destroy of cluster")
	}

	stdoutReader := bufio.NewReader(stdout)
	go handleReader(stdoutReader)

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

// terraformPrepare - заполнить конфиг провайдера (for example yandex_compute_instance_group.tf)
func (t *Terraform) terraformPrepare(templatesConfig TemplatesConfig, settings *config.DeploySettings) error {
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

	err := Prepare(templatesInit[2].CPU,
		templatesInit[3].RAM,
		templatesInit[4].Disk,
		templatesInit[1].Platform,
		settings.Nodes)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

func (t *Terraform) getIPMapping() (MapAddresses, error) {
	/*
		Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
		 своя пара - внешний (NAT) и внутренний ip.
		 Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
		  т.к. использование encoding/json
		влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
		по переданному тегу json, который можно преобразовать в том числе в строку.
	*/
	var mapIP MapAddresses
	tsStateWorkDir := fmt.Sprintf("%s/terraform.tfstate", t.WorkDirectory)
	data, err := ioutil.ReadFile(tsStateWorkDir)
	if err != nil {
		return mapIP, merry.Prepend(err, "failed to read file terraform.tfstate")
	}

	masterExternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterExternalIP := masterExternalIPArray.Get("attributes").Get("network_interface.0").Get("nat_ip_address")
	mapIP.masterExternalIP = masterExternalIP.Str

	masterInternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterInternalIP := masterInternalIPArray.Get("attributes").Get("network_interface.0").Get("ip_address")
	mapIP.masterInternalIP = masterInternalIP.Str

	metricsExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsExternalIP := metricsExternalIPArray.Get("instances.0").Get("network_interface.0").Get("nat_ip_address")
	mapIP.metricsExternalIP = metricsExternalIP.Str

	metricsInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsInternalIP := metricsInternalIPArray.Get("instances.0").Get("network_interface.0").Get("ip_address")
	mapIP.metricsInternalIP = metricsInternalIP.Str

	ingressExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressExternalIP := ingressExternalIPArray.Get("instances.1").Get("network_interface.0").Get("nat_ip_address")
	mapIP.ingressExternalIP = ingressExternalIP.Str

	ingressInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressInternalIP := ingressInternalIPArray.Get("instances.1").Get("network_interface.0").Get("ip_address")
	mapIP.ingressInternalIP = ingressInternalIP.Str

	postgresExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresExternalIP := postgresExternalIPArray.Get("instances.2").Get("network_interface.0").Get("nat_ip_address")
	mapIP.postgresExternalIP = postgresExternalIP.Str

	postgresInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresInternalIP := postgresInternalIPArray.Get("instances.2").Get("network_interface.0").Get("ip_address")
	mapIP.postgresInternalIP = postgresInternalIP.Str

	return mapIP, nil
}

// installTerraform - установить terraform, если не установлен
func (t *Terraform) installTerraform() error {
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
