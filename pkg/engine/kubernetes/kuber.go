package kubernetes

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/pkg/database/config"
	"gopkg.in/yaml.v2"

	"github.com/ansel1/merry"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	applyconfig "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

const (
	sshNotFoundCode = 127

	// кол-во попыток подключения при ошибке
	connectionRetryCount = 3
)

var errPortCheck = errors.New("port Check failed")

var errProviderChoice = errors.New("selected provider not found")

func CreateKubernetes(settings *config.Settings,
	terraformAddressMap terraform.MapAddresses,
	sshClient engineSsh.Client) (k *Kubernetes, err error) {

	k = &Kubernetes{
		workingDirectory:  settings.WorkingDirectory,
		clusterConfigFile: filepath.Join(settings.WorkingDirectory, "config"),

		addressMap: terraformAddressMap,
		sc:         sshClient,

		provider:       settings.DeploySettings.Provider,
		sessionIsLocal: settings.Local,

		isSshKeyFileOnMaster: false,
	}
	k.sshKeyFileName, k.sshKeyFilePath = k.sc.GetPrivateKeyInfo()

	llog.Infof("kubernetes init success on directory '%s', with provider '%s', and ssh key file '%s'",
		k.workingDirectory, k.provider, k.sshKeyFilePath)
	return
}

type Kubernetes struct {
	workingDirectory  string
	clusterConfigFile string

	addressMap terraform.MapAddresses

	sshKeyFileName string
	sshKeyFilePath string
	sshTunnel      *engineSsh.Result
	sc             engineSsh.Client

	isSshKeyFileOnMaster bool
	sessionIsLocal       bool

	portForward *engineSsh.Result

	provider string
}

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

	if k.sshTunnel = k.openSSHTunnel("kubernetes", clusterK8sPort, reserveClusterK8sPort); k.sshTunnel.Err != nil {
		err = merry.Prepend(k.sshTunnel.Err, "failed to create ssh tunnel")
		return
	}

	pod, err := k.DeployStroppy()
	if err != nil {
		return nil, 0, merry.Prepend(err, "failed to deploy stroppy pod")
	}

	if pod.Status.Phase != "Running" {
		return nil, 0, merry.Prepend(err, "stroppy pod status is not running")
	}

	pPortForward = k.openSSHTunnel("monitoring", clusterMonitoringPort, reserveClusterMonitoringPort)
	llog.Println(pPortForward)
	if pPortForward.Err != nil {
		return nil, 0, merry.Prepend(pPortForward.Err, "failed to port forward")
	}

	port = k.sshTunnel.Port
	k.portForward = pPortForward

	return
}

func (k *Kubernetes) executeCommand(text string) (err error) {
	var commandSessionObject engineSsh.Session
	if commandSessionObject, err = k.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	if result, err := commandSessionObject.CombinedOutput(text); err != nil {
		// вывводим, чтобы было проще диагностировать
		llog.Errorln(string(result))
		return merry.Prepend(err, "output collect failed")
	}

	return
}

func (k *Kubernetes) getSessionObject() (stdout io.Reader, session engineSsh.Session, err error) {
	if session, err = k.sc.GetNewSession(); err != nil {
		err = merry.Prepend(err, "failed to open ssh connection")
		return
	}

	if stdout, err = session.StdoutPipe(); err != nil {
		err = merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")

		if err = session.Close(); err != nil {
			llog.Warnf("getSessionObject: k8s ssh session can not closed: %v", err)
		}
	}

	return
}

func (k *Kubernetes) ExecuteCommand(text string) (err error) {
	err = k.executeCommand(text)
	return
}

