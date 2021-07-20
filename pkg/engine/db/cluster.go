package db

import (
	"os/exec"

	v1 "k8s.io/api/core/v1"
)

type Status string

const ExecTimeout = 20

type ClusterSpec struct {
	MainPod *v1.Pod
	Pods    []*v1.Pod
}

type Cluster interface {
	Deploy() error
	GetSpecification() ClusterSpec
}

// ClusterTunnel
/* структура хранит результат открытия port-forward туннеля к кластеру:
 * Command - структура, которая хранит атрибуты команды, которая запустила туннель
 * Error - возможная ошибка при открытии туннеля
 * LocalPort - порт локальной машины для туннеля */
type ClusterTunnel struct {
	Command   *exec.Cmd
	Error     error
	LocalPort *int
}
