/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package ssh

import (
	"fmt"
	"io"
)

type Session interface {
	CombinedOutput(string) ([]byte, error)
	StdoutPipe() (io.Reader, error)
	Close() error
}

type Client interface {
	GetNewSession() (Session, error)
	GetPrivateKeyInfo() (string, string)
}

type ClientType string

const (
	DummyClient  ClientType = "dummy"
	LocalClient  ClientType = "local"
	RemoteClient ClientType = "remote"
)

func CreateClient(wd, address, provider string, clientType ClientType) (c Client, err error) {
	switch clientType {
	case RemoteClient:
		c, err = createSSHClient(wd, address, provider)

	case LocalClient:
		c, err = createLocalClient(wd)

	case DummyClient:
		c, err = createDummyClient()

	default:
		err = fmt.Errorf("unknown client type '%v'", clientType)
	}

	return
}
