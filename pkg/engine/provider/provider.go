package provider

import (
	"errors"
	"io/ioutil"

	"github.com/ansel1/merry"
	"gopkg.in/yaml.v2"
)

type Provider interface {
	Prepare(string) error
	PerformAdditionalOps() error
}

type TemplatesConfig struct {
	Yandex Configurations
	Oracle Configurations
}

type ConfigurationUnitParams struct {
	Description string
	Platform    string
	CPU         int
	RAM         int
	Disk        int
}

type Configurations struct {
	Small    []ConfigurationUnitParams
	Standard []ConfigurationUnitParams
	Large    []ConfigurationUnitParams
	Xlarge   []ConfigurationUnitParams
	Xxlarge  []ConfigurationUnitParams
	Maximum  []ConfigurationUnitParams
}

const TemplatesFileName = "templates.yaml"

var ErrChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

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

func ReadTemplates() (*TemplatesConfig, error) {
	var templatesConfig TemplatesConfig
	data, err := ioutil.ReadFile(TemplatesFileName)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yaml")
	}

	err = yaml.Unmarshal(data, &templatesConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yaml")
	}

	return &templatesConfig, nil
}
