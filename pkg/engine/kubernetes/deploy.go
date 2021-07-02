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
	if err = k.loadFilesToMaster(); err != nil {
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

	if err = k.prepareDeployStroppy(); err != nil {
		err = merry.Prepend(err, "failed to prepare stroppy pod deploy")
		return
	}

	if err = k.DeployStroppy(); err != nil {
		err = merry.Prepend(err, "failed to deploy stroppy pod")
		return
	}
	llog.Infoln("status of stroppy pod deploy: success")

	pPortForward = k.openSSHTunnel(monitoringSshEntity, clusterMonitoringPort, reserveClusterMonitoringPort)

	if pPortForward.Err != nil {
		return nil, 0, merry.Prepend(pPortForward.Err, "failed to port forward")
	}
	llog.Infoln("status of creating ssh tunnel for the access to monitoring: success")

	port = k.sshTunnel.Port
	k.portForward = pPortForward

	return
}

func (k *Kubernetes) DeployStroppy() error {
	clientSet, err := k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get client set for deploy stroppy")
	}

	deployConfigStroppyPath := filepath.Join(k.workingDirectory, deployConfigStroppyFile)
	deployConfig, err := ioutil.ReadFile(deployConfigStroppyPath)
	if err != nil {
		return merry.Prepend(err, "failed to read config file for deploy stroppy")
	}

	stroppy := applyconfig.Pod(stroppyPodName, ResourceDefaultNamespace)

	if err = yaml.Unmarshal(deployConfig, &stroppy); err != nil {
		return merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	time.Sleep(5 * time.Minute)

	llog.Infoln("Applying stroppy pod...")
	k.stroppyPod, err = clientSet.CoreV1().
		Pods(ResourceDefaultNamespace).
		Apply(context.TODO(), stroppy, metav1.ApplyOptions{
			TypeMeta: metav1.TypeMeta{
				Kind:       "",
				APIVersion: "",
			},
			DryRun:       []string{},
			Force:        false,
			FieldManager: stroppyFieldManager,
		})
	if err != nil {
		llog.Error(merry.Prepend(err, "failed to apply pod stroppy"))
	}

	// на случай чуть большего времени на переход в running, ожидаем 5 минут, если не запустился - возвращаем ошибку
	if k.stroppyPod.Status.Phase != v1.PodRunning {
		if err = k.waitStroppyPod(clientSet); err != nil {
			llog.Error(err)
		}
	}

	llog.Infoln("Applying the stroppy pod: success")
	return nil
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

func (k *Kubernetes) prepareDeployStroppy() error {
	llog.Infoln("Preparing of stroppy pod deploy")

	if err := k.executeCommand(dockerRepLoginCmd); err != nil {
		return merry.Prepend(err, "failed to login in prvivate repository")
	}
	llog.Infoln("login in prvivate repository: success")

	clientSet, err := k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientset for stroppy secret")
	}

	secretFilePath := filepath.Join(k.workingDirectory, deployConfigStroppyFile)
	secretFile, err := ioutil.ReadFile(secretFilePath)
	if err != nil {
		return merry.Prepend(err, "failed to read config file for stroppy secret")
	}

	secret := applyconfig.Secret("stroppy-secret", "default")

	err = yaml.Unmarshal([]byte(secretFile), &secret)
	if err != nil {
		return merry.Prepend(err, "failed to unmarshall stroppy secret configuration")
	}

	dockerConfigData := map[string][]byte{
		".dockerconfigjson": []byte(" eyJhdXRocyI6eyJyZWdpc3RyeS5naXRsYWIuY29tIjp7InVzZXJuYW1lIjoiZ2l0bGFiK2RlcGxveS10b2tlbi00ODkxMTEiLCJwYXNzd29yZCI6ImJ6Ykd6M2p3ZjFKc1RyeHZ6Tjd4IiwiYXV0aCI6IloybDBiR0ZpSzJSbGNHeHZlUzEwYjJ0bGJpMDBPRGt4TVRFNllucGlSM296YW5kbU1VcHpWSEo0ZG5wT04zZz0ifX19"),
	}

	secret = secret.WithData(dockerConfigData)

	llog.Infoln("Applying the stroppy secret...")
	_, err = clientSet.CoreV1().Secrets("default").Apply(context.TODO(), secret, metav1.ApplyOptions{
		TypeMeta:     metav1.TypeMeta{},
		DryRun:       []string{},
		Force:        false,
		FieldManager: "stroppy-deploy",
	})
	if err != nil {
		return merry.Prepend(err, "failed to apply stroppy secret")
	}

	llog.Infoln("applying of k8s secret: success")

	return nil
}
