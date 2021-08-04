package ssh

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

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

func createSshClient(wd, address, provider string) (cc Client, err error) {
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

// ExecuteCommandWorker - выполнить команду на определенном воркере с сохранением результата
func ExecuteCommandWorker(workingDirectory, address, text, provider string) (result []byte, err error) {
	client, err := CreateClient(workingDirectory, address, provider, RemoteClient)
	if err != nil {
		return nil, merry.Prepend(err, "failed to create ssh client")
	}

	var commandSessionObject Session

	if commandSessionObject, err = client.GetNewSession(); err != nil {
		return nil, merry.Prepend(err, "failed to get ssh session")
	}

	defer commandSessionObject.Close()

	llog.Debugf("executing of commands:%v \n", text)

	if result, err = commandSessionObject.CombinedOutput(text); err != nil {
		// проверка на длину массива добавлена для случая, когда grep возвращает пустую строку, что приводит к exit code 1
		if len(result) != 0 {
			return nil, merry.Prependf(err, "terraform command exec failed with output `%s`", string(result))
		}
	}

	llog.Debugln("result of commands оn worker: ", string(result))

	return
}

func IsExistEntity(address string, checkCommand string, checkString string, workingDirectory string, provider string) (checkResult bool, err error) {
	var CmdResult []byte
	if CmdResult, err = ExecuteCommandWorker(workingDirectory, address, checkCommand, provider); err != nil {
		if err != nil {
			errorMessage := fmt.Sprintf("failed to execute command on worker %v", address)
			return false, merry.Prepend(err, errorMessage)
		}
	}

	if strings.Contains(string(CmdResult), checkString) {

		llog.Infoln("entity already exist or parted")
		return true, nil
	}

	llog.Infoln("entity has not been exist yet")
	return false, nil
}
