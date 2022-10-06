package terraform

import (
	"errors"

	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

var (
	errPrepare = errors.New(
		"neutral provider does not support deployment preparaion, please specify yandex or oracle",
	)
	errNetworkLabel = errors.New(
		"neutral provider does not support deployment additional step operation, use yandex or oracle",
	)
)

func createNeutralProvider() *neutralProvider {
	return &neutralProvider{}
}

type neutralProvider struct{}

func (np *neutralProvider) Prepare() error {
	return errPrepare
}

func (np *neutralProvider) AddNetworkDisks(_ int) (err error) {
	return errNetworkLabel
}

func (np *neutralProvider) GetAddressMap(
	_ int,
) (mapIPAddresses map[string]map[string]string, _ error) {
	mapIPAddresses = make(map[string]map[string]string)
	return
}

func (np *neutralProvider) GetInstancesAddresses() *provider.InstanceAddresses {
	return &provider.InstanceAddresses{
		Masters: make(map[string]provider.AddrPair),
		Workers: make(map[string]provider.AddrPair),
	}
}

func (np *neutralProvider) GetSubnet() string {
	return ""
}

func (np *neutralProvider) GetNodesInfo() map[string]*provider.NodeParams {
	return make(map[string]*provider.NodeParams)
}

func (np *neutralProvider) GetInternalSubnet() (string, error) {
	return "", nil
}

func (np *neutralProvider) IsPrivateKeyExist(_ string) bool {
	return true
}

func (np *neutralProvider) RemoveProviderSpecificFiles() {}

func (np *neutralProvider) Name() string {
	return provider.Neutral
}

func (np *neutralProvider) GetDeploymentCommands() (firstStep, thirdStep string) {
	const neutralEcho = "echo neutral provider command"
	firstStep = neutralEcho
	thirdStep = neutralEcho
	return
}

func (np *neutralProvider) CheckSSHPrivateKey(workDir string) error {
	return nil
}

func (np *neutralProvider) CheckSSHPublicKey(workDir string) error {
	return nil
}

func (np *neutralProvider) GetTfStateScheme() interface{} {
	return nil
}
