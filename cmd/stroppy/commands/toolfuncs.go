package commands

import (
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
)

func createChaos(settings *config.Settings) (_chaos chaos.Controller) {
	k, err := kubernetes.CreateShell(settings)
	if err != nil {
		llog.Fatalf("failed to construct kubernetes: %v", err)
	}

	_chaos = chaos.CreateController(k, settings.WorkingDirectory, settings.UseChaos)
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
