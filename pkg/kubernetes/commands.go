package kubernetes

import (
	"os"
	"path/filepath"
	"time"

	engine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"

	"github.com/ansel1/merry"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func (k *Kubernetes) ExecuteRemoteCommand(podName, containerName string, testCmd []string, logFileName string) (beginTime int64, endTime int64, err error) {
	var config *rest.Config
	if config, err = k.Engine.GetKubeConfig(); err != nil {
		err = merry.Prepend(err, "failed to get config for execute remote test")
		return
	}

	var clientSet *kubernetes.Clientset
	if clientSet, err = k.Engine.GetClientSet(); err != nil {
		err = merry.Prepend(err, "failed to get clientset for execute remote test")
		return
	}

	// формируем запрос для API k8s
	executeRequest := clientSet.CoreV1().RESTClient().Post().
		Resource(engine.ResourcePodName).
		Name(stroppy.PodName).
		Namespace(engine.ResourceDefaultNamespace).
		SubResource(engine.SubresourceExec).Timeout(60)

	option := &v1.PodExecOptions{
		TypeMeta:  metav1.TypeMeta{},
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
		Container: containerName,
		Command:   testCmd,
	}

	executeRequest.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	// подключаемся к API-серверу
	var _exec remotecommand.Executor
	if _exec, err = remotecommand.NewSPDYExecutor(config, "POST", executeRequest.URL()); err != nil {
		err = merry.Prepend(err, "failed to execute remote test")
		return
	}

	logFilePath := filepath.Join(k.Engine.WorkingDirectory, logFileName)

	var logFile *os.File
	if logFile, err = os.Create(logFilePath); err != nil {
		err = merry.Prepend(err, "failed to create test log file")
		return
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
	if err = _exec.Stream(streamOptions); err != nil {
		err = merry.Prepend(err, "failed to get stream of exec command")
		return
	}

	endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) + 20000
	return
}
