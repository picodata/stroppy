package db

import (
	"path/filepath"

	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

const (
	cockroachWorkDir = "cockroach"

	cockroachClusterName       = "cockroachdb"
	cockroachClientName        = "cockroachdb-client-secure"
	cockroachPublicServiceName = "service/cockroachdb-public"
)

func createCockroachCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string) (fc Cluster) {
	fc = &cockroachCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, cockroachWorkDir),
			cockroachWorkDir,
			dbURL,
			0,
			0,
			false,
		),
	}
	return
}

type cockroachCluster struct {
	*commonCluster
}

func (cc *cockroachCluster) Connect() (cluster interface{}, err error) {
	cluster, err = cluster2.NewCocroachCluster(cc.DBUrl)
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
