package foundationdb

import (
	"path/filepath"

	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const foundationDbDirectory = "foundationdb"

func CreateCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (fc *Cluster) {
	fc = &Cluster{
		k:  k,
		sc: sc,
		wd: filepath.Join(wd, foundationDbDirectory),
	}
	return
}

type Cluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
}

func (fc *Cluster) Deploy() (err error) {
	fdbClusterDeploymentDirectoryPath := fc.wd
	if err = fc.k.LoadFile(fdbClusterDeploymentDirectoryPath, "/home/ubuntu/foundationdb"); err != nil {
		return
	}
	llog.Infoln("copying cluster_with_client.yaml: success")

	return
}
