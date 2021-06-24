package db

import (
	"errors"
	"os/exec"
)

type Status string

const ExecTimeout = 20

var ErrorPodsNotFound = errors.New("one of pods is not found")

type ClusterStatus struct {
	Status Status
	Err    error
}

type Cluster interface {
	Deploy() error
	OpenPortForwarding() error
	GetStatus() error
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
