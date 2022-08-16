/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package sshtunnel

import (
	"io"
	"net"

	"github.com/ansel1/merry"
	"golang.org/x/crypto/ssh"

	llog "github.com/sirupsen/logrus"
)

const sshDefaultPortNumber = 22

type logger interface {
	Printf(string, ...interface{})
}

type SSHTunnel struct {
	Local      *Endpoint
	Server     *Endpoint
	Remote     *Endpoint
	Config     *ssh.ClientConfig
	Log        logger
	Conns      []net.Conn
	serverConn *ssh.Client
	isOpen     bool
	close      chan interface{}
}

func (tunnel *SSHTunnel) logf(fmt string, args ...interface{}) {
	if tunnel.Log != nil {
		tunnel.Log.Printf(fmt, args...)
	}
}

func newConnectionWaiter(listener net.Listener, c chan net.Conn) {
	conn, err := listener.Accept()
	if err != nil {
		return
	}
	c <- conn
}

func (tunnel *SSHTunnel) Start() (err error) {
	var listener net.Listener
	if listener, err = net.Listen("tcp", tunnel.Local.String()); err != nil {
		return merry.Prepend(err, "net listen failed")
	}

	tunnel.isOpen = true
	tunnel.Local.Port = listener.Addr().(*net.TCPAddr).Port

	llog.Debugf("Server connection string '%s'", tunnel.Server.String())

	var serverConn *ssh.Client
	if serverConn, err = ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config); err != nil {
		tunnel.logf("Server dial error: %s", err)
		return merry.Prepend(err, "server dial error")
	}

	tunnel.logf("Established ssh connection to remote")
	tunnel.serverConn = serverConn

	go tunnel.tunnelProcess(listener)
	return
}

func (tunnel *SSHTunnel) tunnelProcess(listener net.Listener) {
	for {
		if !tunnel.isOpen {
			break
		}

		c := make(chan net.Conn)
		go newConnectionWaiter(listener, c)
		tunnel.logf("listening for new connections...")

		select {
		case <-tunnel.close:
			tunnel.logf("close signal received, closing...")
			tunnel.isOpen = false
		case conn := <-c:
			tunnel.Conns = append(tunnel.Conns, conn)
			tunnel.logf("accepted connection")
			go tunnel.forward(conn)
		}
	}

	var err error
	total := len(tunnel.Conns)
	for i, conn := range tunnel.Conns {
		tunnel.logf("closing the netConn (%d of %d)", i+1, total)
		if err = conn.Close(); err != nil {
			tunnel.logf(err.Error())
		}
	}
	if err = tunnel.serverConn.Close(); err != nil {
		tunnel.logf("failed to ssh connection to remote: %s", err)
	}

	if err = listener.Close(); err != nil {
		tunnel.logf("failed to close listener: %s", err)
	}
	tunnel.logf("tunnel closed")
}

// TODO: add net.Conn pool
func (tunnel *SSHTunnel) forward(localConn net.Conn) {
	remoteConn, err := tunnel.serverConn.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		tunnel.logf("remote dial error: %s", err)
		return
	}
	tunnel.Conns = append(tunnel.Conns, remoteConn)
	tunnel.logf("connected to %s\n", tunnel.Remote.String())
	copyConn := func(writer, reader net.Conn) {
		_, err := io.Copy(writer, reader)
		if err != nil {
			tunnel.logf("io.Copy error: %s", err)
		}
	}
	go copyConn(localConn, remoteConn)
	go copyConn(remoteConn, localConn)
}

func (tunnel *SSHTunnel) Close() {
	tunnel.close <- struct{}{}
}

// NewSSHTunnel creates a new single-use tunnel. Supplying "0" for localport will use a random port.
func NewSSHTunnel(
	targetPort int,
	targetHostname,
	targetUser string,
	sshTunnelAuth ssh.AuthMethod,
) *SSHTunnel {
	//nolint:exhaustivestruct
	sshTunnel := &SSHTunnel{
		Config: &ssh.ClientConfig{
			User: targetUser,
			Auth: []ssh.AuthMethod{sshTunnelAuth},
			HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
				// Always accept key.
				return nil
			},
		},
		Local:  NewLocalEndpoint(targetPort, ""),
		Server: NewRemoteEndpoint(targetHostname, sshDefaultPortNumber, targetUser),
		Remote: NewLocalEndpoint(targetPort, ""),
		close:  make(chan interface{}),
	}

	return sshTunnel
}
