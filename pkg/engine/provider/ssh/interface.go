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

func CreateClient(wd, address, provider string, isRemote bool) (c Client, err error) {
	if isRemote {
		c, err = createClient(wd, address, provider)
	} else {
		c, err = createDummyClient(wd)
	}

	return
}
