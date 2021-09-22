package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/ansel1/merry"
	"github.com/bramvdbogaerde/go-scp"
	"github.com/bramvdbogaerde/go-scp/auth"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"golang.org/x/crypto/ssh"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	cp "k8s.io/kubectl/pkg/cmd/cp"
)

func (k *Kubernetes) LoadFile(sourceFilePath, destinationFilePath string) (err error) {
	if err = k.installSshKeyFileOnMaster(); err != nil {
		return
	}

	// не уверен, что для кластера нам нужна проверка публичных ключей на совпадение, поэтому ssh.InsecureIgnoreHostKey
	var clientSSHConfig ssh.ClientConfig
	clientSSHConfig, err = auth.PrivateKey("ubuntu", k.sshKeyFilePath, ssh.InsecureIgnoreHostKey())
	if err != nil {
		return
	}

	masterFullAddress := fmt.Sprintf("%v:22", k.AddressMap["external"]["master"])

	client := scp.NewClient(masterFullAddress, &clientSSHConfig)
	if err = client.Connect(); err != nil {
		return merry.Prepend(err, "Couldn't establish a connection to the server for copy rsa to master")
	}

	var sourceFile *os.File
	if sourceFile, err = os.Open(sourceFilePath); err != nil {
		return merry.Prependf(err, "failed to open local file '%s'", sourceFilePath)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			llog.Warnf("failed to close local descriptor for '%s' file: %v",
				sourceFilePath, err)
		}
	}()

	if err = client.CopyFile(sourceFile, destinationFilePath, "0664"); err != nil {
		return merry.Prepend(err, "error while copying file metrics-server.yaml")
	}

	client.Close()
	return
}

func (k *Kubernetes) LoadDirectory(directorySourcePath, destinationPath string) (err error) {
	destinationPath = fmt.Sprintf("ubuntu@%s:%s", k.AddressMap["external"]["master"], destinationPath)

	copyDirectoryCmd := exec.Command("scp", "-r", "-i", k.sshKeyFilePath, "-o", "StrictHostKeyChecking=no",
		directorySourcePath, destinationPath)

	llog.Infof("now loading '%s' directory to kubernetes master destination '%s' (keyfile '%s', wd: '%s')",
		directorySourcePath, destinationPath, k.sshKeyFilePath, copyDirectoryCmd.Dir)

	var output []byte
	if output, err = copyDirectoryCmd.CombinedOutput(); err != nil {
		return merry.Prependf(err, "error while copying directory to k8 master: %v, output: '%s'",
			err, string(output))
	}

	return
}

func (k Kubernetes) DownloadFile(remoteFullSourceFilePath, localPath string) (err error) {
	return
}

/* loadFilesToMaster
 * скопировать на мастер-ноду private key для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
func (k *Kubernetes) loadFilesToMaster() (err error) {
	masterExternalIP := k.AddressMap["external"]["master"]
	llog.Infoln(masterExternalIP)

	if k.provider == "yandex" {
		/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
		если кластер пока не готов*/
		llog.Infoln("Checking status of port 22 on the cluster's master...")
		var masterPortAvailable bool
		for i := 0; i <= connectionRetryCount; i++ {
			masterPortAvailable = engine.IsRemotePortOpen(masterExternalIP, 22)
			if !masterPortAvailable {
				llog.Infof("status of check the master's port 22:%v. Repeat #%v", errPortCheck, i)
				time.Sleep(execTimeout * time.Second)
			} else {
				break
			}
		}
		if !masterPortAvailable {
			return merry.Prepend(errPortCheck, "master's port 22 is not available")
		}
	}

	metricsServerFilePath := filepath.Join(k.WorkingDirectory, "monitoring", "metrics-server.yaml")
	if err = k.LoadFile(metricsServerFilePath, "/home/ubuntu/metrics-server.yaml"); err != nil {
		return
	}
	llog.Infoln("copying metrics-server.yaml: success")

	ingressGrafanaFilePath := filepath.Join(k.WorkingDirectory, "monitoring", "ingress-grafana.yaml")
	if err = k.LoadFile(ingressGrafanaFilePath, "/home/ubuntu/ingress-grafana.yaml"); err != nil {
		return
	}
	llog.Infoln("copying ingress-grafana.yaml: success")

	grafanaDirectoryPath := filepath.Join(k.WorkingDirectory, "monitoring", "grafana-on-premise")
	if err = k.LoadDirectory(grafanaDirectoryPath, "/home/ubuntu"); err != nil {
		return
	}
	llog.Infoln("copying grafana-on-premise: success")

	return
}

func (k *Kubernetes) LoadFileToPodV2(podName, containerName, sourcePath, destinationPath string) (err error) {
	ioStreams, _, _, _ := genericclioptions.NewTestIOStreams()
	copyOptions := cp.NewCopyOptions(ioStreams)

	src := sourcePath
	copyOptions.ClientConfig, err = k.getKubeConfig()
	if err != nil {
		return merry.Prepend(err, "failed to get kube config for copy file to pod")
	}

	GroupName := ""
	SchemeGroupVersion := schema.GroupVersion{Group: GroupName, Version: "v1"}
	copyOptions.ClientConfig.GroupVersion = &SchemeGroupVersion
	copyOptions.ClientConfig.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: scheme.Codecs}
	// llog.Infoln(copyOptions.ClientConfig)

	copyOptions.Clientset, err = k.GetClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientset for copy file to pod")
	}

	destSecretFile := fmt.Sprintf("/root/deploy/%v", k.clusterConfigFile)
	dest := fmt.Sprintf("%v/%v:%v", ResourceDefaultNamespace, stroppyPodName, destSecretFile)
	args := []string{src, dest}
	if err := copyOptions.Run(args); err != nil {
		llog.Errorln(err.Error(), copyOptions.ErrOut)
		return merry.Prepend(err, "failed to run copy file")
	}
	return nil
}

func (k *Kubernetes) LoadFileToPod(podName, containerName, sourcePath, destinationPath string) (err error) {
	var restConfig *rest.Config
	if restConfig, err = k.getKubeConfig(); err != nil {
		err = merry.Prepend(err, "failed to get kubeconfig for clientSet")
		return
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

	return
}

func (k *Kubernetes) CopyFileFromPodToPod(sourcePath string, destinationPath string) (err error) {
	return
}

func (k *Kubernetes) LoadFileFromPod(podName, sourcePath, kubeMasterFsPath string) (err error) {
	return
}
