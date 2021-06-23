package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	llog "github.com/sirupsen/logrus"

	"github.com/ansel1/merry"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"golang.org/x/crypto/ssh"
)

type Result struct {
	Port   int
	Tunnel *sshtunnel.SSHTunnel
	Err    error
}

func getPrivateKeyFileName(provider string, workingDirectory string) (privateKeyFileName string, err error) {
	// проверяем наличие приватного ключа
	switch provider {
	case "yandex":
		// переименовать единообразно ключи нельзя, т.к. Yandex.Cloud ожидает именно id_rsa
		privateKeyFileName = "id_rsa"
	case "oracle":
		privateKeyFileName = "private_key.pem"
	}

	privateKeyFilePath := filepath.Join(workingDirectory, privateKeyFileName)
	if _, err = os.Stat(privateKeyFilePath); err != nil {
		if os.IsNotExist(err) {
			return privateKeyFileName, merry.Prepend(err, "private key file not found. Create it, please.")
		}
		return privateKeyFileName, merry.Prepend(err, "failed to find private key file")
	}
	return
}

func createClient(wd, address, provider string) (cc Client, err error) {
	c := &client{
		workingDirectory: wd,
		provider:         provider,
	}

	if c.keyFileName, err = getPrivateKeyFileName(provider, wd); err != nil {
		return
	}
	c.keyFilePath = filepath.Join(wd, c.keyFileName)

	if c.keyFileBytes, err = ioutil.ReadFile(c.keyFilePath); err != nil {
		err = merry.Prependf(err, "failed to read '%s' key file content", c.keyFilePath)
		return
	}

	if c.internalClient, err = c.getClientInstance(address); err != nil {
		return
	}
	llog.Infof("remote secure shell client created")

	cc = c
	return
}

type client struct {
	workingDirectory string
	provider         string

	keyFileName  string
	keyFilePath  string
	keyFileBytes []byte

	internalClient *ssh.Client
}

func (sc *client) GetNewSession() (session Session, err error) {
	session, err = sc.internalClient.NewSession()
	return
}

func (sc *client) getClientInstance(address string) (client *ssh.Client, err error) {
	var signer ssh.Signer
	if signer, err = ssh.ParsePrivateKey(sc.keyFileBytes); err != nil {
		err = merry.Prepend(err, "failed to parse private key for ssh client")
		return
	}

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

func (sc *client) GetPrivateKeyInfo() (string, string) {
	return sc.keyFileName, sc.keyFilePath
}
