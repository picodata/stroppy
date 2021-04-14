package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ansel1/merry"
	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ssh"
)

const terraformDeployDir = "../deploy_dev/terraform/"

const terraformWorkDir = "../deploy_dev/terraform/stroppy-deploy"

const configFile = "config.json"

// кол-во попыток подключения при ошибке
const repeatConnect = 3

// задержка для случаев ожидания переповтора или соблюдения порядка запуска
const delayForCommand = 2

// размер ответа terraform show при незапущенном кластере
const linesNotInitTerraformShow = 13

// кол-во подов при успешном деплое k8s в master-ноде
const countPodsRunning = 41

const sshNotFoundCode = 127

type mapAddresses struct {
	masterExternalIP   string
	masterInternalIP   string
	metricsExternalIP  string
	metricsInternalIP  string
	ingressExternalIP  string
	ingressInternalIP  string
	postgresExternalIP string
	postgresInternalIP string
}

type chanSSHTunnel struct {
	cmd *exec.Cmd
	err error
}

type chanPortForward struct {
	cmd *exec.Cmd
	err error
}

// installTerraform - установить terraform, если не установлен
func installTerraform() error {
	llog.Infoln("Preparing the installation terraform...")
	downloadArchiveCmd := exec.Command("curl", "-O",
		"https://releases.hashicorp.com/terraform/0.14.7/terraform_0.14.7_linux_amd64.zip")
	downloadArchiveCmd.Dir = terraformDeployDir
	err := downloadArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to download archive of terraform")
	}
	unzipArchiveCmd := exec.Command("unzip", "terraform_0.14.7_linux_amd64.zip")
	llog.Infoln(unzipArchiveCmd.String())
	unzipArchiveCmd.Dir = terraformDeployDir
	err = unzipArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to unzip archive of terraform")
	}
	rmArchiveCmd := exec.Command("rm", "terraform_0.14.7_linux_amd64.zip")
	rmArchiveCmd.Dir = terraformDeployDir
	err = rmArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to remove archive of terraform")
	}
	installCmd := exec.Command("sudo", "install", "terraform", "/usr/bin/terraform")
	llog.Infoln(installCmd.String())
	installCmd.Dir = terraformDeployDir
	err = installCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to install terraform")
	}
	tabCompleteCmd := exec.Command("terraform", "-install-autocomplete")
	llog.Infoln(tabCompleteCmd.String())
	tabCompleteCmd.Dir = terraformDeployDir
	err = tabCompleteCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to add Tab complete to terraform")
	}
	return nil
}

// terraformInit - подготовить среду для развертывания
func terraformInit() error {
	initCmd := exec.Command("terraform", "init")
	initCmd.Dir = terraformWorkDir
	llog.Infoln("Initializating terraform...")
	initCmdResult := initCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}
	llog.Infoln("Terraform initialized")
	return nil
}

// terraformApply - развернуть кластер
func terraformApply() error {
	checkLaunchTerraform := exec.Command("terraform", "show")
	checkLaunchTerraform.Dir = terraformWorkDir
	checkLaunchTerraformResult, err := checkLaunchTerraform.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to check terraform applying")
	}
	// при незапущенном кластера terraform возвращает пустую строку длиной 13 символов, либо no state c пробелами до 13
	if len(checkLaunchTerraformResult) > linesNotInitTerraformShow {
		llog.Infof("terraform already applied, deploy continue...")
		return nil
	}
	applyCMD := exec.Command("terraform", "apply", "-auto-approve")
	applyCMD.Dir = terraformWorkDir
	llog.Infoln("Applying terraform...")
	result, err := applyCMD.CombinedOutput()
	if err != nil {
		llog.Errorln(string(result))
		return merry.Wrap(err)
	}
	log.Printf("Terraform applied")
	return nil
}

// terraformDestroy - уничтожить кластер
func terraformDestroy() error {
	destroyCmd := exec.Command("terraform", "destroy", "-force")
	destroyCmd.Dir = terraformWorkDir
	llog.Infoln("Destroying terraform...")
	initCmdResult := destroyCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}
	llog.Infoln("Terraform destroyed")
	return nil
}