// OpenPortForward открывает port-forward туннель для вызывающей функции(caller)
func (k *Kubernetes) OpenPortForward(caller string, ports []string, reqURL *url.URL,
	stopPortForward chan struct{}, readyPortForward chan struct{}) (err error) {

	llog.Printf("Opening of port-forward of %v...\n", caller)

	var kubeConfig *rest.Config
	if kubeConfig, err = k.getKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kube config")
	}

	var httpTransaction http.RoundTripper
	var updater spdy.Upgrader
	if httpTransaction, updater, err = spdy.RoundTripperFor(kubeConfig); err != nil {
		return merry.Prepend(err, "failed to create spdy transaction for port-forward")
	}

	var portForwardLog *os.File
	portForwardLog, err = os.OpenFile("portForwardPostgres.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644)
	if err != nil {
		return merry.Prepend(err, "failed to create or open log file for port-forward")
	}
	// defer portForwardLog.Close() не делаем, поскольку при выходе из приложения runtime сам все закроет

	dialer := spdy.NewDialer(updater,
		&http.Client{Transport: httpTransaction},
		http.MethodPost, reqURL)

	var portForward *portforward.PortForwarder
	portForward, err = portforward.New(dialer, ports,
		stopPortForward, readyPortForward, portForwardLog, portForwardLog)
	if err != nil {
		return merry.Prepend(err, "failed to get port-forwarder")
	}

	defer close(stopPortForward)
	if err = portForward.ForwardPorts(); err != nil {
		err = merry.Prepend(err, "failed to open port-forward")
	}
	return
}

