/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"strings"

	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"gitlab.com/picodata/stroppy/pkg/tools"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func (k *Kubernetes) Deploy() (err error) {
	if err = k.loadFilesToMaster(); err != nil {
		return merry.Prepend(err, "failed to сopy files to cluster")
	}

	if err = k.deploy(); err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	err = tools.Retry("copy file from master on deploy",
		func() (err error) {
			err = k.Engine.CopyFileFromMaster(kubeengine.ConfigPath)

			return
		},
		tools.RetryStandardRetryCount,
		tools.RetryStandardWaitingTime)
	if err != nil {
		return merry.Prepend(err, "failed to copy kube config from master")
	}

	if err = k.Engine.EditClusterURL(clusterK8sPort); err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	k.KubernetesPort = k.Engine.OpenSecureShellTunnel(kubeengine.SSHEntity, clusterK8sPort)
	if k.KubernetesPort.Err != nil {
		err = merry.Prepend(k.KubernetesPort.Err, "failed to create ssh tunnel")

		return
	}

	llog.Infoln("status of creating ssh tunnel for the access to k8s: success")

	if err = k.Engine.AddNodeLabels(kubeengine.ResourceDefaultNamespace); err != nil {
		return merry.Prepend(err, "failed to add labels to cluster nodes")
	}

	k.StroppyPod = stroppy.CreateStroppyPod(k.Engine)
	if err = k.StroppyPod.Deploy(); err != nil {
		err = merry.Prepend(err, "failed to deploy stroppy pod")

		return
	}

	llog.Infoln("status of stroppy pod deploy: success")

	return
}

func (k *Kubernetes) OpenPortForwarding() (err error) {
	k.MonitoringPort = k.Engine.OpenSecureShellTunnel(monitoringSSHEntity, clusterMonitoringPort)
	if k.MonitoringPort.Err != nil {
		return merry.Prepend(k.MonitoringPort.Err, "cluster monitoring")
	}

	llog.Infoln("status of creating ssh tunnel for the access to monitoring: success")

	return
}

func (k *Kubernetes) Shutdown() {
	k.MonitoringPort.Tunnel.Close()
}

// deploy - развернуть k8s внутри кластера в cloud.
//     Последовательно формируем файл deploy_kubernetes.sh,
//	   даем ему права на выполнение и выполняем.
//	   1-й шаг - добавляем первую часть команд (deployk8sFirstStepCmd)
//	   2-й шаг - подставляем ip адреса в hosts.ini и добавляем команду с его записью в файл
//	   3-й шаг - добавляем вторую часть команд (deployk8sThirdStepCmd)
//	   4-й шаг - выдаем файлу права на выполнение и выполняем
func (k *Kubernetes) deploy() (err error) {
	var isDeployed bool

	if isDeployed, err = k.checkMasterDeploymentStatus(); err != nil {
		return merry.Prepend(err, "failed to Check deploy k8s in master node")
	}

	if isDeployed {
		llog.Infoln("k8s already success deployed")

		return
	}

	providerSpecificFirstStep, providerSpecificThirdStep := k.provider.GetDeploymentCommands()

	if err = k.Engine.ExecuteCommand(providerSpecificFirstStep); err != nil {
		return merry.Prepend(err, "first step failed")
	}

	llog.Printf("First step deploy k8s: success")

	secondStepCommandText := k.craftClusterDeploymentScript()

	if err = k.Engine.DebugCommand(secondStepCommandText, false); err != nil {
		return merry.Prepend(err, "second step failed")
	}

	llog.Printf("Second cluster deployment step: success")

	if err = k.Engine.DebugCommand(providerSpecificThirdStep, false); err != nil {
		return merry.Prepend(err, "third step")
	}

	llog.Printf("Third cluster deployment step: success")

	return
}

func (k *Kubernetes) Stop() {
	defer k.KubernetesPort.Tunnel.Close()

	llog.Infoln("status of ssh tunnel close: success")
}

// checkMasterDeploymentStatus проверяет, что все поды k8s в running, что подтверждает успешность разворачивания k8s.
func (k *Kubernetes) checkMasterDeploymentStatus() (bool, error) {
	masterExternalIP := k.Engine.AddressMap["external"]["master"]

	commandClientType := engineSsh.RemoteClient
	if k.Engine.UseLocalSession {
		commandClientType = engineSsh.LocalClient
	}

	sshClient, err := engineSsh.CreateClient(k.Engine.WorkingDirectory,
		masterExternalIP,
		k.provider.Name(),
		commandClientType)
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
		e, ok := err.(*ssh.ExitError)
		if !ok {
			return false, merry.Prepend(err, "failed сheck deploy k8s")
		}

		// если вернулся not found(код 127), это норм, если что-то другое - лучше проверить
		const sshNotFoundCode = 127
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
