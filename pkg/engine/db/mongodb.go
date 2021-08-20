package db

import (
	"path/filepath"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

const (
	mongoDirectory = "mongodb"

	mongoOperatorName     = "mongodb-kubernetes-operator"
	mongoClusterNamespace = "mongodbcommunity"
)

func createMongoCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string) (mongo Cluster) {
	mongo = &mongoCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, mongoDirectory),
			mongoDirectory,
			dbURL,
		),
	}
	return
}

type mongoCluster struct {
	*commonCluster
}

func (mongo *mongoCluster) Connect() (cluster interface{}, err error) {
	return
}

func (mongo *mongoCluster) Deploy() (err error) {
	if err = mongo.deploy(); err != nil {
		return merry.Prepend(err, "base deployment step")
	}

	err = mongo.examineCluster("MongoDB",
		mongoClusterNamespace,
		mongoOperatorName,
		"")
	if err != nil {
		return
	}

	return
}

func (mongo *mongoCluster) GetSpecification() (spec ClusterSpec) {
	return
}
