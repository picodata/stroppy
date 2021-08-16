package commands

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

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

func initLogFacility(settings *config.Settings) (err error) {
	formatter := new(llog.TextFormatter)

	// Stackoverflow wisdom
	formatter.TimestampFormat = "Jan _2 15:04:05.000"
	formatter.FullTimestamp = true
	formatter.ForceColors = true
	llog.SetFormatter(formatter)

	var l llog.Level
	if l, err = llog.ParseLevel(settings.LogLevel); err != nil {
		return merry.Prependf(err, "'%s' log level parse", settings.LogLevel)
	}
	llog.SetLevel(l)

	if len(os.Args) < 2 {
		err = fmt.Errorf("not enought arguments")
		return
	}

	startDateTime := time.Now().Format(time.RFC3339)
	// startDateTime := time.Now().Format("2009-11-10_23-00-00")
	logFileName := fmt.Sprintf("%s_test_run_%s.log", os.Args[1], startDateTime)

	var logFileDescriptor *os.File
	logFileDescriptor, err = os.OpenFile(filepath.Join(settings.WorkingDirectory, logFileName),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY,
		fs.ModeAppend)
	if err != nil {
		err = merry.Prependf(err, "open log file '%s' in '%s' directory", logFileName, settings.WorkingDirectory)
		return
	}

	llog.SetOutput(io.MultiWriter(os.Stdout, logFileDescriptor))
	return
}
