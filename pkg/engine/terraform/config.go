package terraform

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

const installableTerraformVersion = "0.15.4"

const yandexProvider = "yandex"

const oracleProvider = "oracle"
