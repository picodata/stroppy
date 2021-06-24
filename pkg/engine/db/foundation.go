package db

import (
	"path/filepath"

	"github.com/ansel1/merry"

	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const (
	foundationDbDirectory = "foundationdb"
	foundationClusterName = "sample-cluster"
)

func CreateFoundationCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (fc Cluster) {
	fc = &foundationCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, foundationDbDirectory),
			foundationDbDirectory,
		),
	}
	return
}

type foundationCluster struct {
	*commonCluster
}

func (fc *foundationCluster) Deploy() (err error) {
	if err = fc.deploy(); err != nil {
		return merry.Prepend(err, "deploy failed")
	}

	var session engineSsh.Session
	if session, err = fc.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "fix_client_version session")
	}
	if _, err = session.CombinedOutput("chmod +x foundationdb/fix_client_version.sh && ./foundationdb/fix_client_version.sh"); err != nil {
		return merry.Prepend(err, "fix_client_version.sh failed")
	}

	return
}

func (fc *foundationCluster) OpenPortForwarding() (err error) {
	err = fc.openPortForwarding(foundationClusterName, []string{":"})
	return
}

func (fc *foundationCluster) GetStatus() (err error) {
	// already returns success
	return
}
