package db

import (
	"os/exec"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"

	v1 "k8s.io/api/core/v1"
)

type Status string

const ExecTimeout = 20

type ClusterSpec struct {
	MainPod *v1.Pod
	Pods    []*v1.Pod
}

type Cluster interface {
	Deploy() error
	GetSpecification() ClusterSpec
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

func CreateCluster(dbtype string, sc ssh.Client, k *kubernetes.Kubernetes, wd string) (_cluster Cluster, err error) {
	switch dbtype {
	default:
		err = merry.Errorf("unknown database type '%s'", dbtype)

	case cluster.Postgres:
		_cluster = createPostgresCluster(sc, k, wd)

	case cluster.Foundation:
		_cluster = createFoundationCluster(sc, k, wd)
	}

	return
}
