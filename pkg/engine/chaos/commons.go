package chaos

import "gitlab.com/picodata/stroppy/pkg/engine/kubernetes"

type Controller interface {
	Deploy() error
	ExecuteCommand(string) error
	Stop()
}

func CreateController(k *kubernetes.Kubernetes, wd string, isChaosEnabled bool) (c Controller) {
	if isChaosEnabled {
		c = createWorkableController(k, wd)
	} else {
		c = createDummyChaos()
	}
	return
}
