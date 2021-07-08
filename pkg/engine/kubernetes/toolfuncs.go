package kubernetes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
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

func (k *Kubernetes) ExecuteRemoteTest(testCmd []string, logFileName string) (beginTime int64, endTime int64, err error) {
	config, err := k.getKubeConfig()
	if err != nil {
		return 0, 0, merry.Prepend(err, "failed to get config for execute remote test")
	}

	clientSet, err := k.GetClientSet()
	if err != nil {
		return 0, 0, merry.Prepend(err, "failed to get clientset for execute remote test")
	}

	// формируем запрос для API k8s
	executeRequest := clientSet.CoreV1().RESTClient().Post().
		Resource(ResourcePodName).
		Name(stroppyPodName).
		Namespace(ResourceDefaultNamespace).
		SubResource(SubresourceExec).Timeout(60)

	option := &v1.PodExecOptions{
		TypeMeta:  metav1.TypeMeta{},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
		Container: "",
		Command:   testCmd,
	}

	executeRequest.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	// подключаемся к API-серверу
	var _exec remotecommand.Executor
	if _exec, err = remotecommand.NewSPDYExecutor(config, "POST", executeRequest.URL()); err != nil {
		return 0, 0, merry.Prepend(err, "failed to execute remote test")
	}

	logFilePath := filepath.Join(k.workingDirectory, logFileName)

	logFile, err := os.Create(logFilePath)
	if err != nil {
		return 0, 0, merry.Prepend(err, "failed to create test log file")
	}

	defer logFile.Close()

	streamOptions := remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            logFile,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: nil,
	}

	// для графаны преобразуем в миллисекунды. Примитивно, но точность не принципиальна.
	// сдвиг +/- 20 сек для того, чтобы тест на графиках был явно заметен относительно "фона"
	beginTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) - 20000

	// выполняем запрос и выводим стандартный вывод в указанное в опциях место
	err = _exec.Stream(streamOptions)
	if err != nil {
		return 0, 0, merry.Prepend(err, "failed to get stream of exec command")
	}

	endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) + 20000

	return beginTime, endTime, nil
}

func (k Kubernetes) waitStroppyPod(_ *kubernetes.Clientset) (err error) {
	waitingTime := 5 * time.Minute

	const waitTimeQuantum = 10 * time.Second
	for k.stroppyPod.Status.Phase != v1.PodRunning && waitingTime > 0 {

		waitingTime -= waitTimeQuantum
		time.Sleep(waitTimeQuantum)

		llog.Debugf("waitStroppyPod: pod status: %v\n",
			k.stroppyPod.Status.Phase)

	}

	if k.stroppyPod.Status.Phase != v1.PodRunning {
		err = merry.Errorf("stroppy pod still not running, 5 minutes left, current status: '%v",
			k.stroppyPod.Status.Phase)
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
