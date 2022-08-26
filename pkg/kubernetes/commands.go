/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

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

//nolint:gocritic // because here two conflicting lint rules nonamedreturns and unnamedResult
func (k *Kubernetes) ExecuteRemoteCommand(
	_, containerName string,
	testCmd []string,
	logFileName string,
) (int64, int64, error) {
	var (
		beginTime int64
		endTime   int64
		config    *rest.Config
		clientSet *kubernetes.Clientset
		err       error
	)

	if config, err = k.Engine.GetKubeConfig(); err != nil {
		return beginTime, endTime, merry.Prepend(
			err, "failed to get config for execute remote test",
		)
	}

	if clientSet, err = k.Engine.GetClientSet(); err != nil {
		return beginTime, endTime, merry.Prepend(
			err, "failed to get clientset for execute remote test",
		)
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
		return beginTime, endTime, merry.Prepend(err, "failed to execute remote test")
	}

	logFilePath := filepath.Join(k.Engine.WorkingDirectory, logFileName)

	var logFile *os.File
	if logFile, err = os.Create(logFilePath); err != nil {
		return beginTime, endTime, merry.Prepend(err, "failed to create test log file")
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
		return beginTime, endTime, merry.Prepend(err, "failed to get stream of exec command")
	}

	endTime = (time.Now().UTC().UnixNano() / int64(time.Millisecond)) + 20000

	return beginTime, endTime, nil
}
