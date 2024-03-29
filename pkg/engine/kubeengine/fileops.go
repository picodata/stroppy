/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubeengine

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"

	"gitlab.com/picodata/stroppy/pkg/state"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/ansel1/merry"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

func (e *Engine) LoadFile(
	sourceFilePath, destinationFilePath string,
	shellState *state.State,
) (err error) {
	if err = e.installSSHKeyFileOnMaster(shellState); err != nil {
		return
	}

	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение, поэтому ssh.InsecureIgnoreHostKey
	var clientSSHConfig ssh.ClientConfig

	//#nosec
	if clientSSHConfig, err = auth.PrivateKey(
		"stroppy",
		e.sshKeyFilePath,
		ssh.InsecureIgnoreHostKey(),
	); err != nil {
		return
	}

	masterFullAddress := fmt.Sprintf(
		"%v:22",
		shellState.InstanceAddresses.GetFirstMaster().External,
	)

	client := scp.NewClient(masterFullAddress, &clientSSHConfig)
	if err = client.Connect(); err != nil {
		return merry.Prepend(
			err,
			"Couldn't establish a connection to the server for copy rsa to master",
		)
	}

	var sourceFile *os.File
	if sourceFile, err = os.Open(sourceFilePath); err != nil {
		return merry.Prependf(err, "failed to open local file '%s'", sourceFilePath)
	}
	defer func() {
		if err = sourceFile.Close(); err != nil {
			llog.Warnf("failed to close local '%s' descriptor: %v",
				sourceFilePath, err)
		}
	}()

	if err = client.CopyFile(sourceFile, destinationFilePath, "0664"); err != nil {
		return merry.Prepend(err, "error while copying file metrics-server.yaml")
	}

	client.Close()
	return
}

// / Run few shell commands on remote host, and copy files via scp.
func (e *Engine) LoadDirectory(
	directorySourcePath, destinationPath string,
	shellState *state.State,
) error {
	var err error

	if err = e.ExecuteF(`mkdir -p "%s"`, destinationPath); err != nil {
		return merry.Prepend(err, fmt.Sprintf("path creation failed: %v", err))
	}

	destinationPath = fmt.Sprintf(
		"stroppy@%s:%s",
		shellState.InstanceAddresses.GetFirstMaster().External,
		destinationPath,
	)

	copyDirectoryCmd := exec.Command(
		"scp", "-r", "-i",
		e.sshKeyFilePath,
		"-o",
		"StrictHostKeyChecking=no",
		directorySourcePath,
		destinationPath,
	)

	llog.Infof(
		"now loading '%s' directory to kubernetes master destination '%s' (keyfile '%s', wd: '%s')",
		directorySourcePath,
		destinationPath,
		e.sshKeyFilePath,
		copyDirectoryCmd.Dir,
	)

	var output []byte
	if output, err = copyDirectoryCmd.CombinedOutput(); err != nil {
		return merry.Prependf(err, "error while copying directory to k8 master: %v, output: '%s'",
			err, string(output))
	}

	return nil
}

func (e Engine) DownloadFile(remoteFullSourceFilePath, localPath string) (err error) {
	return
}

func (e *Engine) LoadFileToPod(
	podName, containerName, sourcePath, destinationPath string,
) error {
	var (
		restConfig *rest.Config
		err        error
	)

	if restConfig, err = e.GetKubeConfig(); err != nil {
		return merry.Prepend(err, "failed to get kubeconfig for clientSet")
	}

	restConfig.Host = "localhost:6444"

	var coreClient *corev1client.CoreV1Client
	if coreClient, err = corev1client.NewForConfig(restConfig); err != nil {
		return merry.Prepend(err, "failed to get client")
	}

	var reader io.Reader
	if reader, err = os.Open(sourcePath); err != nil {
		return merry.Prependf(err, "source file '%s'", sourcePath)
	}

	path, err := os.Getwd()
	if err != nil {
		return merry.Prependf(err, "failed to get work durectory")
	}

	sourceFullPath := fmt.Sprintf("%v/%v", path, sourcePath)

	req := coreClient.RESTClient().
		Post().
		Namespace(ResourceDefaultNamespace).
		Resource(ResourcePodName).
		Name(podName).
		SubResource(SubresourceExec).
		VersionedParams(&v1.PodExecOptions{
			Container: containerName,
			Command:   []string{"cp", sourceFullPath, destinationPath},
			Stdin:     true,
			Stdout:    false,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	var _exec remotecommand.Executor
	if _exec, err = remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL()); err != nil {
		return merry.Prepend(err, "exec get")
	}

	var stderr bytes.Buffer
	err = _exec.Stream(remotecommand.StreamOptions{
		Stdin:  reader,
		Stderr: &stderr,
		Tty:    false,
	})
	if err != nil {
		return merry.Prependf(err, "command exec failed, stderr: `%s`", stderr.String())
	}

	return nil
}

func (e *Engine) CopyFileFromPodToPod(sourcePath string, destinationPath string) (err error) {
	return
}

func (e *Engine) LoadFileFromPod(podName, sourcePath, kubeMasterFsPath string) (err error) {
	return
}
