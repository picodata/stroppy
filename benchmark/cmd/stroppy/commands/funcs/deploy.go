package funcs

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gitlab.com/picodata/benchmark/stroppy/internal/database/config"
	"gitlab.com/picodata/benchmark/stroppy/pkg/sshtunnel"
	"gitlab.com/picodata/benchmark/stroppy/pkg/statistics"

	"github.com/ansel1/merry"
	scp "github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	"golang.org/x/crypto/ssh"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/tools/clientcmd"
)

const terraformWorkDir = "deploy/"

const configFile = "deploy/test_config.json"

// кол-во попыток подключения при ошибке
const connectionRetryCount = 3

// задержка для случаев ожидания переповтора или соблюдения порядка запуска
const execTimeout = 5

// размер ответа terraform show при незапущенном кластере
const linesNotInitTerraformShow = 13

const templatesFile = "deploy/templates.yml"

// кол-во подов при успешном деплое k8s в master-ноде
const runningPodsCount = 41

const sshNotFoundCode = 127

const clusterMonitoringPort = 8080

const reserveClusterMonitoringPort = 8081

const clusterK8sPort = 6443

const reserveClusterK8sPort = 6444

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

/*
структура хранит результат открытия port-forward туннеля к кластеру:
cmd - структура, которая хранит атрибуты команды, которая запустила туннель
err - возможная ошибка при открытии туннеля
localPort - порт локальной машины для туннеля
*/
type tunnelToCluster struct {
	cmd       *exec.Cmd
	err       error
	localPort *int
}

var errPortCheck = errors.New("port Check failed")

type TemplatesConfig struct {
	Yandex Configurations
}

type Configurations struct {
	Small    []ConfigurationUnitParams
	Standard []ConfigurationUnitParams
	Large    []ConfigurationUnitParams
	Xlarge   []ConfigurationUnitParams
	Xxlarge  []ConfigurationUnitParams
	Maximum  []ConfigurationUnitParams
}

type ConfigurationUnitParams struct {
	Description string
	Platform    string
	CPU         int
	RAM         int
	Disk        int
}

// installTerraform - установить terraform, если не установлен
func installTerraform() error {
	llog.Infoln("Preparing the installation terraform...")

	downloadArchiveCmd := exec.Command("curl", "-O",
		"https://releases.hashicorp.com/terraform/0.14.7/terraform_0.14.7_linux_amd64.zip")
	downloadArchiveCmd.Dir = terraformWorkDir
	err := downloadArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to download archive of terraform")
	}

	unzipArchiveCmd := exec.Command("unzip", "terraform_0.14.7_linux_amd64.zip")
	llog.Infoln(unzipArchiveCmd.String())
	unzipArchiveCmd.Dir = terraformWorkDir
	err = unzipArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to unzip archive of terraform")
	}

	rmArchiveCmd := exec.Command("rm", "terraform_0.14.7_linux_amd64.zip")
	rmArchiveCmd.Dir = terraformWorkDir
	err = rmArchiveCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to remove archive of terraform")
	}

	installCmd := exec.Command("sudo", "install", "terraform", "/usr/bin/terraform")
	llog.Infoln(installCmd.String())
	installCmd.Dir = terraformWorkDir
	err = installCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to install terraform")
	}

	tabCompleteCmd := exec.Command("terraform", "-install-autocomplete")
	llog.Infoln(tabCompleteCmd.String())
	tabCompleteCmd.Dir = terraformWorkDir
	err = tabCompleteCmd.Run()
	if err != nil {
		return merry.Prepend(err, "failed to add Tab complete to terraform")
	}

	return nil
}

// terraformInit - подготовить среду для развертывания
func terraformInit() error {
	llog.Infoln("Initializating terraform...")

	initCmd := exec.Command("terraform", "init")
	initCmd.Dir = terraformWorkDir
	initCmdResult := initCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}

	llog.Infoln("Terraform initialized")
	return nil
}

var errChooseConfig = errors.New("failed to choose configuration. Unexpected configuration cluster template")

