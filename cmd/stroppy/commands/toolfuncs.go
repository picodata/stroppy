/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package commands

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

func createPayload(settings *config.Settings) (_payload payload.Payload) {
	sc, err := kubeengine.CreateSystemShell(settings)
	if err != nil {
		llog.Fatalf("create payload: %v", err)
	}

	tf := terraform.CreateTerraform(settings.DeploymentSettings, settings.WorkingDirectory, settings.WorkingDirectory)
	if err = tf.InitProvider(); err != nil {
		llog.Fatalf("provider init failed: %v", err)
	}

	var addressMap map[string]map[string]string

	if addressMap, err = tf.GetAddressMap(); err != nil {
		llog.Fatalf("failed to get address map: %v", err)
	}

	var k *kubernetes.Kubernetes

	k, err = kubernetes.CreateKubernetes(settings, tf.Provider, addressMap, sc)
	if err != nil {
		llog.Fatalf("init kubernetes failed")
	}

	var _cluster db.Cluster

	if _cluster, err = db.CreateCluster(settings.DatabaseSettings, sc, k, settings.WorkingDirectory); err != nil {
		llog.Fatalf("failed to create cluster: %v", err)
	}

	_chaos := chaos.CreateController(k.Engine, settings.WorkingDirectory, settings.UseChaos)
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
		err = fmt.Errorf("not enough arguments")

		return
	}

	startDateTime := time.Now().Format(time.RFC3339)
	// startDateTime := time.Now().Format("2009-11-10_23-00-00")
	logFileName := fmt.Sprintf("%s_test_run_%s.log", os.Args[1], startDateTime)

	var logFileDescriptor *os.File
	// по умолчанию файл создается без прав вообще, нужны права на чтение
	var modePerm fs.FileMode = 444

	logFileDescriptor, err = os.OpenFile(filepath.Join(settings.WorkingDirectory, logFileName), os.O_CREATE|os.O_APPEND|os.O_RDWR, modePerm)
	if err != nil {
		err = merry.Prependf(err, "open log file '%s' in '%s' directory", logFileName, settings.WorkingDirectory)

		return
	}

	llog.SetOutput(io.MultiWriter(os.Stdout, logFileDescriptor))

	return
}
