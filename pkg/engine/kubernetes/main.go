package kubernetes

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"

	"k8s.io/client-go/rest"

	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/config"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	"k8s.io/client-go/kubernetes"
)

const (
	sshNotFoundCode = 127

	// кол-во попыток подключения при ошибке
	connectionRetryCount = 3
)

var (
	errPortCheck      = errors.New("port Check failed")
	errProviderChoice = errors.New("selected provider not found")
)

func CreateShell(settings *config.Settings) (k *Kubernetes, err error) {
	kubernetesMasterAddress := settings.TestSettings.KubernetesMasterAddress
	if kubernetesMasterAddress == "" {
		err = fmt.Errorf("kubernetes master address is empty")
		return
	}

	commandClientType := engineSsh.RemoteClient
	var sc engineSsh.Client
	sc, err = engineSsh.CreateClient(settings.WorkingDirectory,
		kubernetesMasterAddress,
		settings.DeploySettings.Provider,
		commandClientType)
	if err != nil {
		err = merry.Prependf(err, "setup ssh tunnel to '%s'", kubernetesMasterAddress)
		return
	}

	k = createKubernetesObject(settings, nil, sc)
	return
}

func createKubernetesObject(settings *config.Settings,
	terraformAddressMap map[string]map[string]string,
	sshClient engineSsh.Client) (pObj *Kubernetes) {

	pObj = &Kubernetes{
		workingDirectory:  settings.WorkingDirectory,
		clusterConfigFile: filepath.Join(settings.WorkingDirectory, "config"),

		AddressMap: terraformAddressMap,
		sc:         sshClient,

		provider:        settings.DeploySettings.Provider,
		useLocalSession: settings.Local,

		isSshKeyFileOnMaster: false,
	}
	return
}

func CreateKubernetes(settings *config.Settings,
	terraformAddressMap map[string]map[string]string,
	sshClient engineSsh.Client) (k *Kubernetes, err error) {

	k = createKubernetesObject(settings, terraformAddressMap, sshClient)
	k.sshKeyFileName, k.sshKeyFilePath = k.sc.GetPrivateKeyInfo()

	llog.Infof("kubernetes init success on directory '%s', with provider '%s', and ssh key file '%s'",
		k.workingDirectory, k.provider, k.sshKeyFilePath)
	return
}

type Kubernetes struct {
	workingDirectory  string
	clusterConfigFile string

	AddressMap map[string]map[string]string

	sshKeyFileName string
	sshKeyFilePath string
	sshTunnel      *engineSsh.Result
	sc             engineSsh.Client

	isSshKeyFileOnMaster bool
	useLocalSession      bool

	portForward *engineSsh.Result

	provider string

	StroppyPod *v1.Pod
}

func (k *Kubernetes) GetClientSet() (clientSet *kubernetes.Clientset, err error) {
	var _config *rest.Config
	if _config, err = k.getKubeConfig(); err != nil {
		err = merry.Prepend(err, "failed to get kubeconfig for clientSet")
		return
	}

	// clientSet - клиент для обращения к группам сущностей k8s
	if clientSet, err = kubernetes.NewForConfig(_config); err != nil {
		return nil, merry.Prepend(err, "failed to create clientSet")
	}

	return
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
}
