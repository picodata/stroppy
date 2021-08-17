package kubernetes

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/pkg/tools"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8s_errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// editClusterURL - подменить адрес и порт внутри kubeconfig
func (k *Kubernetes) editClusterURL(port int) error {
	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	url := fmt.Sprintf("https://localhost:%v", port)
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

// CopyFileFromMaster - скопировать файл c мастер-инстанса кластера
func (k *Kubernetes) CopyFileFromMaster(filePath string) (err error) {
	if k.useLocalSession {
		var homeDirPath string
		if homeDirPath, err = os.UserHomeDir(); err != nil {
			return merry.Prepend(err, "get home dir")
		}
		if filePath == kubeConfigPath {
			kubeConfigFilePath := filepath.Join(homeDirPath, filePath)
			k.clusterConfigFile = kubeConfigFilePath
		}
		return
	}

	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/%v", k.AddressMap["external"]["master"], filePath)
	copyFromMasterCmd := exec.Command("scp", "-i", k.sshKeyFileName, "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	copyFromMasterCmd.Dir = k.workingDirectory

	llog.Infoln(copyFromMasterCmd.String())
	llog.Debugf("Working directory is `%s`\n", copyFromMasterCmd.Dir)

	var output []byte
	if output, err = copyFromMasterCmd.CombinedOutput(); err != nil {
		return merry.Prependf(err, "failed to execute command copy from master, output: %s", string(output))
	}

	return
}

func (k *Kubernetes) installSshKeyFileOnMaster() (err error) {
	if k.isSshKeyFileOnMaster {
		return
	}

	masterExternalIP := k.AddressMap["external"]["master"]
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

func (k *Kubernetes) WaitPod(podName, namespace string,
	creationWait bool, waitTime time.Duration) (targetPod *v1.Pod, err error) {

	const waitTimeQuantum = 10 * time.Second
	if waitTime < waitTimeQuantum {
		err = fmt.Errorf("input wait time %v (s) is less than quantum 10 seconds", waitTime.Seconds())
		return
	}

	var clientSet *kubernetes.Clientset
	if clientSet, err = k.GetClientSet(); err != nil {
		err = merry.Prepend(err, "get client set")
		return
	}

	targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
		podName,
		metav1.GetOptions{
			TypeMeta:        metav1.TypeMeta{},
			ResourceVersion: "",
		})
	if err != nil {
		if k8s_errors.IsNotFound(err) && creationWait {

			llog.Infof("WaitPod: go wait '%s/%s' pod creation...",
				namespace, podName)

			creationWaitTime := waitTime
			for k8s_errors.IsNotFound(err) && creationWaitTime > 0 {

				creationWaitTime -= waitTimeQuantum
				time.Sleep(waitTimeQuantum)

				targetPod, err = clientSet.CoreV1().Pods(namespace).Get(context.TODO(),
					podName,
					metav1.GetOptions{
						TypeMeta:        metav1.TypeMeta{},
						ResourceVersion: "",
					})
			}

			if err != nil {
				err = merry.Prependf(err, "'%s/%s' pod creation failed", namespace, podName)
				return
			}

			if targetPod == nil {
				err = fmt.Errorf("pod '%s/%s' still not created", namespace, podName)
				return
			}

		} else {
			err = merry.Prepend(err, "get pod")
			return
		}
	}

	if targetPod.Status.Phase == v1.PodRunning {
		llog.Debugf("WaitPod: pod '%s/%s' already running", namespace, podName)
		return
	}

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

		llog.Infof("WaitPod: '%s' pod status: %v", targetPod.Name, targetPod.Status.Phase)
	}

	if targetPod.Status.Phase != v1.PodRunning {
		err = merry.Errorf("pod still not running, %d minutes left, current status: '%v'",
			waitTime/time.Minute, targetPod.Status.Phase)
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

	var workersIps string

	for _, address := range k.AddressMap["internal"] {
		workersIps += fmt.Sprintf("%v;", address)
	}

	workingDirectory := fmt.Sprintf("%v/%v", k.workingDirectory, "grafana-on-premise")
	getImagesCmd := exec.Command("./get_png.sh", fmt.Sprintf("%v", startTime), fmt.Sprintf("%v", finishTime), monImagesArchName, workersIps)
	getImagesCmd.Dir = workingDirectory
	if result, err := getImagesCmd.CombinedOutput(); err != nil {
		llog.Errorln(string(result))
		return merry.Prepend(err, "failed to get monitoring images")
	}

	llog.Infoln("getting of monitoring images: success")
	return nil
}

// AddNodeLabels - добавить labels worker-нодам кластера для разделения stroppy и СУБД
func (k Kubernetes) AddNodeLabels(_ string) (err error) {
	llog.Infoln("Starting of add labels to cluster nodes")

	clientSet, err := k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get client set for deploy stroppy")
	}

	// используем получения списка нод ради точного кол-ва нод кластера.
	// deploySettings.nodes не используем из-за разного кол-ва nodes для одинакового кол-ва воркеров в yc и oc

	nodeListRaw, err := tools.RetryWithResult("get nodes list",
		func() (result interface{}, err error) {
			nodesList, err := clientSet.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
			return nodesList, err
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime)

	if err != nil {
		return merry.Prepend(err, "failed to get nodes list")
	}

	nodesList, ok := nodeListRaw.(*v1.NodeList)
	if !ok {
		return merry.Prepend(err, "failed to check nodes list type")
	}

	for i := 1; i < len(nodesList.Items); i++ {
		name := fmt.Sprintf("worker-%v", i)

		node, err := clientSet.CoreV1().Nodes().Get(context.TODO(), name, metav1.GetOptions{})
		if err != nil {
			return merry.Prepend(err, "failed to get node")
		}

		currentLabels := node.GetLabels()

		if _, ok := currentLabels["worker-type"]; ok {
			llog.Infoln("this node already been marked")
			continue
		}

		currentLabels["worker-type"] = "dbms-worker"
		node.SetLabels(currentLabels)

		// последний воркер оставляем для stroppy
		if i == len(nodesList.Items)-1 {
			currentLabels["worker-type"] = "stroppy-worker"
			node.SetLabels(currentLabels)
		}

		// применяем изменения на ноду
		err = tools.Retry("adding labels to nodes",
			func() (err error) {
				_, err = clientSet.CoreV1().Nodes().Update(context.TODO(), node, metav1.UpdateOptions{})
				return
			},
			tools.RetryStandardRetryCount,
			tools.RetryStandardWaitingTime)
		if err != nil {
			return merry.Prepend(err, "failed to update node")
		}
	}

	llog.Infoln("Add labels to cluster nodes: success")

	return
}

// getHostsFileAttributes - получить атрибуты для заполнения файла hosts.ini для использования при деплое k8s кластера
func (k Kubernetes) getHostsFileAttributes() (deployK8sSecondStep string) {
	var workersAddressString string
	var masterAddressString string
	var workersString string

	internalAddressMap := k.AddressMap["internal"]

	var keys []string
	for k := range k.AddressMap["internal"] {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for i, k := range keys {
		if i == 0 {
			masterAddressString = fmt.Sprintf("master ansible_host=%v ip=%v etcd_member_name=etcd1 \n", internalAddressMap["master"], internalAddressMap["master"])
		} else {
			workersAddressString += fmt.Sprintf("worker-%v ansible_host=%v ip=%v etcd_member_name=etcd%v \n", i, internalAddressMap[k], internalAddressMap[k], i+1)
			workersString += fmt.Sprintf("worker-%v \n", i)
		}
	}

	instancesString := masterAddressString + workersAddressString

	deployK8sSecondStep = fmt.Sprintf(deployK8sSecondStepTemplate, instancesString, workersString, workersString)

	return deployK8sSecondStep
}