func mappingIP() (mapAddresses, error) {
	/*
		Функция парсит файл terraform.tfstate и возвращает массив ip. У каждого экземпляра
		 своя пара - внешний (NAT) и внутренний ip.
		 Для парсинга используется сторонняя библиотека gjson - https://github.com/tidwall/gjson,
		  т.к. использование encoding/json
		влечет создание группы структур большого размера, что ухудшает читаемость. Метод Get возвращает gjson.Result
		по переданному тегу json, который можно преобразовать в том числе в строку.
	*/
	var mapIP mapAddresses
	tsStateWorkDir := fmt.Sprintf("%v/terraform.tfstate", terraformWorkDir)
	data, err := ioutil.ReadFile(tsStateWorkDir)
	if err != nil {
		return mapIP, merry.Prepend(err, "failed to read file terraform.tfstate")
	}
	masterExternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterExternalIP := masterExternalIPArray.Get("attributes").Get("network_interface.0").Get("nat_ip_address")
	mapIP.masterExternalIP = masterExternalIP.Str
	masterInternalIPArray := gjson.Parse(string(data)).Get("resources.1").Get("instances.0")
	masterInternalIP := masterInternalIPArray.Get("attributes").Get("network_interface.0").Get("ip_address")
	mapIP.masterInternalIP = masterInternalIP.Str
	metricsExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsExternalIP := metricsExternalIPArray.Get("instances.0").Get("network_interface.0").Get("nat_ip_address")
	mapIP.metricsExternalIP = metricsExternalIP.Str
	metricsInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	metricsInternalIP := metricsInternalIPArray.Get("instances.0").Get("network_interface.0").Get("ip_address")
	mapIP.metricsInternalIP = metricsInternalIP.Str
	ingressExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressExternalIP := ingressExternalIPArray.Get("instances.1").Get("network_interface.0").Get("nat_ip_address")
	mapIP.ingressExternalIP = ingressExternalIP.Str
	ingressInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	ingressInternalIP := ingressInternalIPArray.Get("instances.1").Get("network_interface.0").Get("ip_address")
	mapIP.ingressInternalIP = ingressInternalIP.Str
	postgresExternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresExternalIP := postgresExternalIPArray.Get("instances.2").Get("network_interface.0").Get("nat_ip_address")
	mapIP.postgresExternalIP = postgresExternalIP.Str
	postgresInternalIPArray := gjson.Parse(string(data)).Get("resources.2").Get("instances.0").Get("attributes")
	postgresInternalIP := postgresInternalIPArray.Get("instances.2").Get("network_interface.0").Get("ip_address")
	mapIP.postgresInternalIP = postgresInternalIP.Str
	return mapIP, nil
}

func getClientSSH(ipAddress string) (*ssh.Client, error) {
	privateKeyFile := fmt.Sprintf("%s/id_rsa", terraformWorkDir)
	privateKeyRaw, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get id_rsa for ssh client")
	}

	signer, err := ssh.ParsePrivateKey(privateKeyRaw)
	if err != nil {
		return nil, merry.Prepend(err, "failed to parse id_rsa for ssh client")
	}
	// линтер требует указания всех полей структуры при присвоении переменной
	//nolint:exhaustivestruct
	config := &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		//nolint:gosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", ipAddress, 22)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, merry.Prepend(err, "failed to start ssh connection for ssh client")
	}
	return client, nil
}

