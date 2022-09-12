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

	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/internal/payload"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gitlab.com/picodata/stroppy/pkg/engine/chaos"
	"gitlab.com/picodata/stroppy/pkg/engine/db"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

func createPayload(shellState *state.State) (payload.Payload, error) {
	var (
		sshClient ssh.Client
		err       error
	)

	if sshClient, err = kubeengine.CreateSystemShell(shellState.Settings); err != nil {
		return nil, merry.Prepend(err, "failed to create system shell")
	}

	terraformProvider := terraform.CreateTerraform(
		shellState.Settings.DeploymentSettings,
		shellState.Settings.WorkingDirectory,
		shellState.Settings.WorkingDirectory,
	)
	if err = terraformProvider.InitProvider(); err != nil {
		return nil, merry.Prepend(err, "failed to init provider")
	}

	var kube *kubernetes.Kubernetes

	if kube, err = kubernetes.CreateKubernetes(sshClient, shellState); err != nil {
		return nil, merry.Prepend(err, "failed to create kubernetes")
	}

	var dbCluster db.Cluster

	if dbCluster, err = db.CreateCluster(sshClient, kube, shellState); err != nil {
		return nil, merry.Prepend(err, "failed to create database cluster")
	}

	chaosController := chaos.CreateController(kube.Engine, shellState)

	var dbPayload payload.Payload

	if dbPayload, err = payload.CreatePayload(
		dbCluster,
		shellState.Settings,
		chaosController,
	); err != nil {
		return nil, merry.Prepend(err, "failed to create payload")
	}

	return dbPayload, nil
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
	logFileDescriptor, err = os.OpenFile(filepath.Join(settings.WorkingDirectory, logFileName),
		os.O_CREATE|os.O_APPEND|os.O_RDWR,
		modePerm)
	if err != nil {
		err = merry.Prependf(
			err,
			"open log file '%s' in '%s' directory",
			logFileName,
			settings.WorkingDirectory,
		)
		return
	}

	llog.SetOutput(io.MultiWriter(os.Stdout, logFileDescriptor))
	return
}
