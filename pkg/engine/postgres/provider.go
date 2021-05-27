package postgres

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "github.com/zalando/postgres-operator/pkg/apis/acid.zalan.do/v1"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"

	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
)

const runningPodStatus = "Running"

const successPostgresPodsCount = 3

const maxNotFoundCount = 5

func CreatePostgresCluster(sc *engineSsh.Client,
	k *kubernetes.Kubernetes,
	terraformAddressMap terraform.MapAddresses) (pc *PostgreCluster) {

	pc = &PostgreCluster{
		k:          k,
		addressMap: terraformAddressMap,
		sc:         sc,
	}
	return
}

type PostgreCluster struct {
	sc         *engineSsh.Client
	k          *kubernetes.Kubernetes
	addressMap terraform.MapAddresses
}

// Deploy
// развернуть postgres в кластере
func (pc *PostgreCluster) Deploy() error {
	llog.Infoln("Prepare deploy of postgres")

	sshSession, err := pc.sc.GetNewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for deploy of postgres")
	}
	defer sshSession.Close()

	llog.Infoln("Starting deploy of postgres")
	deployCmd := "chmod +x deploy_operator.sh && ./deploy_operator.sh"
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}

	stdoutReader := bufio.NewReader(stdout)
	go engine.HandleReader(stdoutReader)

	_, err = sshSession.CombinedOutput(deployCmd)
	if err != nil {
		return merry.Wrap(err)
	}

	llog.Infoln("Finished deploy of postgres")
	return nil
}

func (pc *PostgreCluster) OpenPortForwarding() error {
	stopPortForwardPostgres := make(chan struct{})
	readyPortForwardPostgres := make(chan struct{})
	errorPortForwardPostgres := make(chan error)

	clientset, err := pc.k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientset for open port-forward of postgres")
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	reqURL := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace("default").
		Name("acid-postgres-cluster-0").
		SubResource("portforward").URL()

	go pc.k.OpenPortForward("postgres", []string{"6432:5432"}, reqURL,
		stopPortForwardPostgres, readyPortForwardPostgres, errorPortForwardPostgres)

	select {
	case <-readyPortForwardPostgres:
		llog.Infof("Port-forwarding for postgres is started success\n")
		return nil
	case errPortForwardPostgres := <-errorPortForwardPostgres:
		llog.Errorf("Port-forwarding for postgres is started failed\n")
		return merry.Prepend(errPortForwardPostgres, "failed to started port-forward for postgres")
	}
}

// checkDeployPostgres - проверить, что все поды postgres в running
func (pc *PostgreCluster) GetStatus() (*engine.ClusterStatus, error) {
	llog.Infoln("Checking of deploy postgres...")

	var successPodsCount int64

	var notFoundCount int64

	clusterStatus := &engine.ClusterStatus{
		Status: "failed",
		Err:    nil,
	}

	clientset, err := pc.k.GetClientSet()
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
		//nolint:exhaustivestruct
		acidPostgresZeroPod, err := clientset.CoreV1().Pods("default").Get(context.TODO(),
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

		// чтобы не ждать до следующей итерации
		if successPodsCount >= successPostgresPodsCount {
			llog.Infoln("Сhecking of deploy postgres: success")
			break
		}
	}

	if notFoundCount >= maxNotFoundCount {
		clusterStatus.Err = engine.ErrorPodsNotFound
		return clusterStatus, nil
	}

	clusterStatus.Status = engine.DeploySuccess
	return clusterStatus, nil
}

// getPostgresPodsCount - получить кол-во подов postgres, которые должны быть созданы
func (pc *PostgreCluster) getPostgresPodsCount() (*int64, error) {
	manifestFile, err := ioutil.ReadFile("deploy/postgres-manifest.yaml")
	if err != nil {
		return nil, merry.Prepend(err, "failed to read postgres-manifest.yaml")
	}

	//nolint:exhaustivestruct
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
