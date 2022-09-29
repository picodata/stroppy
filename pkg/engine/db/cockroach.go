package db

import (
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry/v2"
)

const (
	cockroachWorkDir = "cockroach"

	cockroachClusterName       = "cockroachdb"
	cockroachClientName        = "cockroachdb-client-secure"
	cockroachPublicServiceName = "service/cockroachdb-public"
)

func createCockroachCluster(
	sshClient engineSsh.Client,
	kube *kubernetes.Kubernetes,
	shellState *state.State,
) Cluster {
	return &cockroachCluster{
		commonCluster: createCommonCluster(
			sshClient,
			kube,
			shellState,
		),
	}
}

type cockroachCluster struct {
	*commonCluster
}

func (cc *cockroachCluster) Connect() (interface{}, error) {
	return cluster.NewCockroachCluster(cc.DBUrl, cc.connectionPoolSize) //nolint
}

func (cc *cockroachCluster) Deploy(
	_ *kubernetes.Kubernetes,
	shellState *state.State,
) error {
	var err error

	if err = cc.deploy(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy cockroach cluster")
	}

	if err = cc.examineCluster(cockroachClusterName,
		kubeengine.ResourceDefaultNamespace,
		cockroachClientName,
		cockroachClusterName,
	); err != nil {
		return merry.Prepend(err, "failled to examine cockroach cluster")
	}

	if err = cc.openPortForwarding(cockroachPublicServiceName, []string{""}); err != nil {
		return merry.Prepend(err, "failed to forvard ports for cockroach")
	}

	return nil
}

func (cc *cockroachCluster) GetSpecification() ClusterSpec {
	return cc.clusterSpec
}
