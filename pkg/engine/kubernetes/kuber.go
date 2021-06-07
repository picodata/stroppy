package kubernetes

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ansel1/merry"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"golang.org/x/crypto/ssh"
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

func CreateKubernetes(wd string,
	terraformAddressMap terraform.MapAddresses,
	sshClient *engineSsh.Client, privateKeyFile string, provider string) (k *Kubernetes, err error) {

	k = &Kubernetes{
		workingDirectory:  wd,
		privateKeyFile:    privateKeyFile,
		clusterConfigFile: filepath.Join(wd, "config"),

		addressMap: terraformAddressMap,
		sc:         sshClient,

		provider: provider,
	}

	return
}

type Kubernetes struct {
	workingDirectory  string
	privateKeyFile    string
	clusterConfigFile string

	addressMap terraform.MapAddresses
	sshTunnel  engineSsh.Result
	sc         *engineSsh.Client

	portForward engine.ClusterTunnel

	provider string
}

func (k *Kubernetes) Deploy() (pPortForward *engine.ClusterTunnel, port int, err error) {
	if err = k.copyToMaster(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to сopy RSA to cluster")
	}

	if err = k.deploy(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to deploy k8s")
	}

	if err = k.copyConfigFromMaster(); err != nil {
		return nil, 0, merry.Prepend(err, "failed to copy kube config from Master")
	}

	sshTunnelChan := make(chan engineSsh.Result)
	portForwardChan := make(chan engine.ClusterTunnel)
	go k.openSSHTunnel(sshTunnelChan)

	k.sshTunnel = <-sshTunnelChan
	if k.sshTunnel.Err != nil {
		return nil, 0, merry.Prepend(k.sshTunnel.Err, "failed to create ssh tunnel")
	}

	go k.openMonitoringPortForward(portForwardChan)
	portForward := <-portForwardChan
	llog.Println(portForward)
	if portForward.Error != nil {
		return nil, 0, merry.Prepend(portForward.Error, "failed to port forward")
	}

	port = k.sshTunnel.Port
	pPortForward = &portForward
	return
}

func (k *Kubernetes) executeCommand(text string) (err error) {
	var commandSessionObject *ssh.Session
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

func (k *Kubernetes) runCommand(text string) (stdout io.Reader, session *ssh.Session, err error) {
	if session, err = k.sc.GetNewSession(); err != nil {
		err = merry.Prepend(err, "failed to open ssh connection")
		return
	}

	if stdout, err = session.StdoutPipe(); err != nil {
		err = merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")

		if err = session.Close(); err != nil {
			llog.Warnf("runCommand: k8s ssh session can not closed: %v", err)
		}
	}

	return
}

func (k *Kubernetes) ExecuteCommand(text string) (err error) {
	err = k.executeCommand(text)
	return
}

// OpenPortForward - открыть port-forward туннель для вызывающей функции(caller)
func (k *Kubernetes) OpenPortForward(caller string, ports []string, reqURL *url.URL,
	stopPortForward chan struct{}, readyPortForward chan struct{}, errorPortForward chan error) {
	llog.Printf("Opening of port-forward of %v...\n", caller)

	config, err := k.getKubeConfig()
	if err != nil {
		llog.Errorf("failed to get kubeconfig for open port-forward of %v: %v", caller, err)
		errorPortForward <- err
	}

	httpTransaction, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		llog.Errorf("failed to create http transction for port-forward of %v: %v\n", caller, err)
		errorPortForward <- err
	}

	portForwardLog, err := os.OpenFile("portForwardPostgres.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		llog.Errorf("failed to create or open log file for port-forward of %v: %v", caller, err)
		errorPortForward <- err
	}
	defer portForwardLog.Close()

	//nolint:exhaustivestruct
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: httpTransaction}, http.MethodPost, reqURL)
	portForward, err := portforward.New(dialer, ports,
		stopPortForward, readyPortForward, portForwardLog, portForwardLog)
	if err != nil {
		llog.Errorf("failed to get port-forwarder of %v: %v\n", caller, err)
		errorPortForward <- err
	}

	err = portForward.ForwardPorts()
	defer close(stopPortForward)
	if err != nil {
		llog.Errorf("failed to open port-forward of %v: %v\n", caller, err)
		errorPortForward <- err
	}
}

