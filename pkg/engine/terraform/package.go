package terraform

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gopkg.in/yaml.v2"
)

// размер ответа terraform show при незапущенном кластере
const linesNotInitTerraformShow = 13

const (
	stateFileName     = "terraform.tfstate"
	templatesFileName = "templates.yaml"
)

var (
	errChooseConfig   = errors.New("failed to choose configuration. Unexpected configuration cluster template")
	errVersionParsed  = errors.New("failed to parse version")
	errChooseProvider = errors.New("failed to choose provider. Unexpected provider's name")
)

func CreateTerraform(settings *config.DeploySettings, exeFolder, cfgFolder string) (t *Terraform) {
	t = &Terraform{
		settings:          settings,
		exePath:           filepath.Join(exeFolder, "terraform"),
		templatesFilePath: filepath.Join(cfgFolder, templatesFileName),
		stateFilePath:     "",
		addressMap:        &MapAddresses{},
		isInit:            false,
		WorkDirectory:     cfgFolder,
	}
	t.stateFilePath = filepath.Join(t.WorkDirectory, stateFileName)

	return
}

type MapAddresses struct {
	MasterExternalIP   string
	MasterInternalIP   string
	MetricsExternalIP  string
	MetricsInternalIP  string
	IngressExternalIP  string
	IngressInternalIP  string
	DatabaseExternalIP string
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

	version version
}

type TemplatesConfig struct {
	Yandex Configurations
	Oracle Configurations
}

type version struct {
	major  int
	minor  int
	bugfix int
}

type Provider interface {
	Prepare()
	PerformAdditionalOps()
}

// GetAddressMap - получить структуру с адресами кластера
func (t *Terraform) GetAddressMap() (addressMap MapAddresses, err error) {
	var _map *MapAddresses
	if _map, err = t.collectInternalExternalAddressMap(); err != nil {
		return
	}

	t.addressMap = _map

	addressMap = *t.addressMap
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

	llog.Printf("Terraform applied\n")
	return
}

// nolint
func (t *Terraform) deleteTfstateFiles() {
	files2BeDeleted := []string{
		stateFileName,
		".terraform.lock.hcl",
	}

	var err error
	for _, file := range files2BeDeleted {
		path := filepath.Join(t.WorkDirectory, file)
		if err = os.Remove(path); err != nil {
			llog.Warnf("delete terraform service files: %v", merry.Prepend(err, path))
		}
	}
}

// Destroy - уничтожить кластер
func (t *Terraform) Destroy() error {
	var destroyCmd *exec.Cmd
	// https://github.com/hashicorp/terraform/releases/tag/v0.15.2
	if t.version.major == 0 {
		if t.version.minor <= 15 {
			if t.version.bugfix < 2 {
				destroyCmd = exec.Command("terraform", "destroy", "-force")
			}
		}
	} else {
		destroyCmd = exec.Command("terraform", "apply", "-destroy")
	}
	destroyCmd.Dir = t.WorkDirectory

	// нужно для успешной передачи yes в команду destroy при версии > 0.15.2
	destroyCmd.Stdout = os.Stdout
	destroyCmd.Stderr = os.Stdout
	destroyCmd.Stdin = os.Stdin

	llog.Infoln("Destroying terraform...")
	if err := destroyCmd.Run(); err != nil {
		return merry.Wrap(err)
	}

	llog.Infoln("Terraform destroyed")

	// \todo: Проработать удаление с учетом того, что приложение statefull в очень широком смысле
	// t.deleteTfstateFiles()
	return nil
}

func (t *Terraform) readTemplates() (*TemplatesConfig, error) {
	var templatesConfig TemplatesConfig
	data, err := ioutil.ReadFile(t.templatesFilePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yaml")
	}

	err = yaml.Unmarshal(data, &templatesConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yaml")
	}

	return &templatesConfig, nil
}

func (t *Terraform) getTerraformVersion() (version, error) {
	var installedVersion version
	getVersionCMD, err := exec.Command("terraform", "version").Output()
	if err != nil {
		return version{}, merry.Wrap(err)
	}

	// получаем из строки идентификатор версии в виде: v0.15.4 (как пример)
	searchExpressionString := regexp.MustCompile(`v[0-9]+.[0-9]+.[0-9]+`)
	installedVersionString := searchExpressionString.FindString(string(getVersionCMD))
	if len(installedVersionString) == 0 {
		return version{}, errVersionParsed
	}

	versionArray := strings.Split(installedVersionString, ".")

	major, _ := strconv.Atoi(versionArray[0])
	minor, _ := strconv.Atoi(versionArray[1])
	bugfix, _ := strconv.Atoi(versionArray[2])

	installedVersion = version{
		major:  major,
		minor:  minor,
		bugfix: bugfix,
	}

	return installedVersion, nil
}

