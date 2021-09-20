package db

import (
	"context"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	cluster2 "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"go.mongodb.org/mongo-driver/bson"
	mongoDriver "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	mongoDirectory = "mongodb"

	mongoOperatorName = "percona-server-mongodb-operator"
)

func createMongoCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, dbURL string, dbPool int, addPool int, sharded bool) (mongo Cluster) {
	mongo = &mongoCluster{
		commonCluster: createCommonCluster(
			sc,
			k,
			filepath.Join(wd, dbWorkingDirectory, mongoDirectory),
			mongoDirectory,
			dbURL,
			dbPool,
			addPool,
			sharded,
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

	connectionPool := uint64(mongo.commonCluster.dbPool) + uint64(mongo.commonCluster.addPool)
	cluster, err = cluster2.NewMongoDBCluster(mongo.DBUrl, connectionPool, mongo.commonCluster.sharded)
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
		kubernetes.ResourceDefaultNamespace,
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

func (mongo *mongoCluster) AddStroppyUser() error {
	var dbURL string

	if mongo.sharded {
		dbURL = "mongodb://userAdmin:userAdmin123456@my-cluster-name-mongos.default.svc.cluster.local/admin?ssl=false"
	} else {
		dbURL = "mongodb://userAdmin:userAdmin123456@my-cluster-name-rs0.default.svc.cluster.local/admin?replicaSet=rs0&ssl=false"
	}

	llog.Debugln("connecting to mongodb for add stroppy user...")
	client, err := mongoDriver.NewClient(options.Client().ApplyURI(dbURL))
	if err != nil {
		return merry.Prepend(err, "failed to create mongoDB client for add stroppy user")
	}

	err = client.Connect(context.TODO())
	if err != nil {
		return merry.Prepend(err, "failed to connect mongoDB database for add stroppy user")
	}

	// проверяем успешность соединения
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		return merry.Prepend(err, "failed to ping mongoDB database for add stroppy user")
	}

	llog.Infoln("Connected to MongoDB for add stroppy user: success")

	addUserCmd := bson.D{
		{Key: "createUser", Value: "admin"},
		{Key: "user", Value: "stroppy"},
		{Key: "pwd", Value: "stroppy"},
		{Key: "roles", Value: bson.A{"readWriteAnyDatabase", "dbAdminAnyDatabase", "clusterAdmin"}}}

	if singleResult := client.Database("admin").RunCommand(context.TODO(), addUserCmd); singleResult.Err() != nil {
		return merry.Prepend(singleResult.Err(), "failed to init sharding for stroppy db")
	}

	return nil
}
