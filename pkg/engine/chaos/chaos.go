package chaos

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
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
	llog.Infoln("Starting of deploy chaos-mesh...")
	err = chaos.k.ExecuteCommand(deployChaosMesh)
	if err != nil {
		return merry.Prepend(err, "failed to deploy of chaos-mesh")
	}
	llog.Infoln("Finished of deploy chaos-mesh")

	return
}