func getProviderDeployCommands(kubernetes *Kubernetes) (string, string, error) {
	// provider := kubernetes.
	switch kubernetes.provider {
	case "yandex":
		// подставляем константы
		return Deployk8sFirstStepYandexCMD, Deployk8sThirdStepYandexCMD, nil

	case "oracle":

		deployk8sFirstStepOracleCMD := fmt.Sprintf(Deployk8sFirstStepOracleTemplate,
			kubernetes.addressMap.MetricsInternalIP, kubernetes.addressMap.IngressInternalIP, kubernetes.addressMap.PostgresInternalIP)
		deployk8sThirdStepOracleCMD := Deployk8sThirdStepOracleCMD

		return deployk8sFirstStepOracleCMD, deployk8sThirdStepOracleCMD, nil

	default:
		return "", "", errProviderChoice
	}
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
	if isDeployed, err = k.checkDeployMaster(); err != nil {
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

	secondStepCommandText := fmt.Sprintf(Deployk8sSecondStepTemplate,
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

// getKubeConfig - получить конфигурацию k8s
func (k *Kubernetes) getKubeConfig() (*rest.Config, error) {
	config, err := clientcmd.BuildConfigFromFlags("", k.clusterConfigFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get config for check deploy of postgres")
	}
	return config, nil
}

func (k *Kubernetes) GetClientSet() (*kubernetes.Clientset, error) {
	config, err := k.getKubeConfig()
	if err != nil {
		return nil, merry.Prepend(err, "failed to get kubeconfig for clientSet")
	}

	// clientSet - клиент для обращения к группам сущностей k8s
	var clientSet *kubernetes.Clientset
	if clientSet, err = kubernetes.NewForConfig(config); err != nil {
		return nil, merry.Prepend(err, "failed to create clientSet")
	}

	return clientSet, nil
}

// checkDeployMaster
// проверить, что все поды k8s в running, что подтверждает успешность деплоя k8s
func (k *Kubernetes) checkDeployMaster() (bool, error) {
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

// openSSHTunnel
// открыть ssh-соединение и передать указатель на него вызывающему коду для управления
func (k *Kubernetes) openSSHTunnel(caller string, mainPort int, reservePort int) (result *engineSsh.Result) {
	mastersConnectionString := fmt.Sprintf("ubuntu@%v", k.addressMap.MasterExternalIP)

	tunnelPort := mainPort
	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Infof("Checking the status of port %v of the localhost for %v...\n", caller, tunnelPort)
	if !engine.IsLocalPortOpen(tunnelPort) {
		// проверяем резервный порт в случае недоступности основного
		tunnelPort = reservePort
		llog.Infof("Checking the status of port %v of the localhost for %v...\n", caller, tunnelPort)
		if !engine.IsLocalPortOpen(tunnelPort) {
			result = &engineSsh.Result{
				Port:   0,
				Tunnel: nil,
				Err:    merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available"),
			}
			return
		}

		// подменяем порт в kubeconfig на локальной машине
		clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
		if err := k.editClusterURL(clusterURL); err != nil {
			llog.Infof("failed to replace port: %v", err)
			result = &engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
			return
		}
	}

	authMethod, err := sshtunnel.PrivateKeyFile(k.sshKeyFilePath)
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		result = &engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
		return
	}

	// Setup the tunnel, but do not yet start it yet.
	var tunnel *sshtunnel.SSHTunnel
	tunnel, err = sshtunnel.NewSSHTunnel(
		mastersConnectionString,
		fmt.Sprintf("localhost:%v", mainPort),
		tunnelPort,
		authMethod,
	)
	if err != nil {
		result = &engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to create tunnel"),
		}
		return
	}

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	if err = tunnel.Start(); err != nil {
		result = &engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to start tunnel"),
		}
		return
	}

	return &engineSsh.Result{Port: tunnelPort, Tunnel: tunnel, Err: nil}
}

func (k Kubernetes) DownloadFile(remoteFullSourceFilePath, localPath string) (err error) {
	return
}

// copyConfigFromMaster
// скопировать файл kube config c мастер-инстанса кластера и применить для использования
func (k *Kubernetes) copyConfigFromMaster() (err error) {
	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", k.addressMap.MasterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", k.sshKeyFileName, "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	copyFromMasterCmd.Dir = k.workingDirectory
	llog.Infoln(copyFromMasterCmd.String())

	if _, err = copyFromMasterCmd.CombinedOutput(); err != nil {
		return merry.Prepend(err, "failed to execute command copy from master")
	}

	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	clusterURL := "https://localhost:6443"
	if err = k.editClusterURL(clusterURL); err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	return
}

func (k *Kubernetes) installSshKeyFileOnMaster() (err error) {
	if k.isSshKeyFileOnMaster {
		return
	}

	masterExternalIP := k.addressMap.MasterExternalIP
	mastersConnectionString := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.ssh", masterExternalIP)
	copyPrivateKeyCmd := exec.Command("scp",
		"-i", k.sshKeyFileName,
		"-o", "StrictHostKeyChecking=no",
		k.sshKeyFileName, mastersConnectionString)
	copyPrivateKeyCmd.Dir = k.workingDirectory

	llog.Infof(copyPrivateKeyCmd.String())

	keyFileCopyed := false
	// делаем переповтор на случай проблем с кластером
	// \todo: https://gitlab.com/picodata/stroppy/-/issues/4
	for i := 0; i <= connectionRetryCount; i++ {
		copyMasterCmdResult, err := copyPrivateKeyCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("failed to copy private key key onto master: %v %v \n", string(copyMasterCmdResult), err)
			copyPrivateKeyCmd = exec.Command("scp",
				"-i", k.sshKeyFileName,
				"-o", "StrictHostKeyChecking=no",
				k.sshKeyFileName, mastersConnectionString)
			time.Sleep(execTimeout * time.Second)
			continue
		}

		keyFileCopyed = true
		llog.Tracef("result of copy private key: %v \n", string(copyMasterCmdResult))
		break
	}
	if !keyFileCopyed {
		return merry.New("key file not copied to master")
	}

	k.isSshKeyFileOnMaster = true
	return
}

func (k *Kubernetes) LoadFile(sourceFilePath, destinationFilePath string) (err error) {
	if err = k.installSshKeyFileOnMaster(); err != nil {
		return
	}

	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение, поэтому ssh.InsecureIgnoreHostKey
	var clientSSHConfig ssh.ClientConfig
	clientSSHConfig, err = auth.PrivateKey("ubuntu", k.sshKeyFilePath, ssh.InsecureIgnoreHostKey())
	if err != nil {
		return
	}

	masterFullAddress := fmt.Sprintf("%v:22", k.addressMap.MasterExternalIP)

	client := scp.NewClient(masterFullAddress, &clientSSHConfig)
	if err = client.Connect(); err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	var sourceFile *os.File
	if sourceFile, err = os.Open(sourceFilePath); err != nil {
		return merry.Prependf(err, "failed to open local file '%s'", sourceFilePath)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			llog.Warnf("failed to close local descriptor for '%s' file: %v",
				sourceFilePath, err)
		}
	}()

	if err = client.CopyFile(sourceFile, destinationFilePath, "0664"); err != nil {
		return merry.Prepend(err, "error while copying file metrics-server.yaml")
	}

	client.Close()
	return
}

