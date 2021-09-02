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
		c, err = createSshClient(wd, address, provider)

	case LocalClient:
		c, err = createLocalClient(wd)

	case DummyClient:
		c, err = createDummyClient()

	default:
		err = fmt.Errorf("unknown client type '%v'", clientType)
	}
	return
}
