package ssh

import "io"

type Session interface {
	CombinedOutput(string) ([]byte, error)
	StdoutPipe() (io.Reader, error)
	Close() error
}

type Client interface {
	GetNewSession() (Session, error)
	GetPrivateKeyInfo() (string, string)
}

func CreateClient(wd, address, provider string, isLocal bool) (c Client, err error) {
	if isLocal {
		c, err = createDummyClient(wd)
	} else {
		c, err = createClient(wd, address, provider)
	}

	return
}
