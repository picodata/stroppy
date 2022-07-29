/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package engine

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

const (
	RW_ROOT_MODE int = 0o600 //nolint // constant
	RSA_KEY_BITS int = 4096  //nolint // constant
)

// IsLocalPortOpen
// проверить доступность порта на localhost
func IsLocalPortOpen(port int) bool {
	address := "localhost:" + strconv.Itoa(port)
	conn, err := net.Listen("tcp", address)
	if err != nil {
		llog.Errorf("port %v at localhost is not available \n", port)
		return false
	}

	if err = conn.Close(); err != nil {
		llog.Errorf("failed to close port %v at localhost after check\n", port)
		return false
	}
	return true
}

// IsRemotePortOpen - проверить доступность порта на удаленной машине кластера
func IsRemotePortOpen(hostname string, port int) bool {
	address := hostname + ":" + strconv.Itoa(port)
	conn, err := net.Dial("tcp", address)
	if err != nil {
		llog.Errorf("port %d at '%s' is not available: %v \n", port, address, err)
		return false
	}

	_ = conn.Close()
	return true
}

// FilterPipe вывводит буфер стандартного вывода в отдельном потоке
func FilterPipe(reader io.Reader, expectedWaitChan bool) (waitChannel chan struct{}) {
	if expectedWaitChan {
		waitChannel = make(chan struct{})
	}

	bufReader := bufio.NewReader(reader)
	go func() {
		printOutput := llog.GetLevel() == llog.DebugLevel
		for {
			str, err := bufReader.ReadString('\n')
			if err != nil {
				break
			}
			if printOutput {
				llog.Debugln(str)
			}
		}

		if expectedWaitChan {
			fmt.Printf("\n\n\n\nYeah, baby! chan id is %v\n\n\n\n\n", waitChannel)
			waitChannel <- struct{}{}
			close(waitChannel)
		}
	}()

	return
}

func IsFileExists(workingDirectory string, file string) bool {
	privateKeyPath := filepath.Join(workingDirectory, file)

	if _, err := os.Stat(privateKeyPath); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		llog.Errorf("failed to find file %v: %v\n", file, err)
		return false
	}
	return true
}

func IsDirExists(checkedDir string) error {
	if _, err := os.Stat(checkedDir); os.IsExist(err) {
		return merry.Prepend(err, "Directory does not exists")
	} else if err != nil {
		return merry.Prepend(err, "Error then checking directory")
	}

	return nil
}

