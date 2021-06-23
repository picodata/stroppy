package commands

import (
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
)

func createChaos(settings *config.Settings) (_chaos *chaos.Controller) {
	addressMap, err := terraform.GetAddressMap(settings.WorkingDirectory, settings.DeploySettings.Provider)
	if err != nil {
		llog.Fatalf("failed to collect terraform address map: %v", err)
	}

	var sc engineSsh.Client
	sc, err = engineSsh.CreateClient(settings.WorkingDirectory,
		addressMap.MasterExternalIP,
		settings.DeploySettings.Provider,
		settings.Local)
	if err != nil {
		llog.Fatalf("failed to setup ssh tunnel: %v", err)
	}

	var k *kubernetes.Kubernetes
	if k, err = kubernetes.CreateKubernetes(settings, *addressMap, sc); err != nil {
		llog.Fatalf("failed to construct kubernetes: %v", err)
	}

	_chaos = chaos.CreateController(k, settings.WorkingDirectory)
	return
}
