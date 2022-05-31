package db

import (
	"path/filepath"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"
)

type yandexCluster struct {
	*commonCluster
}
func createYandexCluster(
	sc engineSsh.Client,
	k *kubernetes.Kubernetes,
	wd string,
	dbURL string,
	connectionPoolSize int,
) (yandex Cluster) {
	yandex = &yandexCluster{
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

// Connect implements Cluster
func (*yandexCluster) Connect() (interface{}, error) {
	panic("unimplemented")
}

// Deploy implements Cluster
func (*yandexCluster) Deploy() error {
	panic("unimplemented")
}

// GetSpecification implements Cluster
func (*yandexCluster) GetSpecification() ClusterSpec {
	panic("unimplemented")
}


