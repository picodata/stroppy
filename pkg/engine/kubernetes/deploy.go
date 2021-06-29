package kubernetes

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfig "k8s.io/client-go/applyconfigurations/core/v1"
)

func (k *Kubernetes) Deploy() (pPortForward *engineSsh.Result, port int, err error) {
	if err = k.copyFilesToMaster(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to сopy files to cluster")
	}

	if err = k.deploy(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to deploy k8s")
	}

	if err = k.copyConfigFromMaster(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to copy kube config from Master")
	}

	if k.sshTunnel = k.openSSHTunnel(kubernetesSshEntity, clusterK8sPort, reserveClusterK8sPort); k.sshTunnel.Err != nil {
		err = merry.Prepend(k.sshTunnel.Err, "failed to create ssh tunnel")
		return
	}
	llog.Infoln("status of creating ssh tunnel for the access to k8s: success")

	var pod *v1.Pod

	pod, err = k.DeployStroppy()
	if err != nil {
		return nil, 0, merry.Prepend(err, "failed to deploy stroppy pod")
	}

	if pod.Status.Phase != v1.PodRunning {
		err = merry.Errorf("stroppy pod is not running")
		return
	}

	llog.Infoln("status of stroppy pod deploy: success")

	pPortForward = k.openSSHTunnel(monitoringSshEntity, clusterMonitoringPort, reserveClusterMonitoringPort)

	if pPortForward.Err != nil {
		return nil, 0, merry.Prepend(pPortForward.Err, "failed to port forward")
	}
	llog.Infoln("status of creating ssh tunnell for the access to monitoring: success")

	port = k.sshTunnel.Port
	k.portForward = pPortForward

	return
}

func (k *Kubernetes) DeployStroppy() (*v1.Pod, error) {
	clientSet, err := k.GetClientSet()
	if err != nil {
		return nil, merry.Prepend(err, "failed to get clientset for deploy stroppy")
	}

	deployConfigStroppyPath := filepath.Join(k.workingDirectory, deployConfigStroppyFile)
	deployConfig, err := ioutil.ReadFile(deployConfigStroppyPath)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	stroppy := applyconfig.Pod("stroppy-client", ResourceDefaultNamespace)

	err = yaml.Unmarshal([]byte(deployConfig), &stroppy)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	llog.Infoln("Applying the stroppy pod...")
	pod, err := clientSet.CoreV1().Pods("default").Apply(context.TODO(), stroppy, metav1.ApplyOptions{
		TypeMeta: metav1.TypeMeta{
			Kind:       "",
			APIVersion: "",
		},
		DryRun:       []string{},
		Force:        false,
		FieldManager: "stroppy-deploy",
	})
	if err != nil {
		return nil, merry.Prepend(err, "failed to apply pod stroppy")
	}
	// на случай чуть большего времени на переход в running, обычно под переходит в running сразу
	time.Sleep(20 * time.Second)

	llog.Infoln("Applying the stroppy pod: success")

	return pod, nil
}

