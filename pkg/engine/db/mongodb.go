package db

import (
	"path/filepath"

	"github.com/ansel1/merry"
	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"
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
	// подключение к локально развернутому mongo без реплики
	if mongo.DBUrl == "" {
		mongo.DBUrl = "mongodb://127.0.0.1:30001"
	}

	cluster, err = cluster2.NewMongoDBCluster(mongo.DBUrl, 64)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to  mongo cluster")
	}
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
