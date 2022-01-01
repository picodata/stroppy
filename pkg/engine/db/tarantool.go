package db

import (
	"path/filepath"

	"github.com/ansel1/merry"
	clusterImplementation "gitlab.com/picodata/stroppy/pkg/database/cluster"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

func createTarantoolCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string, connectionPoolSize int, sharded bool) (tarantool Cluster) {
	tarantool = &tarantoolCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, mongoDirectory),
			mongoDirectory,
			dbURL,
			connectionPoolSize,
			sharded,
		),
	}
	return
}

type tarantoolCluster struct {
	*commonCluster
}

func (tarantool *tarantoolCluster) Connect() (cluster interface{}, err error) {
	// подключение к локально развернутому mongo без реплики
	if tarantool.DBUrl == "" {
		tarantool.DBUrl = "127.0.0.1:3301"
	}

	connectionPool := uint64(tarantool.commonCluster.connectionPoolSize) + uint64(tarantool.commonCluster.addPool)
	cluster, err = clusterImplementation.NewTarantoolCluster(tarantool.DBUrl, connectionPool, tarantool.commonCluster.sharded)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to  tarantool cluster")
	}
	return
}

func (tarantool *tarantoolCluster) Deploy() (err error) {
	return nil
}

func (tarantool *tarantoolCluster) GetSpecification() (spec ClusterSpec) {
	return
}