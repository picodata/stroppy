/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// EditClusterURL - подменить адрес и порт внутри kubeconfig
func (e *Engine) EditClusterURL(port int) error {
	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	url := fmt.Sprintf("https://localhost:%v", port)
	llog.Infoln("changing of cluster url on", url)
	kubeConfig, err := clientcmd.LoadFromFile(e.clusterConfigFile)
	if err != nil {
		return merry.Prepend(err, "failed to load kube config")
	}
	// меняем значение адреса кластера внутри kubeconfig
	kubeConfig.Clusters["cluster.local"].Server = url

	err = clientcmd.WriteToFile(*kubeConfig, e.clusterConfigFile)
	if err != nil {
		return merry.Prepend(err, "failed to write kubeconfig")
	}

	return nil
}

// GetKubeConfig - получить конфигурацию k8s
func (e *Engine) GetKubeConfig() (*rest.Config, error) {
	_config, err := clientcmd.BuildConfigFromFlags("", e.clusterConfigFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get config for check deploy of postgres")
	}
	return _config, nil
}

// CopyFileFromMaster - скопировать файл c мастер-инстанса кластера
func (e *Engine) CopyFileFromMaster(filePath string) (err error) {
	if e.UseLocalSession {
		var homeDirPath string
		if homeDirPath, err = os.UserHomeDir(); err != nil {
			return merry.Prepend(err, "get home dir")
		}
		if filePath == ConfigPath {
			kubeConfigFilePath := filepath.Join(homeDirPath, filePath)
			e.clusterConfigFile = kubeConfigFilePath
		}
		return
	}

	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/%v", e.AddressMap["external"]["master"], filePath)
	copyFromMasterCmd := exec.Command("scp", "-i", e.sshKeyFileName, "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	copyFromMasterCmd.Dir = e.WorkingDirectory

	llog.Infoln(copyFromMasterCmd.String())
	llog.Debugf("Working directory is `%s`\n", copyFromMasterCmd.Dir)

	var output []byte
	if output, err = copyFromMasterCmd.CombinedOutput(); err != nil {
		return merry.Prependf(err, "failed to execute command copy from master, output: %s", string(output))
	}

	return
}

func (e *Engine) installSshKeyFileOnMaster() (err error) {
	if e.isSshKeyFileOnMaster {
		return
	}

	masterExternalIP := e.AddressMap["external"]["master"]
	mastersConnectionString := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.ssh", masterExternalIP)
	copyPrivateKeyCmd := exec.Command("scp",
		"-i", e.sshKeyFileName,
		"-o", "StrictHostKeyChecking=no",
		e.sshKeyFileName, mastersConnectionString)
	copyPrivateKeyCmd.Dir = e.WorkingDirectory

	llog.Infof(copyPrivateKeyCmd.String())

	keyFileCopyed := false
	// делаем повтор на случай проблем с кластером
	// \todo: https://gitlab.com/picodata/stroppy/-/issues/4
	for i := 0; i <= ConnectionRetryCount; i++ {
		copyMasterCmdResult, err := copyPrivateKeyCmd.CombinedOutput()
		if err != nil {
			llog.Errorf("failed to copy private key key onto master: %v %v \n", string(copyMasterCmdResult), err)
			copyPrivateKeyCmd = exec.Command("scp",
				"-i", e.sshKeyFileName,
				"-o", "StrictHostKeyChecking=no",
				e.sshKeyFileName, mastersConnectionString)
			time.Sleep(ExecTimeout * time.Second)
			continue
		}

		keyFileCopyed = true
		llog.Tracef("result of copy private key: %v \n", string(copyMasterCmdResult))
		break
	}
	if !keyFileCopyed {
		return merry.New("key file not copied to master")
	}

	e.isSshKeyFileOnMaster = true
	return
}

// nolint
func (e *Engine) parseKubernetesFilePath(path string) (podName, containerName, internalPath string) {
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

// ExecuteGetingMonImages собирает данные мониторинга.
// Осуществляется запуском скрипта get_png.sh, результат работы которого - архив с набором png-файлов
func (e Engine) CollectMonitoringData(startTime int64, finishTime int64, monitoringPort int, monImagesArchName string) error {
	llog.Infoln("Starting to get monitoring images...")

	llog.Debugln("start time of monitoring data range", time.Unix(startTime/1000, 0).UTC())
	llog.Debugln("finish time of monitoring data range", time.Unix(finishTime/1000, 0).UTC())

	var workersIps string

	for _, address := range e.AddressMap["internal"] {
		workersIps += fmt.Sprintf("%v;", address)
	}

	workingDirectory := filepath.Join(e.WorkingDirectory, "monitoring", "grafana-on-premise")
	getImagesCmd := exec.Command("./get_png.sh",
		fmt.Sprintf("%v", startTime),
		fmt.Sprintf("%v", finishTime),
		fmt.Sprintf("%v", monitoringPort),
		monImagesArchName, workersIps)
	getImagesCmd.Dir = workingDirectory

	if result, err := getImagesCmd.CombinedOutput(); err != nil {
		llog.Errorln(string(result))
		return merry.Prepend(err, "failed to get monitoring images")
	}

	llog.Infoln("getting of monitoring images: success")
	return nil
}
