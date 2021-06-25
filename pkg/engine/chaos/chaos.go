package chaos

import (
	"fmt"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
)

func CreateController(k *kubernetes.Kubernetes, wd string) (c *Controller) {
	c = &Controller{
		wd: filepath.Join(wd, "chaos"),
		k:  k,
	}
	return
}

type Controller struct {
	k  *kubernetes.Kubernetes
	wd string
}

func (chaos *Controller) Deploy() (err error) {
	llog.Infoln("Starting chaos-mesh deployment...")

	err = chaos.k.ExecuteCommand(deployChaosMesh)
	if err != nil {
		return merry.Prepend(err, "failed to deploy of chaos-mesh")
	}

	llog.Infoln("Finished of deploy chaos-mesh")
	return
}

func (chaos *Controller) ExecuteCommand(scenarioName string) (err error) {
	scenarioNameFileName := scenarioName + ".yaml"

	destinationFilePath := filepath.Join("/home/ubuntu", scenarioNameFileName)
	sourceFile := filepath.Join(chaos.wd, scenarioNameFileName)

	if err = chaos.k.LoadFile(sourceFile, destinationFilePath); err != nil {
		return
	}

	err = chaos.k.ExecuteCommand(fmt.Sprintf("kubectl apply -f %s", destinationFilePath))
	return
}