//nolint:forbidigo // print used not for debug
func AskNextAction(workDir string) error {
	var (
		userHome   string
		envDefined bool
		err        error
	)

	llog.Debugf("Enter interactive ssh key creation loop. Working directory: `%s`", workDir)

	// if ssh key
	reader := bufio.NewReader(os.Stdin)

	// check that HOME environment variabled is defined
	if userHome, envDefined = os.LookupEnv("HOME"); !envDefined {
		userHome = workDir
	}

	menu := "Stroppy can not find `id_rsa` private key, what action should be taken?\n" +
		"1) Create new id_rsa private key.\n" +
		fmt.Sprintf("2) Copy id_rsa key file from `%s/.ssh`.\n", userHome) +
		"3) Abort execution.\n" +
		"> "

	fmt.Print(menu)

	for {
		answer, _ := reader.ReadString('\n')
		answer = strings.ToLower(strings.ReplaceAll(answer, "\n", ""))

		llog.Tracef("User command: %s", answer)

		switch {
		case strings.EqualFold(answer, "1"):
			if err = CreatePrivateKey(path.Join(workDir, ".ssh", "id_rsa")); err != nil {
				llog.Errorf("Failed to create ssh private key: %s\n%s", err, menu)

				continue
			}

			return nil
		case strings.EqualFold(answer, "2"):
			if err = CopyFileContents(
				path.Join(userHome, ".ssh", "id_rsa"),
				path.Join(workDir, ".ssh", "id_rsa"),
				os.FileMode(RW_ROOT_MODE), //nolint:nosnakecase // constant
			); err != nil {
				llog.Errorf("Failed to copy id_rsa file: %s\n%s", err, menu)

				continue
			}

			return nil
		case strings.EqualFold(answer, "3"):
			return merry.Errorf(
				"Aborted on checking ssh key files. Please create keys manually. Exiting...",
			)
		case strings.EqualFold(answer, "0"):
			fmt.Print(menu)

			continue
		default:
			fmt.Println("Please type '1', '2', '3' or '0' to print menu.") // nolint
		}

		fmt.Print("> ")
	}
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func CopyFileContents(src, dst string, mode os.FileMode) error {
	var (
		err   error
		bytes []byte
	)

	llog.Debugf("Copying file %s to %s", src, dst)

	if bytes, err = os.ReadFile(src); err != nil {
		return merry.Prepend(err, fmt.Sprintf("Error then reading file %s", src))
	}

	if err = os.WriteFile(dst, bytes, mode); err != nil {
		return merry.Prepend(
			err,
			fmt.Sprintf("Error then copying bytes from %s to %s", src, dst),
		)
	}

	llog.Infof("File %s successfully copied to %s", src, dst)

	return nil
}

//nolint:nosnakecase // because RSA_KEY_BITS and RW_ROOT_MODE is constants
func CreatePrivateKey(fileName string) error {
	var (
		privateKey *rsa.PrivateKey
		err        error
	)

	llog.Infoln("Creating new id_rsa key file")
	llog.Debugf("Rsa key file target path %s", fileName)

	if privateKey, err = rsa.GenerateKey(rand.Reader, RSA_KEY_BITS); err != nil {
		return merry.Prepend(err, "Error then generating ssh private key")
	}

	if err = privateKey.Validate(); err != nil {
		return merry.Prepend(err, "Error then validating ssh private key")
	}

	if err = os.WriteFile(
		fileName,
		pem.EncodeToMemory(&pem.Block{
			Type:    "RSA PRIVATE KEY",
			Headers: map[string]string{},
			Bytes:   x509.MarshalPKCS1PrivateKey(privateKey),
		}),
		fs.FileMode(RW_ROOT_MODE),
	); err != nil {
		return merry.Prepend(err, "Error then writing id_rsa private key file")
	}

	llog.Infoln("New id_rsa key file successfully created")

	return nil
}

//nolint:nosnakecase // because RSA_KEY_BITS and RW_ROOT_MODE is constants
func CreatePublicKey(privKeyFileName, pubKeyFileName string) error {
	var (
		bytes []byte
		err   error
	)

	llog.Infoln("Creating new id_rsa.pub key file")
	llog.Debugf("Rsa key file target path %s", pubKeyFileName)
	llog.Debugf("Source private key file name %s", privKeyFileName)

	if bytes, err = os.ReadFile(privKeyFileName); err != nil {
		return merry.Prepend(err, "Error then reading private key file")
	}

	var pemBlock *pem.Block

	if pemBlock, _ = pem.Decode(bytes); pemBlock == nil {
		return merry.Prepend(err, "Error then decoding private key file")
	}

	var (
		privateKey    *rsa.PrivateKey
		rawPrivateKey interface{}
		isPrivateKey  bool
	)

	switch pemBlock.Type {
	case "OPENSSH PRIVATE KEY":
		if rawPrivateKey, err = ssh.ParseRawPrivateKey(pemBlock.Bytes); err != nil {
			return merry.Prepend(err, "Error then parsing OPENSSH PRIVATE KEY")
		}

		if privateKey, isPrivateKey = rawPrivateKey.(*rsa.PrivateKey); !isPrivateKey {
			return merry.Prepend(err, "Error then casting rawPrivateKey to rsa.PrivateKey")
		}
	case "RSA PRIVATE KEY":
		if privateKey, err = x509.ParsePKCS1PrivateKey(pemBlock.Bytes); err != nil {
			return merry.Prepend(err, "Error then parsing RSA PRIVATE KEY")
		}
	}

	var publicKey ssh.PublicKey

	if publicKey, err = ssh.NewPublicKey(privateKey.Public()); err != nil {
		return merry.Prepend(err, "Error then generating ssh public key")
	}

	if err = os.WriteFile(
		pubKeyFileName,
		ssh.MarshalAuthorizedKey(publicKey),
		fs.FileMode(RW_ROOT_MODE),
	); err != nil {
		return merry.Prepend(err, "Error then writing id_rsa.pub key file")
	}

	llog.Infoln("New id_rsa.pub key file successfully created")

	return nil
}
