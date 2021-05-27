package kubernetes

import (
	"bufio"
	"errors"
	"fmt"
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

func CreateKubernetes(wd string,
	terraformAddressMap terraform.MapAddresses,
	sshClient *engineSsh.Client) (k *Kubernetes) {

	k = &Kubernetes{
		workingDirectory: wd,
		privateKeyFile:   filepath.Join(wd, "id_rsa"),
		addressMap:       terraformAddressMap,
		sc:               sshClient,
	}
	return
}

type Kubernetes struct {
	workingDirectory string
	privateKeyFile   string
	addressMap       terraform.MapAddresses
	sc               *engineSsh.Client

	portForward engine.ClusterTunnel
}

func (k *Kubernetes) Deploy() (err error) {
	if err = k.copyToMaster(); err != nil {
		return merry.Prepend(err, "failed to сopy RSA to cluster")
	}

	err = k.deploy()
	if err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	err = k.copyConfigFromMaster()
	if err != nil {
		return merry.Prepend(err, "failed to copy kube config from Master")
	}
	sshTunnelChan := make(chan engineSsh.Result)
	portForwardChan := make(chan engine.ClusterTunnel)

	go k.openSSHTunnel(sshTunnelChan)
	sshTunnel := <-sshTunnelChan
	if sshTunnel.Err != nil {
		return merry.Prepend(sshTunnel.Err, "failed to create ssh tunnel")
	}
	defer sshTunnel.Tunnel.Close()

	go k.openMonitoringPortForward(portForwardChan)
	portForward := <-portForwardChan
	llog.Println(portForward)
	if portForward.Error != nil {
		return merry.Prepend(portForward.Error, "failed to port forward")
	}

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
				nil,
				merry.Prepend(errPortCheck,
					": ports 8080 and 8081 are not available"), nil,
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
		portForwardChan <- engine.ClusterTunnel{nil, err, nil}
	}
	portForwardChan <- engine.ClusterTunnel{portForwardCmd, nil, &monitoringPort}
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

	var deployOneStep *ssh.Session
	if deployOneStep, err = k.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection for first step deploy")
	}

	if _, err = deployOneStep.CombinedOutput(deployk8sFirstStepCmd); err != nil {
		return merry.Prepend(err, "failed first step deploy k8s")
	}
	log.Printf("First step deploy k8s: success")

	if err = deployOneStep.Close(); err != nil {
		llog.Warnf("failed to close deploy first step session: %v", err)
	}

	var deploySecondStep *ssh.Session
	if deploySecondStep, err = k.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection for second step deploy")
	}

	mapIP := k.addressMap
	deployk8sSecondStepCmd := fmt.Sprintf(deployk8sSecondStepTemplate,
		mapIP.MasterInternalIP, mapIP.MasterInternalIP,
		mapIP.MetricsInternalIP, mapIP.MetricsInternalIP,
		mapIP.IngressInternalIP, mapIP.IngressInternalIP,
		mapIP.PostgresInternalIP, mapIP.PostgresInternalIP,
	)

	if _, err = deploySecondStep.CombinedOutput(deployk8sSecondStepCmd); err != nil {
		return merry.Prepend(err, "failed second step deploy k8s")
	}
	log.Printf("Second step deploy k8s: success")

	if err = deploySecondStep.Close(); err != nil {
		llog.Warnf("failed to close deploy first step session: %v", err)
	}

	deployThirdStep, err := k.sc.GetNewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for second step deploy k8s")
	}

	_, err = deployThirdStep.CombinedOutput(deployk8sThirdStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed third step deploy k8s")
	}
	log.Printf("Third step deploy k8s: success")

	if err = deployThirdStep.Close(); err != nil {
		llog.Warnf("failed to close deploy first step session: %v", err)
	}

	deployFooStep, err := k.sc.GetNewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for third step deploy k8s")
	}

	deployFooStepCmd := "chmod +x deploy_kubernetes.sh && ./deploy_kubernetes.sh -y"
	stdout, err := deployFooStep.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")
	}

	stdoutReader := bufio.NewReader(stdout)
	go engine.HandleReader(stdoutReader)

	llog.Infof("Waiting for deploying about 20 minutes...")
	_, err = deployFooStep.CombinedOutput(deployFooStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed foo step deploy k8s")
	}

	log.Printf("Foo step deploy k8s: success")
	deployFooStep.Close()

	// defer client.Close()
	return
}

// getKubeConfig - получить конфигурацию k8s
func (k *Kubernetes) getKubeConfig() (*rest.Config, error) {
	kubeConfig := "deploy/config"
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get config for check deploy of postgres")
	}
	return config, nil
}

func (k *Kubernetes) GetClientSet() (*kubernetes.Clientset, error) {
	config, err := k.getKubeConfig()
	if err != nil {
		return nil, merry.Prepend(err, "failed to get kubeconfig for clientset")
	}

	// clientset - клиент для обращения к группам сущностей k8s
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create clientset")
	}

	return clientset, nil
}

