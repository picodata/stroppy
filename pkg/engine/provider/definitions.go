package provider

import "errors"

const (
	Oracle = "oracle"
	Yandex = "yandex"

	ClusterTemplateFileName = "templates.yaml"
)

var ErrChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

type Provider interface {
	Prepare() error
	PerformAdditionalOps(int) error
	GetAddressMap(int) (map[string]map[string]string, error)
	IsPrivateKeyExist(string) bool
	RemoveProviderSpecificFiles()
	SetTerraformStatusData([]byte)
	GetDeploymentCommands() (string, string)
	Name() string
}

type ClusterParameters struct {
	Description string
	Platform    string
	CPU         int
	RAM         int
	Disk        int
}

type ClusterConfigurations struct {
	Small    ClusterParameters
	Standard ClusterParameters
	Large    ClusterParameters
	XLarge   ClusterParameters
	XXLarge  ClusterParameters
	Maximum  ClusterParameters
}
