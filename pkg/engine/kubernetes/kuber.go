package kubernetes

import (
	"fmt"
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
	"net/http"
	"net/url"
	"os"
	"os/exec"
)

// openKubePortForward - открыть port-forward туннель для вызывающей функции(caller)
func openKubePortForward(caller string, ports []string, reqURL *url.URL,
	stopPortForward chan struct{}, readyPortForward chan struct{}, errorPortForward chan error) {
	llog.Printf("Opening of port-forward of %v...\n", caller)

	config, err := getKubeConfig()
	if err != nil {
		llog.Errorf("failed to get kubeconfig for open port-forward of %v: %v", caller, err)
		errorPortForward <- err
	}

	httpTransaction, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		llog.Errorf("failed to create http transction for port-forward of %v: %v\n", caller, err)
		errorPortForward <- err
	}

	portForwardLog, err := os.OpenFile("portForwardPostgres.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		llog.Errorf("failed to create or open log file for port-forward of %v: %v", caller, err)
		errorPortForward <- err
	}
	defer portForwardLog.Close()

	//nolint:exhaustivestruct
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: httpTransaction}, http.MethodPost, reqURL)
	portForward, err := portforward.New(dialer, ports,
		stopPortForward, readyPortForward, portForwardLog, portForwardLog)
	if err != nil {
		llog.Errorf("failed to get port-forwarder of %v: %v\n", caller, err)
		errorPortForward <- err
	}

	err = portForward.ForwardPorts()
	defer close(stopPortForward)
	if err != nil {
		llog.Errorf("failed to open port-forward of %v: %v\n", caller, err)
		errorPortForward <- err
	}
}

// openMonitoringPortForward - запустить kubectl port-forward для доступа к мониторингу кластера с локального хоста
func openMonitoringPortForward(portForwardChan chan tunnelToCluster) {
	// проверяем доступность портов 8080 и 8081 на локальной машине
	llog.Infoln("Checking the status of port 8080 of the localhost for monitoring...")
	monitoringPort := clusterMonitoringPort
	if !isLocalPortOpen(monitoringPort) {
		llog.Infoln("Checking the status of port 8081 of the localhost for monitoring...")
		// проверяем доступность резервного порта
		monitoringPort = reserveClusterMonitoringPort
		if !isLocalPortOpen(monitoringPort) {
			portForwardChan <- tunnelToCluster{nil, merry.Prepend(errPortCheck, ": ports 8080 and 8081 are not available"), nil}
		}
	}

	// формируем строку с указанием портов для port-forward
	portForwardSpec := fmt.Sprintf("%v:3000", monitoringPort)
	// уровень --v=4 соответствует debug
	portForwardCmd := exec.Command("kubectl", "port-forward", "--kubeconfig=config", "--log-file=portforward.log",
		"--v=4", "deployment/grafana-stack", portForwardSpec, "-n", "monitoring")
	llog.Infof(portForwardCmd.String())
	portForwardCmd.Dir = terraformWorkDir

	// используем метод старт, т.к. нужно оставить команду запущенной в фоне
	if err := portForwardCmd.Start(); err != nil {
		llog.Infof("failed to execute command  port-forward kubectl:%v ", err)
		portForwardChan <- tunnelToCluster{nil, err, nil}
	}
	portForwardChan <- tunnelToCluster{portForwardCmd, nil, &monitoringPort}
}
