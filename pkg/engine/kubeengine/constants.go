package kubeengine

import "time"

// Externally avail constants
const (
	ResourcePodName           = "pods"
	ResourceService           = "svc"
	ResourceDefaultNamespace  = "default"
	SubresourcePortForwarding = "portforward"
	SubresourceExec           = "exec"
	PodWaitingWaitCreation    = true
	PodWaitingNotWaitCreation = false

	PodWaitingTime10Minutes = 10 * time.Minute

	SshEntity  = "kubernetes"
	ConfigPath = ".kube/config"

	// задержка для случаев ожидания переповтора или соблюдения порядка запуска
	ExecTimeout = 5

	// кол-во попыток подключения при ошибке
	ConnectionRetryCount = 3
)
