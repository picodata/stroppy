/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"os/exec"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	v1 "k8s.io/api/core/v1"
)

type Status string

const dbWorkingDirectory = "databases"

type ClusterSpec struct {
	MainPod *v1.Pod
	Pods    []*v1.Pod
}

type Cluster interface {
	Deploy(*state.State) error
	GetSpecification() ClusterSpec
	Connect() (interface{}, error)
}

// ClusterTunnel
/* структура хранит результат открытия port-forward туннеля к кластеру:
 * Command - структура, которая хранит атрибуты команды, которая запустила туннель
 * Error - возможная ошибка при открытии туннеля
 * LocalPort - порт локальной машины для туннеля */
type ClusterTunnel struct {
	Command   *exec.Cmd
	Error     error
	LocalPort *int
}

func CreateCluster(
	sshClient ssh.Client,
	kube *kubernetes.Kubernetes,
	shellState *state.State,
) (Cluster, error) {
	var (
		dbcluster Cluster
		err       error
	)

	// если кол-во соединений не задано, приравниваем к кол-ву воркеров
	if shellState.Settings.DatabaseSettings.ConnectPoolSize == 0 {
		shellState.Settings.DatabaseSettings.ConnectPoolSize = shellState.
			Settings.DatabaseSettings.Workers
	}

	switch shellState.Settings.DatabaseSettings.DBType {
	default:
		err = merry.Errorf(
			"unknown database type '%s'",
			shellState.Settings.DatabaseSettings.DBType,
		)

	case cluster.Postgres:
		dbcluster = createPostgresCluster(
			sshClient,
			kube,
			shellState,
		)

	case cluster.Foundation:
		dbcluster = createFoundationCluster(
			sshClient,
			kube,
			shellState,
		)

	case cluster.MongoDB:
		dbcluster = createMongoCluster(
			sshClient,
			kube,
			shellState,
		)

	case cluster.Cockroach:
		dbcluster = createCockroachCluster(
			sshClient,
			kube,
			shellState,
		)

	case cluster.Cartridge:
		dbcluster = createCartridgeCluster(
			sshClient,
			kube,
			shellState,
		)

	case cluster.YandexDB:
		dbcluster = createYandexDBCluster(
			sshClient,
			kube,
			shellState,
		)
	}

	return dbcluster, err
}