// openMonitoringPortForward
// запустить kubectl port-forward для доступа к мониторингу кластера с локального хоста
func (k *Kubernetes) openMonitoringPortForward(portForwardChan chan engine.ClusterTunnel) {
	// проверяем доступность портов 8080 и 8081 на локальной машине
	llog.Infof("Checking the status of port '%d' of the localhost for monitoring...", clusterMonitoringPort)
	monitoringPort := clusterMonitoringPort
	if !engine.IsLocalPortOpen(clusterMonitoringPort) {
		llog.Infoln("Checking the status of port 8081 of the localhost for monitoring...")

		// проверяем доступность резервного порта
		if !engine.IsLocalPortOpen(reserveClusterMonitoringPort) {
			portForwardChan <- engine.ClusterTunnel{
				Command:   nil,
				Error:     merry.Prepend(errPortCheck, ": ports 8080 and 8081 are not available"),
				LocalPort: nil,
			}
		}
		monitoringPort = reserveClusterMonitoringPort
	}

	// формируем строку с указанием портов для port-forward
	portForwardSpec := fmt.Sprintf("%v:3000", monitoringPort)
	// уровень --v=4 соответствует debug
	portForwardCmd := exec.Command("kubectl", "port-forward", "--kubeconfig=config", "--log-file=portforward.log",
		"--v=4", "deployment/grafana-stack", portForwardSpec, "-n", "monitoring")
	llog.Infof(portForwardCmd.String())
	portForwardCmd.Dir = k.workingDirectory

	// используем метод старт, т.к. нужно оставить команду запущенной в фоне
	if err := portForwardCmd.Start(); err != nil {
		llog.Infof("failed to execute command  port-forward kubectl:%v ", err)
		portForwardChan <- engine.ClusterTunnel{Command: nil, Error: err, LocalPort: nil}
	} else {
		portForwardChan <- engine.ClusterTunnel{Command: portForwardCmd, Error: nil, LocalPort: &monitoringPort}
	}
}

func getProviderDeployCommands(kubernetes *Kubernetes) (string, string, error) {
	//provider := kubernetes.
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
		fooSession *ssh.Session
		fooStdout  io.Reader
	)
	if fooStdout, fooSession, err = k.runCommand(fooStepCommand); err != nil {
		return merry.Prepend(err, "failed foo step deploy k8s")
	}
	go engine.HandleReader(bufio.NewReader(fooStdout))

	llog.Infof("Waiting for deploying about 20 minutes...")
	fooSessionResult, err := fooSession.CombinedOutput(fooStepCommand)
	if err != nil {
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

	sshClient, err := engineSsh.CreateClient(k.workingDirectory, masterExternalIP, k.provider, k.privateKeyFile)
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
func (k *Kubernetes) openSSHTunnel(sshTunnelChan chan engineSsh.Result) {
	mastersConnectionString := fmt.Sprintf("ubuntu@%v", k.addressMap.MasterExternalIP)

	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Infoln("Checking the status of port 6443 of the localhost for k8s...")
	k8sPort := clusterK8sPort
	if !engine.IsLocalPortOpen(k8sPort) {
		llog.Infoln("Checking the status of port 6444 of the localhost for k8s...")
		// проверяем резервный порт в случае недоступности основного
		k8sPort = reserveClusterK8sPort
		if !engine.IsLocalPortOpen(k8sPort) {
			sshTunnelChan <- engineSsh.Result{
				Port:   0,
				Tunnel: nil,
				Err:    merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available"),
			}
		}

		// подменяем порт в kubeconfig на локальной машине
		clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
		if err := k.editClusterURL(clusterURL); err != nil {
			llog.Infof("failed to replace port: %v", err)
			sshTunnelChan <- engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
		}
	}

	privateKeyFilePath := filepath.Join(k.workingDirectory, k.privateKeyFile)
	authMethod, err := sshtunnel.PrivateKeyFile(privateKeyFilePath)
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		sshTunnelChan <- engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
		return
	}

	// Setup the tunnel, but do not yet start it yet.
	var tunnel *sshtunnel.SSHTunnel
	tunnel, err = sshtunnel.NewSSHTunnel(
		mastersConnectionString,
		fmt.Sprintf("localhost:%v", k8sPort),
		k8sPort,
		authMethod,
	)
	if err != nil {
		sshTunnelChan <- engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to create tunnel"),
		}
		return
	}

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	tunnelStartedChan := make(chan error, 1)
	go tunnel.Start(tunnelStartedChan)
	tunnelStarted := <-tunnelStartedChan
	close(tunnelStartedChan)

	if tunnelStarted != nil {
		sshTunnelChan <- engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to start tunnel"),
		}
		return
	}

	sshTunnelChan <- engineSsh.Result{Port: k8sPort, Tunnel: tunnel, Err: nil}
}

// copyConfigFromMaster
// скопировать файл kube config c мастер-инстанса кластера и применить для использования
func (k *Kubernetes) copyConfigFromMaster() (err error) {
	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", k.addressMap.MasterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", k.privateKeyFile, "-o", "StrictHostKeyChecking=no", connectCmd, ".")
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

