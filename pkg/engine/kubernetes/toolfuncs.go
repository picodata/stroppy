package kubernetes

import (
	"context"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func (k *Kubernetes) editClusterURL(url string) error {
	llog.Infoln("changing of cluster url on", url)
	kubeConfig, err := clientcmd.LoadFromFile(k.clusterConfigFile)
	if err != nil {
		return merry.Prepend(err, "failed to load kube config")
	}
	// меняем значение адреса кластера внутри kubeconfig
	kubeConfig.Clusters["cluster.local"].Server = url

	err = clientcmd.WriteToFile(*kubeConfig, k.clusterConfigFile)
	if err != nil {
		return merry.Prepend(err, "failed to write kubeconfig")
	}

	return nil
}

// getKubeConfig - получить конфигурацию k8s
func (k *Kubernetes) getKubeConfig() (*rest.Config, error) {
	_config, err := clientcmd.BuildConfigFromFlags("", k.clusterConfigFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get config for check deploy of postgres")
	}
	return _config, nil
}

// copyConfigFromMaster
// скопировать файл kube config c мастер-инстанса кластера и применить для использования
func (k *Kubernetes) copyConfigFromMaster() (err error) {
	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", k.addressMap.MasterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", k.sshKeyFileName, "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	copyFromMasterCmd.Dir = k.workingDirectory

	llog.Infoln(copyFromMasterCmd.String())
	llog.Debugf("Working directory is `%s`\n", copyFromMasterCmd.Dir)

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
	// делаем повтор на случай проблем с кластером
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

func (k *Kubernetes) WaitPod(clientSet *kubernetes.Clientset, podName, namespace string) (targetPod *v1.Pod, err error) {
	targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
		podName,
		metav1.GetOptions{
			TypeMeta:        metav1.TypeMeta{},
			ResourceVersion: "",
		})
	if err != nil {
		err = merry.Prepend(err, "get pod")
		return
	}

	waitTime := 10 * time.Minute
	const waitTimeQuantum = 10 * time.Second
	for targetPod.Status.Phase != v1.PodRunning && waitTime > 0 {
		targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
			podName,
			metav1.GetOptions{
				TypeMeta:        metav1.TypeMeta{},
				ResourceVersion: "",
			})
		if err != nil {
			llog.Warnf("WaitPod: failed to update information: %v", err)
			continue
		}

		waitTime -= waitTimeQuantum
		time.Sleep(waitTimeQuantum)

		llog.Debugf("WaitPod: pod status: %v\n",
			k.StroppyPod.Status.Phase)
	}

	if targetPod.Status.Phase != v1.PodRunning {
		err = merry.Errorf("pod still not running, 5 minutes left, current status: '%v",
			k.StroppyPod.Status.Phase)
		return
	}

	return
}

// nolint
func (k *Kubernetes) parseKubernetesFilePath(path string) (podName, containerName, internalPath string) {
	parts := strings.Split(path, "://")
	if len(parts) < 2 {
		return
	}

	kubePart := parts[0]
	podContainerSpec := strings.Split(kubePart, "/")
	podContainerSpecSize := len(podContainerSpec)
	if podContainerSpecSize < 1 {
		podName = kubePart
	} else if podContainerSpecSize == 1 {
		podName = kubePart
	} else {
		podName = podContainerSpec[0]
		containerName = podContainerSpec[1]
	}

	internalPath = parts[1]
	if filepath.Base(internalPath) == internalPath {
		return
	}

	return
}

// executeGetingMonImages - получить данные мониторинга.
// Осуществляется запуском скрипта get_png.sh, результат работы которого - архив с набором png-файлов
func (k Kubernetes) ExecuteGettingMonImages(startTime int64, finishTime int64, monImagesArchName string) error {
	llog.Infoln("Starting to get monitoring images...")

	llog.Debugln("start time of monitoring data range", time.Unix(startTime/1000, 0).UTC())
	llog.Debugln("finish time of monitoring data range", time.Unix(finishTime/1000, 0).UTC())

	workersIps := fmt.Sprintf("%v;%v;%v", k.addressMap.IngressInternalIP, k.addressMap.MetricsInternalIP, k.addressMap.PostgresInternalIP)
	getMonImagesCmd := fmt.Sprintf("cd grafana-on-premise && ./get_png.sh %v %v %v \"%v\"", startTime, finishTime, monImagesArchName, workersIps)

	llog.Debugln(getMonImagesCmd)
	err := k.executeCommand(getMonImagesCmd)
	if err != nil {
		return merry.Prepend(err, "failed to get monitoring images")
	}

	llog.Infoln("getting of monitoring images: success")
	return nil
}

