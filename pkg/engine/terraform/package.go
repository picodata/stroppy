package terraform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
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
	addressMap := make(map[string]map[string]string)

	t = &Terraform{
		settings:          settings,
		exePath:           filepath.Join(exeFolder, "terraform"),
		templatesFilePath: filepath.Join(cfgFolder, templatesFileName),
		stateFilePath:     "",
		addressMap:        addressMap,
		isInit:            false,
		WorkDirectory:     cfgFolder,
	}
	t.stateFilePath = filepath.Join(t.WorkDirectory, stateFileName)

	return
}

// InitProvider - инициализировать провайдера в зависимости от настроек
func (t *Terraform) InitProvider(settings *config.DeploySettings) (err error) {
	switch settings.Provider {
	case yandexProvider:
		t.Provider, err = CreateYandexProvider(settings, t.WorkDirectory)
		if err != nil {
			return merry.Prepend(err, "failed to initialized yandex provider")
		}

	case oracleProvider:
		t.Provider, err = CreateOracleProvider(settings, t.WorkDirectory)
		if err != nil {
			return merry.Prepend(err, "failed to initialized oracle provider")
		}
	}

	return nil
}

type Terraform struct {
	settings *config.DeploySettings

	exePath           string
	templatesFilePath string
	stateFilePath     string

	addressMap map[string]map[string]string
	isInit     bool

	WorkDirectory string

	version *version

	Provider Provider
}

type version struct {
	major  int
	minor  int
	bugfix int
}

// GetAddressMap - получить структуру с адресами кластера
func (t *Terraform) GetAddressMap() (addressMap map[string]map[string]string, err error) {

	return t.Provider.GetAddressMap(t.stateFilePath, t.settings.Nodes)
}

func (t *Terraform) Run() (err error) {

	err = t.Provider.Prepare(t.WorkDirectory)
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
		destroyCmd = exec.Command("terraform", "apply", "-destroy", "--auto-approve")
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

func (t *Terraform) getTerraformVersion() (*version, error) {
	var installedVersion version
	getVersionCMD, err := exec.Command("terraform", "version").Output()
	if err != nil {
		if !errors.Is(err, exec.ErrNotFound) {
			return nil, merry.Wrap(err)
		}

		return nil, nil
	}

	// получаем из строки идентификатор версии в виде: v0.15.4 (как пример)
	searchExpressionString := regexp.MustCompile(`v[0-9]+.[0-9]+.[0-9]+`)
	installedVersionString := searchExpressionString.FindString(string(getVersionCMD))
	if len(installedVersionString) == 0 {
		return nil, errVersionParsed
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

	return &installedVersion, nil
}

// install
// установить terraform, если не установлен
func (t *Terraform) install() error {
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
	unzipArchiveCmd := exec.Command("unzip", "-u", archiveName)
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

	installCmd := exec.Command("bash", "-c", "sudo install terraform /usr/bin/terraform")
	llog.Infoln(installCmd.String())
	installCmd.Dir = t.WorkDirectory
	err = installCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to install terraform")
	}

	llog.Infoln("terrafrom installed: success")
	return nil
}

// init - подготовить среду для развертывания
func (t *Terraform) init() (err error) {
	llog.Infoln("Initializating terraform...")

	if t.version, err = t.getTerraformVersion(); err != nil {
		return merry.Prepend(err, "failed to get terraform version")
	}

	if t.version == nil {
		llog.Infoln("Terraform is not found. Preparing the installation terraform...")

		err = t.install()
		if err != nil {
			return merry.Prepend(err, "failed to install terraform")
		}
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
