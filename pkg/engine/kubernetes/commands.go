package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/ansel1/merry"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func (k *Kubernetes) executeCommand(text string) (err error) {
	var commandSessionObject engineSsh.Session
	if commandSessionObject, err = k.sc.GetNewSession(); err != nil {
		return merry.Prepend(err, "failed to open ssh connection")
	}

	if result, err := commandSessionObject.CombinedOutput(text); err != nil {
		return merry.Prependf(err, "command exec failed with output `%s`", string(result))
	}

	return
}

func (k *Kubernetes) Execute(text string) (err error) {
	err = k.executeCommand(text)
	return
}

func (k *Kubernetes) ExecuteF(text string, args ...interface{}) (err error) {
	err = k.executeCommand(fmt.Sprintf(text, args...))
	return
}

func (k *Kubernetes) ExecuteRemoteTest(testCmd []string, logFileName string) error {
	config, err := k.getKubeConfig()
	if err != nil {
		return merry.Prepend(err, "failed to get config for execute remote test")
	}

	clientSet, err := k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientset for execute remote test")
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
		return merry.Prepend(err, "failed to execute remote test")
	}

	logFilePath := filepath.Join(k.workingDirectory, logFileName)

	logFile, err := os.Create(logFilePath)
	if err != nil {
		return merry.Prepend(err, "failed to create test log file")
	}

	defer logFile.Close()

	streamOptions := remotecommand.StreamOptions{
		Stdin:             os.Stdin,
		Stdout:            logFile,
		Stderr:            os.Stderr,
		Tty:               true,
		TerminalSizeQueue: nil,
	}

	// выполняем запрос и выводим стандартный вывод в указанное в опциях место
	err = _exec.Stream(streamOptions)
	if err != nil {
		return merry.Prepend(err, "failed to get stream of exec command")
	}

	return nil
}
