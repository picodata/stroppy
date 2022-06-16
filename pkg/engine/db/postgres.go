/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package db

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"

	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/kubernetes"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	kuberv1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
)

//nolint // don't known how to fix ireturn
func createPostgresCluster(
	sshClient engineSsh.Client,
	kube *kubernetes.Kubernetes,
	workDir, dbURL string,
	connectionPoolSize int,
) Cluster {
	return &postgresCluster{
		commonCluster: createCommonCluster(
			sshClient,
			kube,
			filepath.Join(workDir, dbWorkingDirectory, cluster.Postgres),
			cluster.Postgres,
			dbURL,
			connectionPoolSize,
			false,
		),
	}
}

type postgresCluster struct {
	*commonCluster
}

func (pc *postgresCluster) Connect() (interface{}, error) {
	var (
		pgCluster interface{}
		err       error
	)

	// для возможности подключиться к БД в кластере с локальной машины
	if pc.DBUrl == "" {
		pc.DBUrl = "postgres://stroppy:stroppy@localhost:6432/stroppy?sslmode=disable"
		llog.Infoln("changed DBURL on", pc.DBUrl)
	}

	if pgCluster, err = cluster.NewPostgresCluster(pc.DBUrl, pc.connectionPoolSize); err != nil {
		return nil, merry.Prepend(err, "Error then creating postgres cluster")
	}

	return pgCluster, nil
}

// Deploy
// разворачивает postgres в кластере
func (pc *postgresCluster) Deploy() (err error) {
	if err = pc.deploy(); err != nil {
		return merry.Prepend(err, "deploy")
	}

	llog.Infoln("Checking of deploy postgres...")

	var postgresPodsCount int64
	var postgresPodName string
	if postgresPodsCount, postgresPodName, err = pc.getClusterParameters(); err != nil {
		return merry.Prepend(err, "failed to get postgres pods count")
	}

	postgresPodNameTemplate := postgresPodName + "-%d"
	for i := int64(0); i < postgresPodsCount; i++ {
		podName := fmt.Sprintf(postgresPodNameTemplate, i)

		var targetPod *kuberv1.Pod
		targetPod, err = pc.k.Engine.WaitPod(podName, kubeengine.ResourceDefaultNamespace,
			kubeengine.PodWaitingWaitCreation, kubeengine.PodWaitingTimeTenMinutes)

		if err != nil {
			err = merry.Prepend(err, "waiting")
			return
		}

		pc.clusterSpec.Pods = append(pc.clusterSpec.Pods, targetPod)
		llog.Infof("'%s/%s' pod registered", targetPod.Namespace, targetPod.Name)
		if i == 0 {
			pc.clusterSpec.MainPod = targetPod
			llog.Debugln("... and this pod is main")
		}
	}

	runningPodsCount := len(pc.clusterSpec.Pods)
	if runningPodsCount < int(postgresPodsCount) {
		return fmt.Errorf("finded only %d postgres pods, expected %d",
			runningPodsCount, postgresPodsCount)
	}

	if pc.clusterSpec.MainPod == nil {
		return errors.New("main pod does not exists")
	}

	err = pc.openPortForwarding(pc.clusterSpec.MainPod.Name, []string{"6432:5432"})
	return
}

func (pc *postgresCluster) GetSpecification() (spec ClusterSpec) {
	spec = pc.clusterSpec
	return
}

// getClusterParameters возвращает кол-во подов postgres, которые должны быть созданы
func (pc *postgresCluster) getClusterParameters() (podsCount int64, clusterName string, err error) {
	manifestFilePath := filepath.Join(pc.wd, "postgres-manifest.yaml")

	var manifestFileContent []byte
	if manifestFileContent, err = ioutil.ReadFile(manifestFilePath); err != nil {
		err = merry.Prepend(err, "failed to read postgres-manifest.yaml")
		return
	}

	specStartPos := bytes.Index(manifestFileContent, []byte("\n---\napiVersion: \"acid.zalan.do"))
	if specStartPos > 0 {
		// пропускаем первую часть конфига, если таковая имеется
		manifestFileContent = manifestFileContent[specStartPos+5:]
	}

	var obj runtime.Object
	obj, _, err = scheme.Codecs.UniversalDeserializer().
		Decode(manifestFileContent, nil, &v1.Postgresql{})
	if err != nil {
		err = merry.Prepend(err, "failed to decode postgres-manifest.yaml")
		return
	}

	postgresSQLConfig, ok := obj.(*v1.Postgresql)
	if !ok {
		err = merry.Prepend(err, "failed to check format postgres-manifest.yaml")
		return
	}

	podsCount = int64(postgresSQLConfig.Spec.NumberOfInstances)
	clusterName = postgresSQLConfig.Name
	return
}