func getCPUCount(templateConfig []ConfigurationUnitParams) int {
	cpuCount := templateConfig[2].CPU
	return cpuCount
}

func getRAMSize(templateConfig []ConfigurationUnitParams) int {
	ramSize := templateConfig[3].RAM
	return ramSize
}

func getDiskSize(templateConfig []ConfigurationUnitParams) int {
	diskSize := templateConfig[4].Disk
	return diskSize
}

func getPlatform(templateConfig []ConfigurationUnitParams) string {
	platform := templateConfig[1].Platform
	return platform
}

// prepare
// создать конфигурационный файл провайдера
func (t *Terraform) prepare(templatesConfig TemplatesConfig, settings *config.DeploySettings) error {
	var templatesInit []ConfigurationUnitParams

	switch settings.Provider {
	case "yandex":
		switch settings.Flavor {
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

	case "oracle":
		switch settings.Flavor {
		case "small":
			templatesInit = templatesConfig.Oracle.Small
		case "standard":
			templatesInit = templatesConfig.Oracle.Standard
		case "large":
			templatesInit = templatesConfig.Oracle.Large
		case "xlarge":
			templatesInit = templatesConfig.Oracle.Xlarge
		case "xxlarge":
			templatesInit = templatesConfig.Oracle.Xxlarge
		default:
			return merry.Wrap(errChooseConfig)
		}
	default:
		return merry.Wrap(errChooseProvider)
	}

	cpuCount := getCPUCount(templatesInit)

	ramSize := getRAMSize(templatesInit)

	diskSize := getDiskSize(templatesInit)

	platform := getPlatform(templatesInit)

	switch settings.Provider {
	default:
		return merry.Errorf("unknown provider '%s'", settings.Provider)

	case "yandex":
		err := PrepareYandex(cpuCount, ramSize,
			diskSize, settings.Nodes, platform, t.WorkDirectory)
		if err != nil {
			return merry.Wrap(err)
		}

	case "oracle":
		err := PrepareOracle(cpuCount, ramSize,
			diskSize, settings.Nodes, t.WorkDirectory)
		if err != nil {
			return merry.Wrap(err)
		}
	}

	return nil
}

func (t *Terraform) collectInternalExternalAddressMap() (mapIP *MapAddresses, err error) {
	if !t.isInit {
		err = errors.New("terraform not init")
		return
	}

	mapIP, err = getAddressMap(t.stateFilePath, t.settings.Provider)
	return
}

// install
// установить terraform, если не установлен
func (t *Terraform) install() error {
	llog.Infoln("Terraform is not found. Preparing the installation terraform...")

	downloadURL := fmt.Sprintf("https://releases.hashicorp.com/terraform/%v/terraform_%v_linux_amd64.zip",
		installableTerraformVersion, installableTerraformVersion)
	downloadArchiveCmd := exec.Command("curl", "-O",
		downloadURL)
	downloadArchiveCmd.Dir = t.WorkDirectory
	err := downloadArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to download archive of terraform")
	}

	archiveName := fmt.Sprintf("terraform_%v_linux_amd64.zip", installableTerraformVersion)
	unzipArchiveCmd := exec.Command("unzip", archiveName)
	llog.Infoln(unzipArchiveCmd.String())
	unzipArchiveCmd.Dir = t.WorkDirectory
	err = unzipArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to unzip archive of terraform")
	}

	rmArchiveCmd := exec.Command("rm", archiveName)
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
func (t *Terraform) init() (err error) {
	llog.Infoln("Initializating terraform...")

	if t.version, err = t.getTerraformVersion(); err != nil {
		return merry.Prepend(err, "failed to get terraform version")
	}

	initCmd := exec.Command("terraform", "init")
	initCmd.Dir = t.WorkDirectory
	initCmdResult, err := initCmd.CombinedOutput()
	if err != nil {
		// вместо exit code из err возвращаем стандартный вывод, чтобы сразу видеть ошибку
		return merry.Errorf("terraform init '%s' command return error: %v", string(initCmdResult), err)
	}

	t.isInit = true
	llog.Infoln("Terraform initialized")
	return nil
}
