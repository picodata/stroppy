package kubernetes

import (
	"errors"
	v1 "k8s.io/api/core/v1"
	"net/url"
	"path/filepath"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/terraform"
	"k8s.io/client-go/kubernetes"
)

const (
	sshNotFoundCode = 127

	// кол-во попыток подключения при ошибке
	connectionRetryCount = 3
)

var errPortCheck = errors.New("port Check failed")

var errProviderChoice = errors.New("selected provider not found")

func CreateKubernetes(settings *config.Settings,
	terraformAddressMap terraform.MapAddresses,
	sshClient engineSsh.Client) (k *Kubernetes, err error) {

	k = &Kubernetes{
		workingDirectory:  settings.WorkingDirectory,
		clusterConfigFile: filepath.Join(settings.WorkingDirectory, "config"),

		addressMap: terraformAddressMap,
		sc:         sshClient,

		provider:       settings.DeploySettings.Provider,
		sessionIsLocal: settings.Local,

		isSshKeyFileOnMaster: false,
	}
	k.sshKeyFileName, k.sshKeyFilePath = k.sc.GetPrivateKeyInfo()

	llog.Infof("kubernetes init success on directory '%s', with provider '%s', and ssh key file '%s'",
		k.workingDirectory, k.provider, k.sshKeyFilePath)
	return
}

type Kubernetes struct {
	workingDirectory  string
	clusterConfigFile string

	addressMap terraform.MapAddresses

	sshKeyFileName string
	sshKeyFilePath string
	sshTunnel      *engineSsh.Result
	sc             engineSsh.Client

	isSshKeyFileOnMaster bool
	sessionIsLocal       bool

	portForward *engineSsh.Result

	provider string

	stroppyPod *v1.Pod
}

func (k *Kubernetes) GetClientSet() (*kubernetes.Clientset, error) {
	_config, err := k.getKubeConfig()
	if err != nil {
		return nil, merry.Prepend(err, "failed to get kubeconfig for clientSet")
	}

	// clientSet - клиент для обращения к группам сущностей k8s
	var clientSet *kubernetes.Clientset
	if clientSet, err = kubernetes.NewForConfig(_config); err != nil {
		return nil, merry.Prepend(err, "failed to create clientSet")
	}

	return clientSet, nil
}

func (k *Kubernetes) GetResourceURL(resource, namespace, name, subresource string) (url *url.URL, err error) {
	var clientSet *kubernetes.Clientset
	if clientSet, err = k.GetClientSet(); err != nil {
		return
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	url = clientSet.CoreV1().RESTClient().Post().
		Resource(resource).
		Namespace(namespace).
		Name(name).
		SubResource(subresource).URL()
	return
}

func (k *Kubernetes) Stop() {
	defer k.sshTunnel.Tunnel.Close()
	llog.Infoln("status of ssh tunnel close: success")

	// defer k.portForward.Tunnel.Close()
	// llog.Infoln("status of port-forward's close: success")
}
