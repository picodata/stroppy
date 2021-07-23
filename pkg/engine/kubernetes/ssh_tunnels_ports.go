package kubernetes

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// OpenPortForward открывает port-forward туннель для вызывающей функции(caller)
func (k *Kubernetes) OpenPortForward(caller string, ports []string, reqURL *url.URL,
	stopPortForward chan struct{}) (err error) {

	llog.Debugf("opening port-forward for %s, with url `%s`", caller, reqURL.String())

	var kubeConfig *rest.Config
	if kubeConfig, err = k.getKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kube config")
	}

	var httpTransaction http.RoundTripper
	var updater spdy.Upgrader
	if httpTransaction, updater, err = spdy.RoundTripperFor(kubeConfig); err != nil {
		return merry.Prepend(err, "failed to create spdy transaction for port-forward")
	}

	var portForwardLog *os.File
	portForwardLog, err = os.OpenFile("portForwardPostgres.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY,
		0o644)
	if err != nil {
		return merry.Prepend(err, "failed to create or open log file for port-forward")
	}
	// defer portForwardLog.Close() не делаем, поскольку при выходе из приложения runtime сам все закроет

	dialer := spdy.NewDialer(updater,
		&http.Client{Transport: httpTransaction},
		http.MethodPost, reqURL)

	readyPortForward := make(chan struct{})

	var portForward *portforward.PortForwarder
	portForward, err = portforward.New(dialer, ports,
		stopPortForward, readyPortForward, portForwardLog, portForwardLog)
	if err != nil {
		return merry.Prepend(err, "failed to get port-forwarder")
	}

	portForwardingError := make(chan error)
	go func() {
		if err = portForward.ForwardPorts(); err != nil {
			portForwardingError <- merry.Prepend(err, "failed to open port-forward")
		}
	}()

	select {
	case <-readyPortForward:
		return

	case err = <-portForwardingError:
		return
	}
}

func (k *Kubernetes) getSessionObject() (stdout io.Reader, session engineSsh.Session, err error) {
	if session, err = k.sc.GetNewSession(); err != nil {
		err = merry.Prepend(err, "failed to open ssh connection")
		return
	}

	if stdout, err = session.StdoutPipe(); err != nil {
		err = merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")

		if err = session.Close(); err != nil {
			llog.Warnf("getSessionObject: k8s ssh session can not closed: %v", err)
		}
	}

	return
}

// OpenSecureShellTunnel

// открыть ssh-соединение и передать указатель на него вызывающему коду для управления
func (k *Kubernetes) OpenSecureShellTunnel(caller string, mainPort int, reservePort int) (result *engineSsh.Result) {
	mastersConnectionString := fmt.Sprintf("ubuntu@%v", k.addressMap["external"]["master"])

	tunnelPort := mainPort
	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Infof("Checking the status of %s port on the localhost for %v...\n", caller, tunnelPort)
	if !engine.IsLocalPortOpen(tunnelPort) {
		// проверяем резервный порт в случае недоступности основного
		tunnelPort = reservePort
		llog.Infof("Checking the status of port %v of the localhost for %v...\n", caller, tunnelPort)
		if !engine.IsLocalPortOpen(tunnelPort) {
			result = &engineSsh.Result{
				Port:   0,
				Tunnel: nil,
				Err:    merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available"),
			}
			return
		}

		// если туннель для k8s и недоступен основной порт, то меняем его на резервный
		if tunnelPort == 6444 {
			clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
			if err := k.editClusterURL(clusterURL); err != nil {
				llog.Infof("failed to replace port: %v", err)
				result = &engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
				return
			}
		}

	}

	authMethod, err := sshtunnel.PrivateKeyFile(k.sshKeyFilePath)
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		result = &engineSsh.Result{Port: 0, Tunnel: nil, Err: err}
		return
	}

	// Setup the tunnel, but do not yet start it yet.
	var tunnel *sshtunnel.SSHTunnel
	tunnel, err = sshtunnel.NewSSHTunnel(
		mastersConnectionString,
		fmt.Sprintf("localhost:%v", mainPort),
		tunnelPort,
		authMethod,
	)
	if err != nil {
		result = &engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to create tunnel"),
		}
		return
	}

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	if err = tunnel.Start(); err != nil {
		result = &engineSsh.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to start tunnel"),
		}
		return
	}

	return &engineSsh.Result{Port: tunnelPort, Tunnel: tunnel, Err: nil}
}