// terraformPrepare - заполнить конфиг провайдера (for example yandex_compute_instance_group.tf)
func terraformPrepare(templatesConfig TemplatesConfig, settings *config.DeploySettings) error {
	var templatesInit []ConfigurationUnitParams

	flavor := settings.Flavor
	switch flavor {
	case "small":
		templatesInit = templatesConfig.Yandex.Small
	case "standard":
		templatesInit = templatesConfig.Yandex.Standard
	case "large":
		templatesInit = templatesConfig.Yandex.Large
	case "xlarge":
		templatesInit = templatesConfig.Yandex.Xlarge
	case "xxlarge":
		templatesInit = templatesConfig.Yandex.Xxlarge
	default:
		return merry.Wrap(errChooseConfig)
	}

	err := Prepare(templatesInit[2].CPU,
		templatesInit[3].RAM,
		templatesInit[4].Disk,
		templatesInit[1].Platform,
		settings.Nodes)
	if err != nil {
		return merry.Wrap(err)
	}

	return nil
}

// terraformApply - развернуть кластер
func terraformApply() error {
	checkLaunchTerraform := exec.Command("terraform", "show")
	checkLaunchTerraform.Dir = terraformWorkDir

	checkLaunchTerraformResult, err := checkLaunchTerraform.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to Check terraform applying")
	}

	// при незапущенном кластера terraform возвращает пустую строку длиной 13 символов, либо no state c пробелами до 13
	if len(checkLaunchTerraformResult) > linesNotInitTerraformShow {
		llog.Infof("terraform already applied, deploy continue...")
		return nil
	}

	llog.Infoln("Applying terraform...")
	applyCMD := exec.Command("terraform", "apply", "-auto-approve")
	applyCMD.Dir = terraformWorkDir
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

	stdout, err := destroyCmd.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdout pipe for logging destroy of cluster")
	}

	stdoutReader := bufio.NewReader(stdout)
	go handleReader(stdoutReader)

	llog.Infoln("Destroying terraform...")
	initCmdResult := destroyCmd.Run()
	if initCmdResult != nil {
		return merry.Wrap(initCmdResult)
	}

	llog.Infoln("Terraform destroyed")
	return nil
}

func getIPMapping() (mapAddresses, error) {
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
	mapIP, err := getIPMapping()
	if err != nil {
		return merry.Prepend(err, "failed to map IP addresses in terraform.tfstate")
	}

	// проверяем наличие файла id_rsa
	privateKeyFile := fmt.Sprintf("%s/id_rsa", terraformWorkDir)
	_, err = os.Stat(privateKeyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return merry.Prepend(err, "private key file not found. Create it, please.")
		}
		return merry.Prepend(err, "failed to find private key file")
	}

	/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
	если кластер пока не готов*/
	masterExternalIP := mapIP.masterExternalIP
	llog.Infoln("Checking status of port 22 on the cluster's master...")
	var masterPortAvailable bool
	for i := 0; i <= connectionRetryCount; i++ {
		masterPortAvailable = isRemotePortOpen(masterExternalIP, 22)
		if !masterPortAvailable {
			llog.Infof("status of Check the master's port 22:%v. Repeat #%v", errPortCheck, i)
			time.Sleep(execTimeout * time.Second)
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
	copyPrivateKeyCmd.Dir = terraformWorkDir

	// делаем переповтор на случай проблем с кластером
	// TO DO: https://gitlab.com/picodata/stroppy/-/issues/4
	for i := 0; i <= connectionRetryCount; i++ {
		copyMasterCmdResult, err := copyPrivateKeyCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("failed to copy RSA key onto master: %v %v \n", string(copyMasterCmdResult), err)
			copyPrivateKeyCmd = exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no",
				"id_rsa", mastersConnectionString)
			time.Sleep(execTimeout * time.Second)
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

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
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

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
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

	client = scp.NewClient(masterAddressPort, &clientSSHConfig)
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

// deployKuberneters - развернуть k8s внутри кластера в cloud
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
		return merry.Prepend(err, "failed to Check deploy k8s in master node")
	}
	if checkDeploy {
		llog.Infoln("k8s already success deployed")
		return nil
	}
	mapIP, err := getIPMapping()
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
		return merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")
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
	mapIP, err := getIPMapping()
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

	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	clusterURL := "https://localhost:6443"
	err = editClusterURL(clusterURL)
	if err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	return nil
}

