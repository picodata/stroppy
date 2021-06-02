package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
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

func GetPrivateKeyFile(provider string, workingDirectory string) (string, error) {
	var privateKeyFile string
	// проверяем наличие приватного ключа
	switch provider {
	case "yandex":
		privateKeyFile = "private_key"
	case "oracle":
		privateKeyFile = "private_key.pem"
	}

	privateKeyFilePath := filepath.Join(workingDirectory, privateKeyFile)
	_, err := os.Stat(privateKeyFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return privateKeyFile, merry.Prepend(err, "private key file not found. Create it, please.")
		}
		return privateKeyFile, merry.Prepend(err, "failed to find private key file")
	}
	return privateKeyFile, nil
}

func CreateClient(wd, address string, provider string, privateKeyFile string) (client *Client, err error) {
	client = &Client{
		workingDirectory: wd,
		provider:         provider,
		privateKeyFile:   privateKeyFile,
	}

	client.internalClient, err = client.getClientInstance(address)
	return
}

type Client struct {
	workingDirectory string
	provider         string
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
		err = merry.Prepend(err, "failed to get private_key for ssh client")
		return
	}

	var signer ssh.Signer
	if signer, err = ssh.ParsePrivateKey(keyBytes); err != nil {
		err = merry.Prepend(err, "failed to parse private_key for ssh client")
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
