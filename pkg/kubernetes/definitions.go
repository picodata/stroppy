package kubernetes

import (
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
)

func CreateKubernetes(settings *config.Settings,
	provider provider.Provider,
	terraformAddressMap map[string]map[string]string,
	sshClient ssh.Client) (k *Kubernetes, err error) {

	k = &Kubernetes{
		provider: provider,
	}

	k.Engine, err = kubeengine.CreateEngine(settings, terraformAddressMap, sshClient)
	return
}

type Kubernetes struct {
	Engine   *kubeengine.Engine
	provider provider.Provider

	StroppyPod *stroppy.Pod

	sshTunnel   *ssh.Result
	portForward *ssh.Result
}
