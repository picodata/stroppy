package db

import (
	"path/filepath"

	"github.com/ansel1/merry"
	clusterImplementation "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

const (
	tarantoolCartridgeOperatorName = "tarantool-operator"
	cartridgeCheckPodName          = "storage"
	cartridgeDirectory             = "cartridge"
)

func createCartridgeCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string, connectionPoolSize int) (cartridge Cluster) {
	cartridge = &cartridgeCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, cartridgeDirectory),
			cartridgeDirectory,
			dbURL,
			connectionPoolSize,
			false,
		),
	}
	return
}

type cartridgeCluster struct {
	*commonCluster
}

func (cartridge *cartridgeCluster) Connect() (cluster interface{}, err error) {
	// подключение к локально развернутому mongo без реплики
	if cartridge.DBUrl == "" {
		cartridge.DBUrl = "127.0.0.1:8081"
	}

	connectionPool := uint64(cartridge.commonCluster.connectionPoolSize) + uint64(cartridge.commonCluster.addPool)
	cluster, err = clusterImplementation.NewCartridgeCluster(cartridge.DBUrl, connectionPool, cartridge.commonCluster.sharded)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to cartridge cluster")
	}
	return
}

func (cartridge *cartridgeCluster) Deploy() (err error) {
	if err = cartridge.deploy(); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	err = cartridge.examineCluster("cartridge",
		kubeengine.ResourceDefaultNamespace,
		tarantoolCartridgeOperatorName,
		cartridgeCheckPodName)
	if err != nil {
		return
	}
	return nil
}

func (cartridge *cartridgeCluster) GetSpecification() (spec ClusterSpec) {
	return
}