/*copyToMaster - скопировать на мастер-ноду ключ id_rsa для работы мастера с воркерами
и файлы для развертывания мониторинга и postgres
*/
func copyToMaster() error {
	/**/
	mapIP, err := mappingIP()
	if err != nil {
		return merry.Prepend(err, "failed to map IP addresses in terraform.tfstate")
	}
	masterExternalIP := mapIP.masterExternalIP
	connectMasterString := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.ssh", masterExternalIP)
	copyMasterCmd := exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no", "id_rsa", connectMasterString)
	llog.Infof(copyMasterCmd.String())
	copyMasterCmd.Dir = terraformWorkDir
	i := 0
	// делаем переповтор на случай проблем с кластером
	for i <= repeatConnect {
		resultcopyMasterCmd, err := copyMasterCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("falied to connect for copy RSA: %v %v \n", string(resultcopyMasterCmd), err)
			i++
			copyMasterCmd = exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no", "id_rsa", connectMasterString)
			time.Sleep(delayForCommand * time.Second)
			continue
		}
		llog.Tracef("result of copy RSA: %v \n", string(resultcopyMasterCmd))
		break
	}
	privateKeyFile := fmt.Sprintf("%s/id_rsa", terraformWorkDir)
	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение
	//nolint:gosec
	clientConfig, _ := auth.PrivateKey("ubuntu", privateKeyFile, ssh.InsecureIgnoreHostKey())
	masterAddressPort := fmt.Sprintf("%v:22", masterExternalIP)
	client := scp.NewClient(masterAddressPort, &clientConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}
	metricsServerFileDir := fmt.Sprintf("%s/metrics-server.yaml", terraformWorkDir)
	metricsServerFile, _ := os.Open(metricsServerFileDir)
	err = client.CopyFile(metricsServerFile, "/home/ubuntu/metrics-server.yaml", "0664")
	if err != nil {
		metricsServerFile.Close()
		return merry.Prepend(err, "error while copying file metrics-server.yaml")
	}
	metricsServerFile.Close()
	client.Close()
	llog.Infoln("copying metrics-server.yaml: success")
	client = scp.NewClient(masterAddressPort, &clientConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}
	ingressGrafanaFileDir := fmt.Sprintf("%s/ingress-grafana.yaml", terraformWorkDir)
	ingressGrafanaFile, _ := os.Open(ingressGrafanaFileDir)
	err = client.CopyFile(ingressGrafanaFile, "/home/ubuntu/ingress-grafana.yaml", "0664")
	if err != nil {
		ingressGrafanaFile.Close()
		return merry.Prepend(err, "error while copying file ingress-grafana.yaml")
	}
	ingressGrafanaFile.Close()
	client.Close()
	llog.Infoln("copying ingress-grafana.yaml: success")
	client = scp.NewClient(masterAddressPort, &clientConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}
	postgresManifestFileDir := fmt.Sprintf("%s/postgres-manifest.yaml", terraformWorkDir)
	postgresManifestFile, _ := os.Open(postgresManifestFileDir)
	err = client.CopyFile(postgresManifestFile, "/home/ubuntu/postgres-manifest.yaml", "0664")
	if err != nil {
		postgresManifestFile.Close()
		return merry.Prepend(err, "error while copying file postgres-manifest.yaml")
	}
	postgresManifestFile.Close()
	client.Close()
	llog.Infoln("copying postgres-manifest.yaml: success")
	client = scp.NewClient(masterAddressPort, &clientConfig)
	err = client.Connect()
	if err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}
	fdbClusterClientFileDir := fmt.Sprintf("%s/cluster_with_client.yaml", terraformWorkDir)
	fdbClusterClientFile, _ := os.Open(fdbClusterClientFileDir)
	err = client.CopyFile(fdbClusterClientFile, "/home/ubuntu/cluster_with_client.yaml", "0664")
	if err != nil {
		fdbClusterClientFile.Close()
		return merry.Prepend(err, "error while copying file postgres-manifest.yaml")
	}
	fdbClusterClientFile.Close()
	client.Close()
	llog.Infoln("copying cluster_with_client.yaml: success")
	return nil
}

const deployk8sFirstStepCmd = `echo \
"sudo apt-get update
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update
sudo apt-get install helm
#add by @nik_sav
sudo apt-get install dialog apt-utils
echo 'sudo debconf debconf/frontend select Noninteractive' | debconf-set-selections
#end add by @nik_sav
sudo apt-get update
git clone --branch v2.15.0 https://github.com/kubernetes-sigs/kubespray.git
cd kubespray
sudo pip3 install -r requirements.txt
rm inventory/local/hosts.ini" | tee deploy_kubernetes.sh
`

