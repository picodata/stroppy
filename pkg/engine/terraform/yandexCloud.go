package terraform

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
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
