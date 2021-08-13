package db

import (
	"os/exec"

	"gitlab.com/picodata/stroppy/pkg/database/config"

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

func CreateCluster(dbConfig *config.DatabaseSettings,
	sc ssh.Client, k *kubernetes.Kubernetes, wd string) (_cluster Cluster, err error) {

	switch dbConfig.DBType {
	default:
		err = merry.Errorf("unknown database type '%s'", dbConfig.DBType)

	case cluster.Postgres:
		_cluster = createPostgresCluster(sc, k, wd, dbConfig.DBURL)

	case cluster.Foundation:
		_cluster = createFoundationCluster(sc, k, wd, dbConfig.DBURL)
	}

	return
}
