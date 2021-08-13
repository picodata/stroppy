package commands

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func createPayload(settings *config.Settings) (_payload payload.Payload) {
	k, err := kubernetes.CreateShell(settings)
	if err != nil {
		llog.Fatalf("failed to construct kubernetes: %v", err)
	}

	var sc engineSsh.Client
	if sc, err = kubernetes.CreateSystemShell(settings); err != nil {
		llog.Fatalf("create payload: %v", err)
	}

	var _cluster db.Cluster
	if _cluster, err = db.CreateCluster(settings.DatabaseSettings, sc, k, settings.WorkingDirectory); err != nil {
		llog.Fatalf("failed to create cluster: %v", err)
	}

	_chaos := chaos.CreateController(k, settings.WorkingDirectory, settings.UseChaos)
	if _payload, err = payload.CreatePayload(_cluster, settings, _chaos); err != nil {
		return
	}
	return
}

func initLogLevel(settings *config.Settings) (err error) {
	var l llog.Level
	if l, err = llog.ParseLevel(settings.LogLevel); err != nil {
		return merry.Prependf(err, "'%s' log level parse", settings.LogLevel)
	}
	llog.SetLevel(l)

	return
}
