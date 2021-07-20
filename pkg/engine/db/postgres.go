package db

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	kuberv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"k8s.io/client-go/kubernetes/scheme"
)

func CreatePostgresCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (pc Cluster) {
	pc = &postgresCluster{
		commonCluster: createCommonCluster(sc,
			k,
			filepath.Join(wd, cluster.Postgres),
			cluster.Postgres),
	}
	return
}

type postgresCluster struct {
	*commonCluster
}

// Deploy
// разворачивает postgres в кластере
func (pc *postgresCluster) Deploy() (err error) {
	if err = pc.commonCluster.deploy(); err != nil {
		return merry.Prepend(err, "deploy")
	}

	llog.Debugln("Checking of deploy postgres...")

	var postgresPodsCount int64
	if postgresPodsCount, err = pc.getPostgresPodsCount(); err != nil {
		return merry.Prepend(err, "failed to get postgres pods count")
	}

	const postgresPodNameTemplate = "acid-postgres-cluster-%d"
	for i := int64(0); i < postgresPodsCount; i++ {
		podName := fmt.Sprintf(postgresPodNameTemplate, i)

		var targetPod *kuberv1.Pod
		targetPod, err = pc.k.WaitPod(podName, kubernetes.ResourceDefaultNamespace,
			kubernetes.PodWaitingWaitCreation, kubernetes.PodWaitingTime10Minutes)
		if err != nil {
			err = merry.Prepend(err, "waiting")
			return
		}

		pc.clusterSpec.Pods = append(pc.clusterSpec.Pods, targetPod)
		if i == 0 {
			pc.clusterSpec.MainPod = targetPod
		}
	}

	err = pc.openPortForwarding(pc.clusterSpec.MainPod.Name, []string{"6432:5432"})
	return
}

func (pc *postgresCluster) GetSpecification() (spec ClusterSpec) {
	spec = pc.clusterSpec
	return
}

// getPostgresPodsCount возвращает кол-во подов postgres, которые должны быть созданы
func (pc *postgresCluster) getPostgresPodsCount() (int64, error) {
	manifestFilePath := filepath.Join(pc.wd, "postgres-manifest.yaml")
	manifestFile, err := ioutil.ReadFile(manifestFilePath)
	if err != nil {
		return 0, merry.Prepend(err, "failed to read postgres-manifest.yaml")
	}

	var obj runtime.Object
	obj, _, err = scheme.Codecs.UniversalDeserializer().
		Decode(manifestFile, nil, &v1.Postgresql{})
	if err != nil {
		return 0, merry.Prepend(err, "failed to decode postgres-manifest.yaml")
	}

	postgresSQLConfig, ok := obj.(*v1.Postgresql)
	if !ok {
		return 0, merry.Prepend(err, "failed to check format postgres-manifest.yaml")
	}

	podsCount := int64(postgresSQLConfig.Spec.NumberOfInstances)
	return podsCount, nil
}