//nolint:lll
const deployk8sThirdStepCmd = `echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' \
inventory/local/group_vars/k8s-cluster/addons.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s-cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
kubectl create namespace monitoring
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install loki grafana/loki-stack --namespace monitoring
helm install grafana-stack prometheus-community/kube-prometheus-stack --namespace monitoring
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml
kubectl apply -f \
https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/v0.31.1/config/samples/deployment.yaml
echo "Waiting foundationdb deploy for 60 seconds..."
sleep 60
kubectl apply -f /home/ubuntu/cluster_with_client.yaml
kubectl apply -f /home/ubuntu/metrics-server.yaml
kubectl apply -f /home/ubuntu/ingress-grafana.yaml
kubectl apply -f \
https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml" \
 | tee -a deploy_kubernetes.sh
`

func deployKuberneters() error {
	/*
		Последовательно формируем файл deploy_kubernetes.sh,
		даем ему права на выполнение и выполняем.
		1-й шаг - добавляем первую часть команд (deployk8sFirstStepCmd)
		2-й шаг - подставляем ip адреса в hosts.ini и добавляем команду с его записью в файл
		3-й шаг - добавляем вторую часть команд (deployk8sThirdStepCmd)
		4-й шаг - выдаем файлу права на выполнение и выполняем
	*/
	checkDeploy, err := checkDeployMaster()
	if err != nil {
		return merry.Prepend(err, "failed to check deploy k8s in master node")
	}
	if checkDeploy {
		llog.Infoln("k8s already success deployed")
		return nil
	}
	mapIP, err := mappingIP()
	if err != nil {
		return merry.Prepend(err, "failed to get IP addresses for deploy k8s")
	}
	client, err := getClientSSH(mapIP.masterExternalIP)
	if err != nil {
		return merry.Prepend(err, "failed to get ssh client for deploy k8s")
	}
	deployOneStep, err := client.NewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for first step deploy")
	}
	_, err = deployOneStep.CombinedOutput(deployk8sFirstStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed first step deploy k8s")
	}
	log.Printf("First step deploy k8s: success")
	deployOneStep.Close()
	deploySecondStep, err := client.NewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for second step deploy")
	}
	deployk8sSecondStepCmd := fmt.Sprintf(`echo \
"tee inventory/local/hosts.ini<<EOF
[all]
master ansible_host=%v ip=%v etcd_member_name=etcd1
worker-1 ansible_host=%v ip=%v etcd_member_name=etcd2
worker-2 ansible_host=%v ip=%v etcd_member_name=etcd3
worker-3 ansible_host=%v ip=%v etcd_member_name=etcd4
	
[kube-master]
master
	
[etcd]
master
worker-1
worker-2
worker-3
	
[kube-node]
worker-1
worker-2
worker-3
	
[k8s-cluster:children]
kube-master
kube-node
EOF" | tee -a deploy_kubernetes.sh
`, mapIP.masterInternalIP, mapIP.masterInternalIP,
		mapIP.metricsInternalIP, mapIP.metricsInternalIP,
		mapIP.ingressInternalIP, mapIP.ingressInternalIP,
		mapIP.postgresInternalIP, mapIP.postgresInternalIP,
	)
	_, err = deploySecondStep.CombinedOutput(deployk8sSecondStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed second step deploy k8s")
	}
	log.Printf("Second step deploy k8s: success")
	deploySecondStep.Close()
	deployThirdStep, err := client.NewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for second step deploy k8s")
	}
	_, err = deployThirdStep.CombinedOutput(deployk8sThirdStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed third step deploy k8s")
	}
	log.Printf("Third step deploy k8s: success")
	deployThirdStep.Close()
	deployFooStep, err := client.NewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for third step deploy k8s")
	}
	deployFooStepCmd := "chmod +x deploy_kubernetes.sh && ./deploy_kubernetes.sh -y"
	stdout, err := deployFooStep.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}
	stdoutReader := bufio.NewReader(stdout)
	go handleReader(stdoutReader)
	llog.Infof("Waiting for deploying about 20 minutes...")
	_, err = deployFooStep.CombinedOutput(deployFooStepCmd)
	if err != nil {
		return merry.Prepend(err, "failed foo step deploy k8s")
	}
	log.Printf("Foo step deploy k8s: success")
	deployFooStep.Close()
	defer client.Close()
	return nil
}