// deploy - развернуть k8s внутри кластера в cloud
func (k *Kubernetes) deploy() (err error) {
	/* Последовательно формируем файл deploy_kubernetes.sh,
	   даем ему права на выполнение и выполняем.
	   1-й шаг - добавляем первую часть команд (deployk8sFirstStepCmd)
	   2-й шаг - подставляем ip адреса в hosts.ini и добавляем команду с его записью в файл
	   3-й шаг - добавляем вторую часть команд (deployk8sThirdStepCmd)
	   4-й шаг - выдаем файлу права на выполнение и выполняем */

	var isDeployed bool
	if isDeployed, err = k.checkMasterDeploymentStatus(); err != nil {
		return merry.Prepend(err, "failed to Check deploy k8s in master node")
	}

	if isDeployed {
		llog.Infoln("k8s already success deployed")
		return
	}

	deployk8sFirstStepCmd, deployk8sThirdStepCmd, err := getProviderDeployCommands(k)
	if err != nil {
		return merry.Prepend(err, "failed to get deploy commands")
	}

	if err = k.executeCommand(deployk8sFirstStepCmd); err != nil {
		return merry.Prepend(err, "first step deployment failed")
	}
	llog.Printf("First step deploy k8s: success")

	mapIP := k.addressMap

	secondStepCommandText := fmt.Sprintf(deployK8sSecondStepTemplate,
		mapIP.MasterInternalIP, mapIP.MasterInternalIP,
		mapIP.MetricsInternalIP, mapIP.MetricsInternalIP,
		mapIP.IngressInternalIP, mapIP.IngressInternalIP,
		mapIP.PostgresInternalIP, mapIP.PostgresInternalIP,
	)
	if err = k.executeCommand(secondStepCommandText); err != nil {
		return merry.Prepend(err, "failed second step deploy k8s")
	}
	llog.Printf("Second step deploy k8s: success")

	if err = k.executeCommand(deployk8sThirdStepCmd); err != nil {
		return merry.Prepend(err, "failed third step deploy k8s")
	}
	llog.Printf("Third step deploy k8s: success")

	const fooStepCommand = "chmod +x deploy_kubernetes.sh && ./deploy_kubernetes.sh -y"

	var (
		fooSession engineSsh.Session
		fooStdout  io.Reader
	)
	if fooStdout, fooSession, err = k.getSessionObject(); err != nil {
		return merry.Prepend(err, "failed foo step deploy k8s")
	}
	go engine.HandleReader(bufio.NewReader(fooStdout))
	llog.Infof("Waiting for deploying about 20 minutes...")

	var fooSessionResult []byte
	if fooSessionResult, err = fooSession.CombinedOutput(fooStepCommand); err != nil {
		llog.Infoln(string(fooSessionResult))
		return merry.Prepend(err, "failed foo step deploy k8s waiting")
	}

	llog.Printf("Foo step deploy k8s: success")
	_ = fooSession.Close()

	return
}

func getProviderDeployCommands(kubernetes *Kubernetes) (string, string, error) {
	// provider := kubernetes.
	switch kubernetes.provider {
	case "yandex":
		// подставляем константы
		return deployK8sFirstStepYandexCMD, deployK8sThirdStepYandexCMD, nil

	case "oracle":

		deployK8sFirstStepOracleCMD := fmt.Sprintf(deployK8sFirstStepOracleTemplate,
			kubernetes.addressMap.MetricsInternalIP, kubernetes.addressMap.IngressInternalIP, kubernetes.addressMap.PostgresInternalIP)
		deployK8sThirdStepOracleCMD := deployK8sThirdStepOracleCMD

		return deployK8sFirstStepOracleCMD, deployK8sThirdStepOracleCMD, nil

	default:
		return "", "", errProviderChoice
	}
}

// checkMasterDeploymentStatus
// проверяет, что все поды k8s в running, что подтверждает успешность разворачивания k8s
func (k *Kubernetes) checkMasterDeploymentStatus() (bool, error) {
	masterExternalIP := k.addressMap.MasterExternalIP

	sshClient, err := engineSsh.CreateClient(k.workingDirectory,
		masterExternalIP,
		k.provider,
		k.sessionIsLocal)
	if err != nil {
		return false, merry.Prependf(err, "failed to establish ssh client to '%s' address", masterExternalIP)
	}

	checkSession, err := sshClient.GetNewSession()
	if err != nil {
		return false, merry.Prepend(err, "failed to open ssh connection for Check deploy")
	}

	const checkCmd = "kubectl get pods --all-namespaces"
	resultCheckCmd, err := checkSession.CombinedOutput(checkCmd)
	if err != nil {
		//nolint:errorlint
		e, ok := err.(*ssh.ExitError)
		if !ok {
			return false, merry.Prepend(err, "failed сheck deploy k8s")
		}
		// если вернулся not found(код 127), это норм, если что-то другое - лучше проверить
		if e.ExitStatus() == sshNotFoundCode {
			return false, nil
		}
	}

	countPods := strings.Count(string(resultCheckCmd), "Running")
	if countPods < runningPodsCount {
		return false, nil
	}

	_ = checkSession.Close()
	return true, nil
}