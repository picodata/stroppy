/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"os/exec"

	"gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"

	"gitlab.com/picodata/stroppy/pkg/database/config"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	v1 "k8s.io/api/core/v1"
)

type Status string

const dbWorkingDirectory = "databases"

type ClusterSpec struct {
	MainPod *v1.Pod
	Pods    []*v1.Pod
}

type Cluster interface {
	Deploy() error
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

//nolint // for future refactoring
func CreateCluster(
	dbConfig *config.DatabaseSettings,
	sshClient ssh.Client,
	kube *kubernetes.Kubernetes,
	workDir string,
) (Cluster, error) {
	var (
		dbcluster Cluster
		err       error
	)

	// если кол-во соединений не задано, приравниваем к кол-ву воркеров
	if dbConfig.ConnectPoolSize == 0 {
		dbConfig.ConnectPoolSize = dbConfig.Workers
	}

	switch dbConfig.DBType {
	default:
		err = merry.Errorf("unknown database type '%s'", dbConfig.DBType)

	case cluster.Postgres:
		dbcluster = createPostgresCluster(
			sshClient,
			kube,
			workDir,
			dbConfig.DBURL,
			dbConfig.ConnectPoolSize,
		)

	case cluster.Foundation:
		dbcluster = createFoundationCluster(sshClient, kube, workDir, dbConfig.DBURL)

	case cluster.MongoDB:
		dbcluster = createMongoCluster(
			sshClient,
			kube,
			workDir,
			dbConfig.DBURL,
			dbConfig.ConnectPoolSize,
			dbConfig.Sharded,
		)

	case cluster.Cockroach:
		dbcluster = createCockroachCluster(
			sshClient,
			kube,
			workDir,
			dbConfig.DBURL,
			dbConfig.ConnectPoolSize,
		)

	case cluster.Cartridge:
		dbcluster = createCartridgeCluster(
			sshClient,
			kube,
			workDir,
			dbConfig.DBURL,
			dbConfig.ConnectPoolSize,
		)

	case cluster.YandexDB:
		dbcluster = createYandexDBCluster(
			sshClient,
			kube,
			workDir,
			dbConfig.DBURL,
			dbConfig.ConnectPoolSize,
		)
	}

	return dbcluster, err
}
