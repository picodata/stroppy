package db

import (
	clusterImplementation "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
)

const (
	tarantoolCartridgeOperatorName = "tarantool-operator"
	cartridgeCheckPodName          = "storage"
	cartridgeDirectory             = "cartridge"
)

func createCartridgeCluster(
	sshClient engineSsh.Client,
	kube *kubernetes.Kubernetes,
	shellState *state.State,
) Cluster {
	return &cartridgeCluster{
		commonCluster: createCommonCluster(
			sshClient,
			kube,
			shellState,
		),
	}
}

type cartridgeCluster struct {
	*commonCluster
}

func (cartridge *cartridgeCluster) Connect() (cluster interface{}, err error) {
	// подключение к локально развернутому mongo без реплики
	if cartridge.DBUrl == "" {
		cartridge.DBUrl = "127.0.0.1:8081"
	}

	connectionPool := uint64(
		cartridge.commonCluster.connectionPoolSize,
	) + uint64(
		cartridge.commonCluster.addPool,
	)
	cluster, err = clusterImplementation.NewCartridgeCluster(
		cartridge.DBUrl,
		connectionPool,
	)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to cartridge cluster")
	}
	return
}

func (cartridge *cartridgeCluster) Deploy(
	_ *kubernetes.Kubernetes,
	shellState *state.State,
) error {
	var err error

	if err = cartridge.deploy(shellState); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	err = cartridge.examineCluster("cartridge",
		kubeengine.ResourceDefaultNamespace,
		tarantoolCartridgeOperatorName,
		cartridgeCheckPodName)
	if err != nil {
		return merry.Prepend(err, "failed to examine cartridge cluster")
	}

	return nil
}

func (cartridge *cartridgeCluster) GetSpecification() (spec ClusterSpec) {
	return
}