type sshResult struct {
	Port   int
	Tunnel *sshtunnel.SSHTunnel
	Err    error
}

// openSSHTunnel - открыть ssh-соединение и передать указатель на него вызывающему коду для управления
func openSSHTunnel(sshTunnelChan chan sshResult) {
	mapIP, err := getIPMapping()
	if err != nil {
		log.Printf("failed to get IP addresses for open ssh tunnel:%v ", err)
		sshTunnelChan <- sshResult{0, nil, err}
	}
	mastersConnectionString := fmt.Sprintf("ubuntu@%v", mapIP.masterExternalIP)

	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Infoln("Checking the status of port 6443 of the localhost for k8s...")
	k8sPort := clusterK8sPort
	if !isLocalPortOpen(k8sPort) {
		llog.Infoln("Checking the status of port 6444 of the localhost for k8s...")
		// проверяем резервный порт в случае недоступности основного
		k8sPort = reserveClusterK8sPort
		if !isLocalPortOpen(k8sPort) {
			sshTunnelChan <- sshResult{0, nil, merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available")}
		}

		// подменяем порт в kubeconfig на локальной машине
		clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
		err = editClusterURL(clusterURL)
		if err != nil {
			llog.Infof("failed to replace port: %v", err)
			sshTunnelChan <- sshResult{0, nil, err}
		}
	}

	authMethod, err := sshtunnel.PrivateKeyFile("deploy/id_rsa")
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		sshTunnelChan <- sshResult{0, nil, err}
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
		sshTunnelChan <- sshResult{0, nil, merry.Prepend(err, "failed to create tunnel")}
	}

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	tunnelStartedChan := make(chan error, 1)
	go tunnel.Start(tunnelStartedChan)
	tunnelStarted := <-tunnelStartedChan
	close(tunnelStartedChan)

	if tunnelStarted != nil {
		sshTunnelChan <- sshResult{0, nil, merry.Prepend(err, "failed to start tunnel")}
		return
	}

	sshTunnelChan <- sshResult{k8sPort, tunnel, nil}
}

// portForwardKubectl - запустить kubectl port-forward для доступа к мониторингу кластера с локального хоста
func portForwardKubectl(portForwardChan chan tunnelToCluster) {
	// проверяем доступность портов 8080 и 8081 на локальной машине
	llog.Infoln("Checking the status of port 8080 of the localhost for monitoring...")
	monitoringPort := clusterMonitoringPort
	if !isLocalPortOpen(monitoringPort) {
		llog.Infoln("Checking the status of port 8081 of the localhost for monitoring...")
		// проверяем доступность резервного порта
		monitoringPort = reserveClusterMonitoringPort
		if !isLocalPortOpen(monitoringPort) {
			portForwardChan <- tunnelToCluster{nil, merry.Prepend(errPortCheck, ": ports 8080 and 8081 are not available"), nil}
		}
	}
	// формируем строку с указанием портов для port-forward
	portForwardSpec := fmt.Sprintf("%v:3000", monitoringPort)
	// уровень --v=4 соответствует debug
	portForwardCmd := exec.Command("kubectl", "port-forward", "--kubeconfig=config", "--log-file=portforward.log",
		"--v=4", "deployment/grafana-stack", portForwardSpec, "-n", "monitoring")
	llog.Infof(portForwardCmd.String())
	portForwardCmd.Dir = terraformWorkDir

	// используем метод старт, т.к. нужно оставить команду запущенной в фоне
	if err := portForwardCmd.Start(); err != nil {
		llog.Infof("failed to execute command  port-forward kubectl:%v ", err)
		portForwardChan <- tunnelToCluster{nil, err, nil}
	}
	portForwardChan <- tunnelToCluster{portForwardCmd, nil, &monitoringPort}
}

