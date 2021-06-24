package postgres

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/cluster"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const runningPodStatus = "Running"

const (
	maxNotFoundCount = 5

	// nolint
	successPostgresPodsCount = 3
)

func CreateCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd string) (pc *Cluster) {
	pc = &Cluster{
		k:  k,
		sc: sc,
		wd: filepath.Join(wd, cluster.Postgres),
	}
	return
}

type Cluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
}

// Deploy
// разворачивает postgres в кластере
func (pc *Cluster) Deploy() (err error) {
	llog.Infoln("Prepare deploy of postgres")

	postgresDeployConfigDirectory := pc.wd
	if err = pc.k.LoadFile(postgresDeployConfigDirectory, "/home/ubuntu/postgres"); err != nil {
		return
	}
	llog.Infoln("copying postgres directory: success")

	var sshSession engineSsh.Session
	if sshSession, err = pc.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection for deploy of postgres")
	}

	// \todo: вынести в gracefulShutdown, если вообще в этом требуется необходимость, поскольку runtime при выходе закроет сам
	// defer sshSession.Close()

	llog.Infoln("Starting deploy of postgres")
	deployCmd := "chmod +x postgres/deploy_operator.sh && ./postgres/deploy_operator.sh"

	var stdout io.Reader
	if stdout, err = sshSession.StdoutPipe(); err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}
	go engine.HandleReader(bufio.NewReader(stdout))

	if _, err = sshSession.CombinedOutput(deployCmd); err != nil {
		return merry.Prepend(err, "command execution failed")
	}

	llog.Infoln("Finished deploy of postgres")
	return nil
}

func (pc *Cluster) OpenPortForwarding() error {
	stopPortForwardPostgres := make(chan struct{})
	readyPortForwardPostgres := make(chan struct{})

	clientSet, err := pc.k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientSet for open port-forward of postgres")
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	reqURL := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace("default").
		Name("acid-postgres-cluster-0").
		SubResource("portforward").URL()

	err = pc.k.OpenPortForward(cluster.Postgres, []string{"6432:5432"}, reqURL,
		stopPortForwardPostgres, readyPortForwardPostgres)
	if err != nil {
		return merry.Prepend(err, "failed to started port-forward for postgres")
	}

	select {
	case <-readyPortForwardPostgres:
		llog.Infof("Port-forwarding for postgres is started success\n")
	}
	return nil
}

// GetStatus проверить, что все поды postgres в running
func (pc *Cluster) GetStatus() (*engine.ClusterStatus, error) {
	llog.Infoln("Checking of deploy postgres...")

	var successPodsCount int64

	var notFoundCount int64

	clusterStatus := &engine.ClusterStatus{
		Status: "failed",
		Err:    nil,
	}

	clientSet, err := pc.k.GetClientSet()
	if err != nil {
		return clusterStatus, merry.Prepend(err, "failed to get clienset for check deploy of postgres")
	}

	postgresPodsCount, err := pc.getPostgresPodsCount()
	if err != nil {
		return clusterStatus, merry.Prepend(err, "failed to get postgres pods count")
	}

	for successPodsCount < *postgresPodsCount && notFoundCount < maxNotFoundCount {
		llog.Infof("waiting for checking %v minutes...\n", engine.ExecTimeout)
		time.Sleep(engine.ExecTimeout * time.Second)

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
			clusterStatus.Err = merry.Prepend(err, internalErrorString)
			return clusterStatus, nil
		case err != nil:
			uknnownErrorString := fmt.Sprintf("Unknown error getting pod %v", podNumber)
			clusterStatus.Err = merry.Prepend(err, uknnownErrorString)
			return clusterStatus, nil
		case err == nil:
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
		clusterStatus.Err = engine.ErrorPodsNotFound
		return clusterStatus, nil
	}

	clusterStatus.Status = engine.DeploySuccess
	return clusterStatus, nil
}

// getPostgresPodsCount возвращает кол-во подов postgres, которые должны быть созданы
func (pc *Cluster) getPostgresPodsCount() (*int64, error) {
	manifestFilePath := filepath.Join(pc.wd, "postgres-manifest.yaml")
	manifestFile, err := ioutil.ReadFile(manifestFilePath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read postgres-manifest.yaml")
	}

	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(manifestFile, nil, &v1.Postgresql{})
	if err != nil {
		return nil, merry.Prepend(err, "failed to decode postgres-manifest.yaml")
	}

	postgresSQLconfig, ok := obj.(*v1.Postgresql)
	if !ok {
		return nil, merry.Prepend(err, "failed to check format postgres-manifest.yaml")
	}

	podsCount := int64(postgresSQLconfig.Spec.NumberOfInstances)
	return &podsCount, nil
}
