package provider

import (
	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

func CreateYandexProvider(settings *config.DeploySettings) (yp *YandexProvider, err error) {
	templatesConfig, err := provider.ReadTemplates()
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates for create provider")
	}

	yp.templatesConfig = *templatesConfig

	yp.settings = settings

	return yp, nil
}

type YandexProvider struct {
	templatesConfig provider.TemplatesConfig
	settings        *config.DeploySettings
}

func (yp *YandexProvider) Prepare(workingDirectory string) error {
	var templatesInit []provider.ConfigurationUnitParams

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
		return merry.Wrap(provider.ErrChooseConfig)
	}

	cpuCount := provider.GetCPUCount(templatesInit)

	ramSize := provider.GetRAMSize(templatesInit)

	diskSize := provider.GetDiskSize(templatesInit)

	platform := provider.GetPlatform(templatesInit)

	err := provider.PrepareYandex(cpuCount, ramSize,
		diskSize, yp.settings.Nodes, platform, workingDirectory)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}