// readCommandFromInput - прочитать стандартный ввод и запустить выбранные команды
func readCommandFromInput(portForwardStruct tunnelToCluster,
	errorExit chan error, successExit chan bool, popChan chan error, payChan chan error) {
	for {
		sc := bufio.NewScanner(os.Stdin)
		for sc.Scan() {
			consoleCmd := sc.Text()
			statistics.StatsInit()
			switch consoleCmd {
			case "quit":
				llog.Println("Exiting...")

				err := portForwardStruct.cmd.Process.Kill()
				if err != nil {
					llog.Errorf("failed to kill process port forward %v. \n Repeat...", err.Error())
				}

				err = terraformDestroy()
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

func readTemplates() (*TemplatesConfig, error) {
	var templatesConfig TemplatesConfig
	data, err := ioutil.ReadFile(templatesFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read templates.yml")
	}
	err = yaml.Unmarshal(data, &templatesConfig)
	if err != nil {
		return nil, merry.Prepend(err, "failed to unmarshall templates.yml")
	}
	return &templatesConfig, nil
}

// executePop - выполнить загрузку счетов в указанную БД
func executePop(cmdType string, databaseType string) error {
	settings, err := readConfig(cmdType, databaseType)
	if err != nil {
		return merry.Prepend(err, "failed to read config")
	}
	if err := Populate(settings); err != nil {
		llog.Errorf("%v", err)
	}
	balance, err := Check(settings, nil)
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
	sum, err := Check(settings, nil)
	if err != nil {
		llog.Errorf("%v", err)
	}

	llog.Infof("Initial balance: %v", sum)

	if err := Pay(settings); err != nil {
		llog.Errorf("%v", err)
	}
	if settings.Check {
		balance, err := Check(settings, sum)
		if err != nil {
			llog.Errorf("%v", err)
		}
		llog.Infof("Final balance: %v", balance)
	}
	return nil
}

// readConfig - прочитать конфигурационный файл test_config.json
func readConfig(cmdType string, databaseType string) (*config.DatabaseSettings, error) {
	settings := config.Defaults()
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to read config file")
	}

	settings.LogLevel = gjson.Parse(string(data)).Get("log_level").Str
	settings.BanRangeMultiplier = gjson.Parse(string(data)).Get("banRangeMultiplier").Float()
	settings.DatabaseType = databaseType
	if databaseType == "postgres" {
		settings.DBURL = "postgres://stroppy:stroppy@localhost/stroppy?sslmode=disable"
	} else if databaseType == "fdb" {
		settings.DBURL = "fdb.cluster"
	}
	if (cmdType == "postgres pop") || (cmdType == "fdb pop") {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.0").Get("pop").Get("count").Int())
	} else if (cmdType == "postgres pay") || (cmdType == "fdb pay") {
		settings.Count = int(gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("count").Int())
		settings.Check = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("Check").Bool()
		settings.ZIPFian = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("zipfian").Bool()
		settings.Oracle = gjson.Parse(string(data)).Get("cmd.1").Get("pay").Get("oracle").Bool()
	}

	return settings, nil
}

// checkDeployMaster - проверить, что все поды k8s в running, что подтверждает успешность деплоя k8s
func checkDeployMaster() (bool, error) {
	mapIP, err := getIPMapping()
	if err != nil {
		return false, merry.Prepend(err, "failed to get IP addresses for copy from master")
	}

	masterExternalIP := mapIP.masterExternalIP
	client, err := getClientSSH(masterExternalIP)
	if err != nil {
		return false, merry.Prepend(err, "failed to create ssh connection for Check deploy")
	}

	checkSession, err := client.NewSession()
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

// isLocalPortOpen - проверить доступность порта на localhost
func isLocalPortOpen(port int) bool {
	address := "localhost:" + strconv.Itoa(port)
	conn, err := net.Listen("tcp", address)
	if err != nil {
		llog.Errorf("port %v at localhost is not available \n", port)
		return false
	}
	defer conn.Close()

	return true
}

// isRemotePortOpen - проверить доступность порта на удаленной машине кластера
func isRemotePortOpen(hostname string, port int) bool {
	address := hostname + ":" + strconv.Itoa(port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		llog.Errorf("port %v at %v is not available: %v \n", port, hostname, err)
		return false
	}
	defer conn.Close()

	return true
}

func editClusterURL(url string) error {
	kubeConfigPath := "deploy/config"
	kubeConfig, err := clientcmd.LoadFromFile(kubeConfigPath)
	if err != nil {
		return merry.Prepend(err, "failed to load kubeconfig")
	}
	// меняем значение адреса кластера внутри kubeconfig
	kubeConfig.Clusters["cluster.local"].Server = url

	err = clientcmd.WriteToFile(*kubeConfig, kubeConfigPath)
	if err != nil {
		return merry.Prepend(err, "failed to write kubeconfig")
	}

	return nil
}

func closePortForward(portForward tunnelToCluster) {
	llog.Infof("Closing of port-forward...")
	/* в нормальном случае wait вернет -1, т.к. после дестроя кластера до завершения stroppy
	процесс port-forward зависает как зомби и wait делает его kill
	*/
	closeStatus, err := portForward.cmd.Process.Wait()
	if err != nil {
		llog.Infof("failed to close port-forward channel: %v", err)
	}

	// если вдруг что-то пошло не так, то kill принудительно до победного либо до истечения кол-ва попыток
	for i := 0; closeStatus.ExitCode() != -1 || i < connectionRetryCount; i++ {
		llog.Errorf("port-forward is not closed. Executing kill...")
		err = portForward.cmd.Process.Kill()
		if err != nil {
			// если процесс уже убит
			if errors.Is(err, os.ErrProcessDone) {
				llog.Infoln("status of port-forward's kill: success")
				break
			}
			log.Printf("status of port-forward's kill: %v. Repeat...", err)
		}
		time.Sleep(execTimeout * time.Second)
	}

	llog.Infoln("status of port-forward's close: success")
}

func Deploy(settings *config.DeploySettings) error {
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

	templatesConfig, err := readTemplates()
	if err != nil {
		return merry.Prepend(err, "failed to read templates.yml")
	}

	// передаем варианты и ключи выбора конфигурации для формирования файла провайдера terraform (пока yandex)
	err = terraformPrepare(*templatesConfig, settings)
	if err != nil {
		return merry.Prepend(err, "failed to prepare terraform")
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

	sshTunnelChan := make(chan sshResult)
	portForwardChan := make(chan tunnelToCluster)

	go openSSHTunnel(sshTunnelChan)
	sshTunnel := <-sshTunnelChan
	if sshTunnel.Err != nil {
		return merry.Prepend(sshTunnel.Err, "failed to create ssh tunnel")
	}
	defer sshTunnel.Tunnel.Close()

	go portForwardKubectl(portForwardChan)
	portForward := <-portForwardChan
	llog.Println(portForward)
	if portForward.err != nil {
		return merry.Prepend(portForward.err, "failed to port forward")
	}

	defer closePortForward(portForward)

	log.Printf(
		`Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
	To access Grafana use address localhost:%v.
	To access to kubernetes cluster in cloud use address localhost:%v.
	Enter "quit" to exit stroppy and destroy cluster.
	Enter "postgres pop" to start populating PostgreSQL with accounts.
	Enter "postgres pay" to start transfers test in PostgreSQL.
	Enter "fdb pop" to start populating FoundationDB with accounts.
	Enter "fdb pay" to start transfers test in FoundationDB.
	To use kubectl for access kubernetes cluster in another console 
	execute command for set environment variables KUBECONFIG before using:
	"export KUBECONFIG=$(pwd)/config"`,
		*portForward.localPort, sshTunnel.Port)

	errorExitChan := make(chan error)
	successExitChan := make(chan bool)
	popChan := make(chan error)
	payChan := make(chan error)
	go readCommandFromInput(portForward, errorExitChan, successExitChan, popChan, payChan)

	select {
	case err = <-errorExitChan:
		llog.Errorf("failed to destroy cluster: %v", err)
		return err
	case success := <-successExitChan:
		llog.Infof("destroy cluster %v", success)
		return nil
	}
}
