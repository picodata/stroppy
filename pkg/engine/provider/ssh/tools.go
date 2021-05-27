package ssh

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"golang.org/x/crypto/ssh"
)

type Result struct {
	Port   int
	Tunnel *sshtunnel.SSHTunnel
	Err    error
}

func CreateClient(wd, address string) (client *Client, err error) {
	client = &Client{
		workingDirectory: wd,
		privateKeyFile:   filepath.Join(wd, "id_rsa"),
	}

	client.internalClient, err = client.getClientInstance(address)
	return
}

type Client struct {
	workingDirectory string
	privateKeyFile   string

	internalClient *ssh.Client
}

func (sc *Client) GetNewSession() (session *ssh.Session, err error) {
	session, err = sc.internalClient.NewSession()
	return
}

func (sc *Client) getClientInstance(address string) (client *ssh.Client, err error) {
	var keyBytes []byte
	if keyBytes, err = ioutil.ReadFile(sc.privateKeyFile); err != nil {
		err = merry.Prepend(err, "failed to get id_rsa for ssh client")
		return
	}

	var signer ssh.Signer
	if signer, err = ssh.ParsePrivateKey(keyBytes); err != nil {
		err = merry.Prepend(err, "failed to parse id_rsa for ssh client")
		return
	}

	//nolint:exhaustivestruct
	config := &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		//nolint:gosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", address, 22)
	if client, err = ssh.Dial("tcp", addr, config); err != nil {
		err = merry.Prepend(err, "failed to start ssh connection for ssh client")
	}
	return
}
