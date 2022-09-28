/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/pkg/kubernetes"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	clusterImplementation "gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	v1 "k8s.io/api/core/v1"
)

const (
	mongoOperatorName = "percona-server-mongodb-operator"
	mongoClusterName  = "sample-cluster"
	mongoDefaultDBUrl = "mongodb://stroppy:stroppy@127.0.0.1:27017;127.0.0.1:" +
		"27017;127.0.0.1:27017/admin?ssl=false"
)

func createMongoCluster(
	sshClient engineSsh.Client,
	kube *kubernetes.Kubernetes,
	shellState *state.State,
) Cluster {
	return &mongoCluster{
		commonCluster: createCommonCluster(
			sshClient,
			kube,
			shellState,
		),
	}
}

type mongoCluster struct {
	*commonCluster
}

func (mongo *mongoCluster) Connect() (cluster interface{}, err error) {
	// подключение к локально развернутому mongo без реплики
	if mongo.DBUrl == "" {
		mongo.DBUrl = mongoDefaultDBUrl
	}

	cluster, err = clusterImplementation.NewMongoDBCluster(
		mongo.DBUrl,
		uint64(mongo.connectionPoolSize),
		mongo.commonCluster.sharded,
		false,
	)
	if err != nil {
		return nil, merry.Prepend(err, "failed to init connect to  mongo cluster")
	}
	return
}

func (mongo *mongoCluster) Deploy(shellState *state.State) error {
	var err error

	if err = mongo.deploy(shellState); err != nil {
		return merry.Prepend(err, "base deployment step")
	}

	llog.Infof("Waiting 5 minutes for mongo deploy...")
	// за 5 минут укладывается развертывание кластера из трех шардов
	time.Sleep(5 * time.Minute)

	if err = mongo.examineCluster("MongoDB",
		kubeengine.ResourceDefaultNamespace,
		mongoOperatorName,
		mongoClusterName,
	); err != nil {
		return merry.Prepend(err, "failed to exeamine mongodb cluster")
	}

	var portForwardPodName *v1.Pod
	// выбираем либо балансер, либо мастер-реплику, в зависимости от конфигурации БД
	for i := range mongo.clusterSpec.Pods {
		switch {
		case mongo.sharded:
			if strings.Contains(mongo.clusterSpec.Pods[i].Name, "mongos") {
				portForwardPodName = mongo.clusterSpec.Pods[i]
			}
		default:
			if strings.Contains(mongo.clusterSpec.Pods[i].Name, "rs0") {
				portForwardPodName = mongo.clusterSpec.Pods[i]
			}
		}
	}

	if portForwardPodName == nil {
		return merry.Errorf("pod for port-forward mongodb no found")
	}

	llog.Debugln("Opening port-forward to pod ", portForwardPodName.Name, "for mongo")
	if err = mongo.openPortForwarding(portForwardPodName.Name, []string{"27017:27017"}); err != nil {
		return merry.Prepend(err, "failed to open port forward for mongo")
	}

	if err = mongo.addStroppyUser(portForwardPodName.Name, shellState); err != nil {
		return merry.Prepend(err, "failed to add stroppy user")
	}

	return nil
}

func (mongo *mongoCluster) GetSpecification() (spec ClusterSpec) {
	return
}

// addStroppyUser - добавить пользователя с необходимыми правами для выполнения тестов
func (mongo *mongoCluster) addStroppyUser(executePodName string, shellState *state.State) error {
	success := false
	var podName string
	// техдолг - заменить имя и пароль на данные из secrets. Нужен отдельный метод.
	createUserCmd := []string{
		"mongo",
		"-u", "userAdmin",
		"-p", "userAdmin123456",
		"--authenticationDatabase", "admin",
		"--eval",
		`db = db.getSiblingDB('admin');
db.createUser({user: "stroppy",pwd: "stroppy",roles: [ {role:"readWriteAnyDatabase", db:"admin"}, {role:"dbAdminAnyDatabase", db:"admin"},{role:"clusterAdmin", db:"admin"} ]})`,
	}

	// проходим по всем, т.к. узнавать, кто из ним мастер - долго и дорого, а для mongos должно сработать на первом
	for i := 0; i < 2; i++ {
		if mongo.sharded {
			podNameTemplate := strings.Split(executePodName, "-")
			podName = fmt.Sprintf(
				"%v-%v-%v-%v-%v",
				podNameTemplate[0],
				podNameTemplate[1],
				podNameTemplate[2],
				podNameTemplate[3],
				i,
			)
		} else {
			podName = executePodName
		}
		llog.Debugf("execute command to pod %v", podName)

		if _, _, err := mongo.k.ExecuteRemoteCommand(
			podName,
			"mongod",
			createUserCmd,
			"addStroppyUser.log",
			shellState,
		); err != nil {
			llog.Errorln(merry.Errorf("failed to add stroppy user to mongo: %v, try %v", err, i))

			// читаем файл с результатом выполнения, чтобы проверить ошибку внутри mongo shell
			resultFilePath := filepath.Join(
				shellState.Settings.WorkingDirectory,
				"addStroppyUser.log",
			)

			result, err := ioutil.ReadFile(resultFilePath)
			if err != nil {
				return merry.Prepend(err, "failed to analyze add stroppy user error")
			}

			// если пользователь уже есть, выходим
			if strings.Contains(string(result), "already exists") {
				success = true

				break
			}

			continue
		}
		success = true

		break
	}

	if !success {
		return merry.Errorf("Adding of stroppy user: unsuccess. The number of attempts has ended")
	}

	llog.Debugln("Adding of stroppy user: success")

	return nil
}