// checkDeployMaster
// проверить, что все поды k8s в running, что подтверждает успешность деплоя k8s
func (k *Kubernetes) checkDeployMaster() (bool, error) {
	masterExternalIP := k.addressMap.MasterExternalIP
	sshClient, err := engineSsh.CreateClient(k.workingDirectory, masterExternalIP)
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
				0,
				nil,
				merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available"),
			}
		}

		// подменяем порт в kubeconfig на локальной машине
		clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
		if err := editClusterURL(clusterURL); err != nil {
			llog.Infof("failed to replace port: %v", err)
			sshTunnelChan <- engineSsh.Result{0, nil, err}
		}
	}

	authMethod, err := sshtunnel.PrivateKeyFile("benchmark/deploy/id_rsa")
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		sshTunnelChan <- engineSsh.Result{0, nil, err}
	}
	// Setup the tunnel, but do not yet start it yet.
	destinationServerString := fmt.Sprintf("localhost:%v", k8sPort)
	tunnel, err := sshtunnel.NewSSHTunnel(
		mastersConnectionString,
		destinationServerString,
		k8sPort,
		authMethod,
	)
	if err != nil {
		sshTunnelChan <- engineSsh.Result{
			0,
			nil,
			merry.Prepend(err, "failed to create tunnel"),
		}
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
			0,
			nil,
			merry.Prepend(err, "failed to start tunnel"),
		}
		return
	}

	sshTunnelChan <- engineSsh.Result{k8sPort, tunnel, nil}
}

// copyConfigFromMaster
// скопировать файл kube config c мастер-инстанса кластера и применить для использования
func (k *Kubernetes) copyConfigFromMaster() (err error) {
	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", k.addressMap.MasterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	llog.Infoln(copyFromMasterCmd.String())
	copyFromMasterCmd.Dir = k.workingDirectory

	if _, err = copyFromMasterCmd.CombinedOutput(); err != nil {
		return merry.Prepend(err, "failed to execute command copy from master")
	}

	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	clusterURL := "https://localhost:6443"
	if err = editClusterURL(clusterURL); err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	return nil
}

/* copyToMaster
 * скопировать на мастер-ноду ключ id_rsa для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
func (k *Kubernetes) copyToMaster() (err error) {
	// проверяем наличие файла id_rsa

	privateKeyFile := fmt.Sprintf("%s/id_rsa", k.workingDirectory)
	if _, err = os.Stat(privateKeyFile); err != nil {
		if os.IsNotExist(err) {
			return merry.Prepend(err, "private key file not found. Create it, please.")
		}
		return merry.Prepend(err, "failed to find private key file")
	}

	/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
	если кластер пока не готов*/
	masterExternalIP := k.addressMap.MasterExternalIP
	llog.Infoln("Checking status of port 22 on the cluster's master...")
	var masterPortAvailable bool
	for i := 0; i <= connectionRetryCount; i++ {
		masterPortAvailable = engine.IsRemotePortOpen(masterExternalIP, 22)
		if !masterPortAvailable {
			llog.Infof("status of Check the master's port 22:%v. Repeat #%v", errPortCheck, i)
			time.Sleep(engine.ExecTimeout * time.Second)
		} else {
			break
		}
	}
	if !masterPortAvailable {
		return merry.Prepend(errPortCheck, "master's port 22 is not available")
	}

	mastersConnectionString := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.ssh", masterExternalIP)
	copyPrivateKeyCmd := exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no",
		"id_rsa", mastersConnectionString)
	llog.Infof(copyPrivateKeyCmd.String())
	copyPrivateKeyCmd.Dir = k.workingDirectory

	// делаем переповтор на случай проблем с кластером
	// TO DO: https://gitlab.com/picodata/stroppy/-/issues/4
	for i := 0; i <= connectionRetryCount; i++ {
		copyMasterCmdResult, err := copyPrivateKeyCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("failed to copy RSA key onto master: %v %v \n", string(copyMasterCmdResult), err)
			copyPrivateKeyCmd = exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no",
				"id_rsa", mastersConnectionString)
			time.Sleep(engine.ExecTimeout * time.Second)
			continue
		}
		llog.Tracef("result of copy RSA: %v \n", string(copyMasterCmdResult))
		break
	}

	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение, поэтому ssh.InsecureIgnoreHostKey
	//nolint:gosec
	clientSSHConfig, _ := auth.PrivateKey("ubuntu", privateKeyFile, ssh.InsecureIgnoreHostKey())
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
			log.Printf("status of port-forward's kill: %v. Repeat...", err)
		}
		time.Sleep(engine.ExecTimeout * time.Second)
	}

	llog.Infoln("status of port-forward's close: success")
}