/* copyToMaster
 * скопировать на мастер-ноду private key для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
func (k *Kubernetes) copyToMaster() (err error) {

	privateKeyFile, err := engineSsh.GetPrivateKeyFile(k.provider, k.workingDirectory)
	if err != nil {
		return merry.Prepend(err, "failed to get private key file")
	}

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

	mastersConnectionString := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.ssh", masterExternalIP)
	copyPrivateKeyCmd := exec.Command("scp", "-i", privateKeyFile, "-o", "StrictHostKeyChecking=no",
		privateKeyFile, mastersConnectionString)
	llog.Infof(copyPrivateKeyCmd.String())
	copyPrivateKeyCmd.Dir = k.workingDirectory

	// делаем переповтор на случай проблем с кластером
	// TO DO: https://gitlab.com/picodata/stroppy/-/issues/4
	for i := 0; i <= connectionRetryCount; i++ {
		copyMasterCmdResult, err := copyPrivateKeyCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("failed to copy private key key onto master: %v %v \n", string(copyMasterCmdResult), err)
			copyPrivateKeyCmd = exec.Command("scp", "-i", privateKeyFile, "-o", "StrictHostKeyChecking=no",
				privateKeyFile, mastersConnectionString)
			time.Sleep(execTimeout * time.Second)
			continue
		}
		llog.Tracef("result of copy private key: %v \n", string(copyMasterCmdResult))
		break
	}

	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение, поэтому ssh.InsecureIgnoreHostKey

	privateKeyFilePath := fmt.Sprintf("%v/%v", k.workingDirectory, privateKeyFile)
	//nolint:gosec
	clientSSHConfig, _ := auth.PrivateKey("ubuntu", privateKeyFilePath, ssh.InsecureIgnoreHostKey())
	masterAddressPort := fmt.Sprintf("%v:22", masterExternalIP)

	client := scp.NewClient(masterAddressPort, &clientSSHConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	metricsServerFileDir := filepath.Join(k.workingDirectory, "metrics-server.yaml")

	metricsServerFile, _ := os.Open(metricsServerFileDir)
	err = client.CopyFile(metricsServerFile, "/home/ubuntu/metrics-server.yaml", "0664")
	if err != nil {
		metricsServerFile.Close()
		return merry.Prepend(err, "error while copying file metrics-server.yaml")
	}
	metricsServerFile.Close()
	client.Close()
	llog.Infoln("copying metrics-server.yaml: success")

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	ingressGrafanaFileDir := filepath.Join(k.workingDirectory, "ingress-grafana.yaml")
	ingressGrafanaFile, _ := os.Open(ingressGrafanaFileDir)
	err = client.CopyFile(ingressGrafanaFile, "/home/ubuntu/ingress-grafana.yaml", "0664")
	if err != nil {
		ingressGrafanaFile.Close()
		return merry.Prepend(err, "error while copying file ingress-grafana.yaml")
	}

	ingressGrafanaFile.Close()
	client.Close()
	llog.Infoln("copying ingress-grafana.yaml: success")

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	postgresManifestFileDir := filepath.Join(k.workingDirectory, "postgres-manifest.yaml")
	postgresManifestFile, _ := os.Open(postgresManifestFileDir)
	err = client.CopyFile(postgresManifestFile, "/home/ubuntu/postgres-manifest.yaml", "0664")
	if err != nil {
		postgresManifestFile.Close()
		return merry.Prepend(err, "error while copying file postgres-manifest.yaml")
	}

	postgresManifestFile.Close()
	client.Close()
	llog.Infoln("copying postgres-manifest.yaml: success")

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	postgresDeployFilePath := filepath.Join(k.workingDirectory, "deploy_operator.sh")
	postgresDeployFile, _ := os.Open(postgresDeployFilePath)
	err = client.CopyFile(postgresDeployFile, "/home/ubuntu/deploy_operator.sh", "0664")
	if err != nil {
		postgresManifestFile.Close()
		return merry.Prepend(err, "error while copying file deploy_operator.sh")
	}

	postgresManifestFile.Close()
	client.Close()
	llog.Infoln("copying deploy_operator.sh: success")

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	fdbClusterClientFileDir := filepath.Join(k.workingDirectory, "cluster_with_client.yaml")
	fdbClusterClientFile, _ := os.Open(fdbClusterClientFileDir)
	err = client.CopyFile(fdbClusterClientFile, "/home/ubuntu/cluster_with_client.yaml", "0664")
	if err != nil {
		fdbClusterClientFile.Close()
		return merry.Prepend(err, "error while copying file cluster_with_client.yaml")
	}

	fdbClusterClientFile.Close()
	client.Close()
	llog.Infoln("copying cluster_with_client.yaml: success")

	return
}

func (k *Kubernetes) Stop() {
	llog.Infof("Closing of port-forward...")
	/* в нормальном случае wait вернет -1, т.к. после дестроя кластера до завершения stroppy
	процесс port-forward зависает как зомби и wait делает его kill
	*/

	defer k.sshTunnel.Tunnel.Close()

	closeStatus, err := k.portForward.Command.Process.Wait()
	if err != nil {
		llog.Infof("failed to close port-forward channel: %v", err)
	}

	// если вдруг что-то пошло не так, то kill принудительно до победного либо до истечения кол-ва попыток
	for i := 0; closeStatus.ExitCode() != -1 || i < connectionRetryCount; i++ {
		llog.Errorf("port-forward is not closed. Executing kill...")
		err = k.portForward.Command.Process.Kill()
		if err != nil {
			// если процесс уже убит
			if errors.Is(err, os.ErrProcessDone) {
				llog.Infoln("status of port-forward's kill: success")
				break
			}
			llog.Printf("status of port-forward's kill: %v. Repeat...", err)
		}
		time.Sleep(engine.ExecTimeout * time.Second)
	}

	llog.Infoln("status of port-forward's close: success")
}