// handleReader - вывести буфер страндартного вывода в отдельном потоке
func handleReader(reader *bufio.Reader) {
	printOutput := llog.GetLevel() == llog.InfoLevel
	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		if printOutput {
			llog.Printf(str)
		}
	}
}

// copyConfigFromMaster - скопировать файд kube config c мастер-инстанса кластера и применить для использования
func copyConfigFromMaster() error {
	mapIP, err := mappingIP()
	if err != nil {
		return merry.Prepend(err, "failed to get IP addresses for copy from master")
	}
	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", mapIP.masterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	llog.Infoln(copyFromMasterCmd.String())
	copyFromMasterCmd.Dir = terraformWorkDir
	_, err = copyFromMasterCmd.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to execute command copy from master")
	}
	cmdForSed := fmt.Sprintf("s/%v/localhost/g", mapIP.masterInternalIP)
	replaceCMD := exec.Command("sed", "-i", cmdForSed, "config")
	llog.Infoln(replaceCMD.String())
	replaceCMD.Dir = terraformWorkDir
	_, err = replaceCMD.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to execute command for sed kube config")
	}
	return nil
}

// openSSHTunnel - открыть ssh-соединение и передать указатель на него вызывающему коду для управления
func openSSHTunnel(sshTunnelChan chan chanSSHTunnel) {
	mapIP, err := mappingIP()
	if err != nil {
		log.Printf("failed to get IP addresses for open ssh tunnel:%v ", err)
		sshTunnelChan <- chanSSHTunnel{nil, err}
	}
	connectString := fmt.Sprintf("ubuntu@%v", mapIP.masterExternalIP)
	openSSHTunnel := exec.Command("ssh", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no",
		"-o", "ServerAliveInterval=60", "-N", "-L", "6443:localhost:6443", "-N", connectString)
	llog.Infof(openSSHTunnel.String())
	openSSHTunnel.Dir = terraformWorkDir
	stdout, err := openSSHTunnel.StdoutPipe()
	if err != nil {
		llog.Error("Failed creating command stdoutpipe: ", err)
		sshTunnelChan <- chanSSHTunnel{nil, err}
	}
	stdoutReader := bufio.NewReader(stdout)
	go handleReader(stdoutReader)
	err = openSSHTunnel.Start()
	if err != nil {
		log.Printf("failed to execute command open ssh tunnel: %v", err)
		sshTunnelChan <- chanSSHTunnel{nil, err}
	}
	sshTunnelChan <- chanSSHTunnel{openSSHTunnel, err}
}

// portForwardKubectl - запустить kubectl port-forward для доступа к мониторингу кластера с локального хоста
func portForwardKubectl(portForwardChan chan chanPortForward) {
	/* указываем конфиг напрямую, без задания отдельной переменной, т.к. есть проблемы с заданием переменных среды
	через exec используя команду export*/
	portForwardCmd := exec.Command("kubectl", "port-forward", "--kubeconfig=config",
		"deployment/grafana-stack", "8080:3000", "-n", "monitoring")
	llog.Infof(portForwardCmd.String())
	portForwardCmd.Dir = terraformWorkDir
	if err := portForwardCmd.Start(); err != nil {
		llog.Infof("failed to execute command  port-forward kubectl:%v ", err)
		portForwardChan <- chanPortForward{nil, err}
	}
	portForwardChan <- chanPortForward{portForwardCmd, nil}
}

