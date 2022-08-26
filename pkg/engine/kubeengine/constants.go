/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import "time"

// Externally avail constants
const (
	ResourcePodName           = "pods"
	ResourceService           = "svc"
	ResourceDefaultNamespace  = "stroppy"
	SubresourcePortForwarding = "portforward"
	SubresourceExec           = "exec"
	PodWaitingWaitCreation    = true
	PodWaitingNotWaitCreation = false

	PodWaitingTimeTenMinutes = 10 * time.Minute

	SSHEntity      = "kubernetes"
	KubeConfigPath = ".kube/config"

	// задержка для случаев ожидания переповтора или соблюдения порядка запуска
	ExecTimeout = 5

	// кол-во попыток подключения при ошибке
	ConnectionRetryCount = 3

	// path to monitoring script.
	GetPngScriptPath = "./get_png.sh"

	SSHUserName = "ubuntu"
)