func (k Kubernetes) getIQNStorage(workersCount int) (iqnMap map[string]string, err error) {

	stateFilePath := filepath.Join(k.workingDirectory, terraformStateFileName)
	var data []byte

	if data, err = ioutil.ReadFile(stateFilePath); err != nil {
		err = merry.Prepend(err, "failed to read file terraform.tfstate")
		return
	}

	iqnMap = make(map[string]string)
	masterInstance := "instances.0"
	iqnMap["master"] = gjson.Parse(string(data)).Get("resources.9").Get(masterInstance).Get("attributes").Get("iqn").Str
	// для Oracle мы задаем при деплое на одну ноду больше, фактически воркеров nodes-1
	for i := 1; i <= workersCount-1; i++ {
		workerInstance := fmt.Sprintf("instances.%v", i)
		worker := fmt.Sprintf("worker-%v", i)
		iqnMap[worker] = gjson.Parse(string(data)).Get("resources.9").Get(workerInstance).Get("attributes").Get("iqn").Str
	}

	return iqnMap, nil

}

func (k Kubernetes) IsExistEntity(address string, checkCommand string, checkString string) (checkResult bool, err error) {
	var CmdResult []byte
	if CmdResult, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, address, checkCommand, k.provider); err != nil {
		if err != nil {
			errorMessage := fmt.Sprintf("failed to execute command on worker %v", address)
			return false, merry.Prepend(err, errorMessage)
		}
	}

	if strings.Contains(string(CmdResult), checkString) {
		llog.Infoln("entity already exist or parted")
		return true, nil
	}

	llog.Infoln("entity has not been exist yet")
	return false, nil
}

func (k Kubernetes) AddNetworkStorages(nodes int, provider string) error {

	iqnMap, err := k.getIQNStorage(nodes)
	if err != nil {
		return merry.Prepend(err, "failed to get IQNs map")
	}

	var addressArray []string
	//временное решение до перехода на поддержку динамического кол-ва нод
	addressArray = append(addressArray, k.addressMap.MasterExternalIP, k.addressMap.MetricsExternalIP,
		k.addressMap.IngressExternalIP, k.addressMap.DatabaseExternalIP)
	/*
		В цикле выполняется следующий алгоритм:
		Если команда проверки вернула false, то выполняем команду создания/добавления сущности.
		Иначе - идем дальше. Это дожно обеспечивать идемпотентность подключения storages в рамках деплоя.
	*/
	for i := range addressArray {

		var targetLoginCmd string

		worker := fmt.Sprintf("worker-%v", i)
		// заполняем шаблон для воркера или мастера
		targetLoginCmd = fmt.Sprintf(targetLoginCmdTemplate, iqnMap[worker], iqnMap[worker], iqnMap[worker])

		if i == 0 {

			worker = "master"

			targetLoginCmd = fmt.Sprintf(targetLoginCmdTemplate, iqnMap["master"], iqnMap["master"], iqnMap["master"])

		}

		llog.Infof("Adding network storage to %v %v", worker, addressArray[i])

		llog.Infoln("checking additional storage mount ...")
		ok, err := k.IsExistEntity(addressArray[i], checkAdddedDiskCmd, "block special")
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage mount ")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, addressArray[i], targetLoginCmd, k.provider); err != nil {
				errorMessage := fmt.Sprintf("additional storage is not mounted to %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("mount additional storage: success")
		}

		llog.Infoln("checking the partition of additional storage...")
		ok, err = k.IsExistEntity(addressArray[i], checkPartedCmd, "primary")
		if err != nil {
			return merry.Prepend(err, "failed to check the partition of additional storage")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, addressArray[i], partedVolumeCmd, k.provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for additional storage partitioning %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Partition of  additional storage: success")
		}

		llog.Infoln("checking of additional storage file system exist...")
		ok, err = k.IsExistEntity(addressArray[i], checkExistFileSystemCmd, "ext4")
		if err != nil {
			return merry.Prepend(err, "failed to check additional storage file system exist.")
		}

		if !ok {
			if _, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, addressArray[i], createfileSystemCmd, k.provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for create additional storage file system %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Create additional storage filesystem: success")
		}

		llog.Infoln("checking of disk /dev/sdb1 mount ...")
		ok, err = k.IsExistEntity(addressArray[i], checkMountCmd, "/opt/local-path-provisioner")
		if err != nil {
			return merry.Prepend(err, "failed to check checking of disk /dev/sdb1 mount")
		}

		if !ok {

			if _, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, addressArray[i], addDirectoryCmdTemplate, k.provider); err != nil {
				errorMessage := fmt.Sprintf("failed to execute commands for add directory to %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infoln("Added directory /opt/local-path-provisioner/: success")

			if _, err = engineSsh.ExecuteCommandWorker(k.workingDirectory, addressArray[i], mountLocalPathTemplate, k.provider); err != nil {
				errorMessage := fmt.Sprintf("failed to mount disk to /opt/local-path-provisioner/ %v", worker)
				return merry.Prepend(err, errorMessage)
			}
			llog.Infof("Mounting of disk /dev/sdb1 to /opt/local-path-provisioner/ %v: success", worker)
		}
		llog.Infof("added network storage to %v", worker)

	}

	return nil
}
