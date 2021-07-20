package terraform

import (
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Provider interface {
	Prepare(string) error
	PerformAdditionalOps(int, string, MapAddresses, string) error
}

func GetCPUCount(templateConfig []ConfigurationUnitParams) int {
	cpuCount := templateConfig[2].CPU
	return cpuCount
}

func GetRAMSize(templateConfig []ConfigurationUnitParams) int {
	ramSize := templateConfig[3].RAM
	return ramSize
}

func GetDiskSize(templateConfig []ConfigurationUnitParams) int {
	diskSize := templateConfig[4].Disk
	return diskSize
}

func GetPlatform(templateConfig []ConfigurationUnitParams) string {
	platform := templateConfig[1].Platform
	return platform
}

func ReadTemplates(wd string) (*TemplatesConfig, error) {
	var templatesConfig TemplatesConfig
	TemplatesFileNamePath := filepath.Join(wd, TemplatesFileName)
	data, err := ioutil.ReadFile(TemplatesFileNamePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yaml")
	}

	err = yaml.Unmarshal(data, &templatesConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yaml")
	}

	llog.Traceln("reading templates.yaml: success")

	return &templatesConfig, nil
}