// readCommandFromInput - прочитать стандартный ввод и запустить выбранные команды
func readCommandFromInput(sshTunnelStruct chanSSHTunnel, portForwardStruct chanPortForward,
	errorExit chan error, successExit chan bool, popChan chan error, payChan chan error) {
	for {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			consoleCmd := sc.Text()
			StatsInit()
			switch consoleCmd {
			case "quit":
				llog.Println("Exiting...")
				for sshTunnelStruct.cmd.ProcessState != nil && portForwardStruct.cmd.ProcessState != nil {
					err := sshTunnelStruct.cmd.Process.Kill()
					if err != nil {
						llog.Errorf("failed to kill process ssh tunel %v. \n Repeat...", err.Error())
						continue
					}
					err = portForwardStruct.cmd.Process.Kill()
					if err != nil {
						llog.Errorf("failed to kill process port forward %v. \n Repeat...", err.Error())
						continue
					}
					break
				}
				err := terraformDestroy()
				if err != nil {
					errorExit <- merry.Prepend(err, "failed to destroy terraform")
				} else {
					successExit <- true
				}
			case "postgres pop":
				{
					llog.Println("Starting accounts populating for postgres...")
					err := executePop(consoleCmd, "postgres")
					if err != nil {
						popChan <- err
					} else {
						llog.Println("Populating of accounts in postgres successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "postgres pay":
				{
					llog.Println("Starting transfer tests for postgres...")
					err := executePay(consoleCmd, "postgres")
					if err != nil {
						payChan <- err
					} else {
						llog.Println("Transfers test in postgres successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "fdb pop":
				{
					llog.Println("Starting accounts populating for fdb...")
					err := executePop(consoleCmd, "fdb")
					if err != nil {
						popChan <- err
					} else {
						llog.Println("Populating of accounts in fdb successed")
						llog.Println("Waiting enter command:")
					}
				}
			case "fdb pay":
				{
					llog.Println("Starting transfer tests for fdb...")
					err := executePay(consoleCmd, "fdb")
					if err != nil {
						payChan <- err
					} else {
						llog.Println("Transfers test in fdb successed")
						llog.Println("Waiting enter command:")
					}
				}
			default:
				llog.Printf("You entered: %v. Expected quit \n", consoleCmd)
			}
		}
	}
}

// executePop - выполнить загрузку счетов в указанную БД
func executePop(cmdType string, databaseType string) error {
	settings, err := readConfig(cmdType, databaseType)
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	if err := populate(settings); err != nil {
		llog.Errorf("%v", err)
	}
	balance, err := check(settings, nil)
	if err != nil {
		llog.Errorf("%v", err)
	}
	llog.Infof("Total balance: %v", balance)
	return nil
}

// executePay - выполнить тест переводов
func executePay(cmdType string, databaseType string) error {
	settings, err := readConfig(cmdType, databaseType)
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	sum, err := check(settings, nil)
	if err != nil {
		llog.Errorf("%v", err)
	}

	llog.Infof("Initial balance: %v", sum)

	if err := pay(settings); err != nil {
		llog.Errorf("%v", err)
	}
	if settings.check {
		balance, err := check(settings, sum)
		if err != nil {
			llog.Errorf("%v", err)
		}
		llog.Infof("Final balance: %v", balance)
	}
	return nil
}

func readConfig(cmdType string, databaseType string) (*DatabaseSettings, error) {
	settings := Defaults()
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read config file")
	}
	settings.log_level = gjson.Parse(string(data)).Get("log_level").Str
	settings.banRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.databaseType = databaseType
	if databaseType == "postgres" {
		settings.dbURL = "postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable"
	} else if databaseType == "fdb" {
		settings.dbURL = "fdb.cluster"
	}
	if (cmdType == "postgres pop") || (cmdType == "fdb pop") {
		settings.count = int(gjson.Parse(string(data)).Get("cmd.0").Get("pop").Get("count").Int())
	} else if (cmdType == "postgres pay") || (cmdType == "fdb pay") {
		settings.count = int(gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("count").Int())
		settings.check = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("check").Bool()
		settings.zipfian = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("zipfian").Bool()
		settings.oracle = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("oracle").Bool()
	}
	return &settings, nil
}

func checkDeployMaster() (bool, error) {
	mapIP, err := mappingIP()
	if err != nil {
		return false, merry.Prepend(err, "failed to get IP addresses for copy from master")
	}
	masterExternalIP := mapIP.masterExternalIP
	client, err := getClientSSH(masterExternalIP)
	if err != nil {
		return false, merry.Prepend(err, "failed to create ssh connection for check deploy")
	}
	checkSession, err := client.NewSession()
	if err != nil {
		return false, merry.Prepend(err, "failed to open ssh connection for check deploy")
	}
	checkCmd := "kubectl get pods --all-namespaces"
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
	if countPods < countPodsRunning {
		return false, nil
	}
	checkSession.Close()
	return true, nil
}

func deploy(settings DeploySettings) error {
	llog.Traceln(settings)
	checkVersionCmd, err := exec.Command("terraform", "version").Output()
	if err != nil {
		log.Printf("Failed to find terraform version")
		if errors.Is(err, exec.ErrNotFound) {
			err := installTerraform()
			if err != nil {
				log.Fatalf("Deploy terraform status: %v ", err)
			} else {
				log.Printf("Deploy terraform status: success")
			}
		}
	}
	if strings.Contains(string(checkVersionCmd), "version") {
		log.Printf("Founded version %v", string(checkVersionCmd[:17]))
	}
	err = terraformInit()
	if err != nil {
		return merry.Prepend(err, "failed to init terraform")
	}
	err = terraformApply()
	if err != nil {
		return merry.Prepend(err, "failed to apply terraform")
	}
	err = copyToMaster()
	if err != nil {
		return merry.Prepend(err, "failed to сopy RSA to cluster")
	}
	err = deployKuberneters()
	if err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}
	err = copyConfigFromMaster()
	if err != nil {
		return merry.Prepend(err, "failed to copy kube config from Master")
	}
	sshTunnelChan := make(chan chanSSHTunnel)
	go openSSHTunnel(sshTunnelChan)
	time.Sleep(delayForCommand * time.Second)
	portForwardChan := make(chan chanPortForward)
	go portForwardKubectl(portForwardChan)
	// добавляем задержку для корректного порядка вывода сообщений
	time.Sleep(delayForCommand * time.Second)
	log.Println(`Started ssh tunnel on ports 6443 for postgres and port-forward for monitoring.
	For access to Grafana use address localhost:8080.
	For access to postgres use address localhost:6443.
	Enter quit to exit stroppy and destroy cluster.
	Enter "postgres pop" to start accounts populating in postgres.
	Enter "postgres pay" to start transfers test in postgres.
	Enter "fdb pop" to start accounts populating in fdb.
	Enter "fdb pay" to start transfers test in fdb.
	For use kubectl in another console execute command for set KUBECONFIG before using:
	"export KUBECONFIG=$(pwd)/config", where $(pwd) - path where was copyed config`)
	sshTunnelStruct := <-sshTunnelChan
	if sshTunnelStruct.err != nil {
		return merry.Prepend(sshTunnelStruct.err, "failed to create ssh tunnel")
	}
	portForwardStruct := <-portForwardChan
	if portForwardStruct.err != nil {
		return merry.Prepend(portForwardStruct.err, "failed to port forward")
	}
	errorExitChan := make(chan error)
	successExitChan := make(chan bool)
	popChan := make(chan error)
	payChan := make(chan error)
	go readCommandFromInput(sshTunnelStruct, portForwardStruct, errorExitChan, successExitChan, popChan, payChan)
	select {
	case err = <-errorExitChan:
		llog.Errorf("failed to destroy cluster: %v", err)
		return err
	case success := <-successExitChan:
		llog.Infof("destroy cluster %v", success)
		return nil
	}
	/*err = terraformDestroy()
	if err != nil {
		return merry.Prepend(err, "failed to destroy terraform")
	} else {
		return nil
	}*/
}
