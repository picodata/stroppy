package db

import (
	"path/filepath"

	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

const (
	cockroachWorkingDir = "cockroach"

	cockroachClusterName       = "cockroachdb"
	cockroachClientName        = ""
	cockroachPublicServiceName = "service/cockroachdb-public"
)

func createCockroachCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string, connectionPoolSize int) (fc Cluster) {

	const operatorDeploymentParameters = "--insecure"
	fc = &cockroachCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, cockroachWorkingDir),
			cockroachWorkingDir,
			dbURL,
			operatorDeploymentParameters,
			connectionPoolSize,
			false,
		),
	}
	return
}

type cockroachCluster struct {
	*commonCluster
}

func (cc *cockroachCluster) Connect() (cluster interface{}, err error) {
	cluster, err = cluster2.NewCockroachCluster(cc.DBUrl, cc.connectionPoolSize)
	return
}

func (cc *cockroachCluster) Deploy() (err error) {
	if err = cc.deploy(); err != nil {
		return
	}

	err = cc.examineCluster(cockroachClusterName,
		kubeengine.ResourceDefaultNamespace,
		cockroachClientName,
		cockroachClusterName)
	if err != nil {
		return
	}

	if err = cc.openPortForwarding(cockroachPublicServiceName, []string{""}); err != nil {
		return
	}
	return
}

func (cc *cockroachCluster) GetSpecification() (spec ClusterSpec) {
	spec = cc.clusterSpec
	return
}
