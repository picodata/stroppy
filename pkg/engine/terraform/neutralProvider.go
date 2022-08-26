package terraform

import (
	"errors"

	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

func createNeutralProvider() (op *neutralProvider) {
	op = &neutralProvider{}
	return
}

type neutralProvider struct{}

//nolint
func (np *neutralProvider) Prepare() (err error) {
	err = errors.New(
		"neutral provider does not support deployment preparaion, please specify yandex or oracle",
	)
	return
}

//nolint
func (np *neutralProvider) AddNetworkDisks(_ int) (err error) {
	err = errors.New(
		"neutral provider does not support deployment additional step operation, use yandex or oracle",
	)
	return
}

//nolint
func (np *neutralProvider) GetAddressMap(
	_ int,
) (mapIPAddresses map[string]map[string]string, _ error) {
	mapIPAddresses = make(map[string]map[string]string)
	return
}

func (np *neutralProvider) IsPrivateKeyExist(_ string) bool {
	return true
}

func (np *neutralProvider) RemoveProviderSpecificFiles() {}

func (np *neutralProvider) SetTerraformStatusData(_ []byte) {}

func (np *neutralProvider) Name() string {
	return provider.Neutral
}

func (np *neutralProvider) GetDeploymentCommands() (firstStep, thirdStep string) {
	const neutralEcho = "echo neutral provider command"
	firstStep = neutralEcho
	thirdStep = neutralEcho
	return
}

func (np *neutralProvider) CheckSSHKeyFiles(workDir string) error {
	return nil
}
