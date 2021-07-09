package db

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const runningPodStatus = "Running"

const postgresClusterName = "acid-postgres-cluster-0"

const (
	maxNotFoundCount = 5

	// nolint
	successPostgresPodsCount = 3
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
	err = pc.commonCluster.deploy()
	return
}

func (pc *postgresCluster) OpenPortForwarding() error {
	return pc.openPortForwarding(postgresClusterName, []string{"6432:5432"})
}

// GetStatus проверить, что все поды postgres в running
func (pc *postgresCluster) GetStatus() error {
	llog.Infoln("Checking of deploy postgres...")

	clientSet, err := pc.k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clienset for check deploy of postgres")
	}

	var postgresPodsCount int64
	if postgresPodsCount, err = pc.getPostgresPodsCount(); err != nil {
		return merry.Prepend(err, "failed to get postgres pods count")
	}

	var successPodsCount int64
	var notFoundCount int64
	for successPodsCount < postgresPodsCount && notFoundCount < maxNotFoundCount {
		llog.Infof("waiting for checking %v seconds...\n", ExecTimeout)
		time.Sleep(ExecTimeout * time.Second)

		podNumber := fmt.Sprintf("acid-postgres-cluster-%d", successPodsCount)
		acidPostgresZeroPod, err := clientSet.CoreV1().Pods("default").Get(context.TODO(),
			podNumber, metav1.GetOptions{
				TypeMeta:        metav1.TypeMeta{},
				ResourceVersion: "",
			})
		switch {
		case k8s_errors.IsNotFound(err):
			llog.Infof("Pod %v not found in default namespace\n", podNumber)
			notFoundCount++

		case k8s_errors.IsInternalError(err):
			internalErrorString := fmt.Sprintf("internal error in pod %v\n", podNumber)
			return merry.Prepend(err, internalErrorString)

		case err != nil:
			unknownErrorString := fmt.Sprintf("unknown error getting pod %v", podNumber)
			return merry.Prepend(err, unknownErrorString)

		default:
			llog.Infof("Found pod %v in default namespace\n", podNumber)
		}

		llog.Infof("status of pod %v: %v\n", podNumber, acidPostgresZeroPod.Status.Phase)
		// Status.Phase - текущий статус пода
		if acidPostgresZeroPod.Status.Phase == runningPodStatus {
			// переходим к следующему поду и сбрасываем счетчик not found
			successPodsCount++
			notFoundCount = 0
		}

	}

	if notFoundCount >= maxNotFoundCount {
		return ErrorPodsNotFound
	}

	return nil
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

func (pc *postgresCluster) GetSpecification() (spec ClusterSpec) {
	spec = pc.clusterSpec
	return
}
