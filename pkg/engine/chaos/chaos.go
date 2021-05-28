package chaos

import (
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func CreateController(sc *engineSsh.Client, k *kubernetes.Kubernetes) (c *Controller) {
	c = &Controller{
		sc: sc,
		k:  k,
	}
	return
}

type Controller struct {
	sc *engineSsh.Client
	k  *kubernetes.Kubernetes
}

func (chaos *Controller) Deploy() (err error) {
	err = chaos.k.ExecuteCommand(deployChaosMesh)
	return
}