func (k *Kubernetes) LoadDirectory(directorySourcePath, destinationPath string) (err error) {
	destinationPath = fmt.Sprintf("ubuntu@%s:%s", k.addressMap.MasterExternalIP, destinationPath)

	copyDirectoryCmd := exec.Command("scp", "-r", "-i", k.sshKeyFilePath, "-o", "StrictHostKeyChecking=no",
		directorySourcePath, destinationPath)

	llog.Infof("now loading '%s' directory to kubernetes master destination '%s' (keyfile '%s', wd: '%s')",
		directorySourcePath, destinationPath, k.sshKeyFilePath, copyDirectoryCmd.Dir)

	var output []byte
	if output, err = copyDirectoryCmd.CombinedOutput(); err != nil {
		return merry.Prependf(err, "error while copying directory to k8 master: %v, output: '%s'",
			err, string(output))
	}

	return
}

/* copyFilesToMaster
 * скопировать на мастер-ноду private key для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
func (k *Kubernetes) copyFilesToMaster() (err error) {
	masterExternalIP := k.addressMap.MasterExternalIP
	llog.Infoln(masterExternalIP)

	if k.provider == "yandex" {
		/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
		если кластер пока не готов*/
		llog.Infoln("Checking status of port 22 on the cluster's master...")
		var masterPortAvailable bool
		for i := 0; i <= connectionRetryCount; i++ {
			masterPortAvailable = engine.IsRemotePortOpen(masterExternalIP, 22)
			if !masterPortAvailable {
				llog.Infof("status of check the master's port 22:%v. Repeat #%v", errPortCheck, i)
				time.Sleep(execTimeout * time.Second)
			} else {
				break
			}
		}
		if !masterPortAvailable {
			return merry.Prepend(errPortCheck, "master's port 22 is not available")
		}
	}

	metricsServerFilePath := filepath.Join(k.workingDirectory, "metrics-server.yaml")
	if err = k.LoadFile(metricsServerFilePath, "/home/ubuntu/metrics-server.yaml"); err != nil {
		return
	}
	llog.Infoln("copying metrics-server.yaml: success")

	ingressGrafanaFilePath := filepath.Join(k.workingDirectory, "ingress-grafana.yaml")
	if err = k.LoadFile(ingressGrafanaFilePath, "/home/ubuntu/ingress-grafana.yaml"); err != nil {
		return
	}
	llog.Infoln("copying ingress-grafana.yaml: success")

	grafanaDirectoryPath := filepath.Join(k.workingDirectory, "grafana-on-premise")
	if err = k.LoadDirectory(grafanaDirectoryPath, "/home/ubuntu"); err != nil {
		return
	}
	llog.Infoln("copying grafana-on-premise: success")

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

	stroppy := applyconfig.Pod("stroppy-client", "default")

	err = yaml.Unmarshal([]byte(deployConfig), &stroppy)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall deploy stroppy configuration")
	}

	llog.Infoln("Applying the stroppy pod...")
	pod, err := clientSet.CoreV1().Pods("default").Apply(context.TODO(), stroppy, metav1.ApplyOptions{
		TypeMeta:     metav1.TypeMeta{},
		DryRun:       []string{},
		Force:        false,
		FieldManager: "stroppy-deploy",
	})
	if err != nil {
		return nil, merry.Prepend(err, "failed to apply pod stroppy")
	}

	return pod, nil
}

func (k *Kubernetes) Stop() {
	defer k.sshTunnel.Tunnel.Close()
	llog.Infoln("status of ssh tunnel close: success")

	// defer k.portForward.Tunnel.Close()
	// llog.Infoln("status of port-forward's close: success")
}
