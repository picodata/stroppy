/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	sshe "gitlab.com/picodata/stroppy/pkg/engine/ssh"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"gitlab.com/picodata/stroppy/pkg/tools"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"golang.org/x/crypto/ssh"
)

// OpenPortForward открывает port-forward туннель для вызывающей функции(caller)
func (e *Engine) OpenPortForward(caller string, ports []string, reqURL *url.URL,
	stopPortForward chan struct{},
) (err error) {
	llog.Debugf("Opening port-forward for %s, with url `%s`", caller, reqURL.String())

	var kubeConfig *rest.Config
	if kubeConfig, err = e.GetKubeConfig(); err != nil {
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

func (e *Engine) GetSessionObject() (io.Reader, sshe.Session, error) {
	var (
		stdout  io.Reader
		session sshe.Session
		err     error
	)

	if session, err = e.sc.GetNewSession(); err != nil {
		return stdout, session, merry.Prepend(err, "failed to open ssh connection")
	}

	if stdout, err = session.StdoutPipe(); err != nil {
		err = merry.Prepend(err, "failed creating command stdoutpipe for logging deploy k8s")

		if err = session.Close(); err != nil {
			llog.Warnf("GetSessionObject: k8s ssh session can not closed: %v", err)
		}
	}

	//nolint:wrapcheck // error already wrapped with merry
	return stdout, session, err
}

// OpenSecureShellTunnel открывает ssh-соединение
//nolint:funlen // logic of this function is inseparable
func (e *Engine) OpenSecureShellTunnel(
	caller, targetHostname string,
	targetPort int,
) *sshe.Result {
	tunnelPort := targetPort
	retryStandardRetryCount := tools.RetryStandardRetryCount
	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Debugf(
		"Checking the status of '%s' port on the '%s:%d'\n",
		caller,
		targetHostname,
		tunnelPort,
	)

	var tunellOpeningResult *sshe.Result

	for !engine.IsRemotePortOpen(targetHostname, tunnelPort) {
		// проверяем резервный порт в случае недоступности основного
		tunnelPort++

		llog.Warnf("Main port for '%s' is %d and he is closed\n", caller, targetPort)
		llog.Debugf(
			"Checking the status of '%s' port on the '%s:%d'\n",
			caller,
			targetHostname,
			tunnelPort,
		)
		// условие добавляем здесь, чтобы не портить им последующий код
		if tunnelPort >= tunnelPort+retryStandardRetryCount {
			tunellOpeningResult = &sshe.Result{
				Port:   0,
				Tunnel: nil,
				Err: fmt.Errorf("check ports %v-%v are not available",
					targetPort, targetPort+retryStandardRetryCount),
			}

			return tunellOpeningResult
		}
	}

	llog.Infof("Remote port %d on %s is openeded", tunnelPort, targetHostname)

	// если туннель для k8s и недоступен основной порт, то меняем его на резервный
	if tunnelPort != targetPort && caller == SSHEntity {
		llog.Debugln(
			fmt.Sprintf("Because we starting ssh tunell for '%s' and port '%d' ",
				caller,
				targetPort,
			),
			fmt.Sprintf("was changed to '%d' start editing cluster url",
				tunnelPort,
			),
		)
		if err := e.EditClusterURL(tunnelPort); err != nil {
			llog.Infof("failed to replace port: %v", err)

			tunellOpeningResult = &sshe.Result{Port: 0, Tunnel: nil, Err: err}

			return tunellOpeningResult
		}
	}

	var (
		authMethod ssh.AuthMethod
		err        error
	)

	if authMethod, err = sshtunnel.PrivateKeyFile(e.sshKeyFilePath); err != nil {
		llog.Infof("failed to use private key file: %v", err)

		tunellOpeningResult = &sshe.Result{Port: 0, Tunnel: nil, Err: err}

		return tunellOpeningResult
	}

	// Setup the tunnel, but do not yet start it yet.
	tunnel := sshtunnel.NewSSHTunnel(
		tunnelPort,
		targetHostname,
		SSHUserName,
		authMethod,
	)

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	if err = tunnel.Start(); err != nil {
		tunellOpeningResult = &sshe.Result{
			Port:   0,
			Tunnel: nil,
			Err:    merry.Prepend(err, "failed to start tunnel"),
		}

		return tunellOpeningResult
	}

	return &sshe.Result{Port: tunnelPort, Tunnel: tunnel, Err: nil}
}
