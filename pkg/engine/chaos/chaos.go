package chaos

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
)

func createWorkableController(k *kubernetes.Kubernetes, wd string) (c Controller) {
	c = &workableController{
		wd: filepath.Join(wd, "chaos"),
		k:  k,

		runningScenarios:     map[string]scenario{},
		runningScenariosLock: sync.Mutex{},

		portForwardStopChan: make(chan struct{}),
	}
	return
}

type workableController struct {
	k  *kubernetes.Kubernetes
	wd string

	runningScenarios     map[string]scenario
	runningScenariosLock sync.Mutex

	portForwardStopChan chan struct{}

	controllerPod, dashboardPod *v1.Pod
}

func (chaos *workableController) enumChaosParts() (err error) {
	var pods []v1.Pod
	if pods, err = chaos.k.ListPods(chaosNamespace); err != nil {
		return
	}

	for i := 0; i < len(pods); i++ {
		pod := pods[i]
		llog.Debugf("examining pod: '%s'/'%s'", pod.Name, pod.GenerateName)

		if strings.HasPrefix(pod.Name, chaosDashboardResourceName) {
			chaos.dashboardPod = pod.DeepCopy()
			llog.Infof("chaos dashboard pod is '%s'", pod.Name)
		} else if strings.HasPrefix(pod.Name, chaosControlManagerName) {
			chaos.controllerPod = pod.DeepCopy()
			llog.Infof("chaos control management pod is '%s'", pod.Name)
		}
	}

	if chaos.dashboardPod == nil {
		return errors.New("chaos dashboard pod not found")
	}
	if chaos.controllerPod == nil {
		return errors.New("chaos control manager pod not found")
	}

	return
}

func (chaos *workableController) establishDashboardAvailability() (err error) {
	// прокидываем порты, что бы можно было открыть веб-интерфейс
	var reqURL *url.URL
	reqURL, err = chaos.k.GetResourceURL(kubernetes.ResourceService,
		chaosNamespace,
		chaos.dashboardPod.Name,
		kubernetes.SubresourcePortForwarding)
	if err != nil {
		return merry.Prepend(err, "failed to get url")
	}

	err = chaos.k.OpenPortForward(chaos.dashboardPod.Name,
		[]string{"2333:2333"},
		reqURL,
		chaos.portForwardStopChan)
	if err != nil {
		// return merry.Prepend(err, "port-forward is not established")
		err = nil
	}

	// \todo: вынести в gracefulShutdown, если вообще в этом требуется необходимость, поскольку runtime при выходе закроет сам
	// defer sshSession.Close()

	_ = chaos.k.OpenSecureShellTunnel(chaosDashboardResourceName, 2333, 2334)
	return
}

func (chaos *workableController) Deploy() (err error) {
	llog.Infoln("Starting chaos-mesh deployment...")

	if err = chaos.k.Execute(deployChaosMesh); err != nil {
		return merry.Prepend(err, "chaos-mesh deployment failed")
	}
	llog.Debugln("chaos-mesh prepared successfully")

	if err = chaos.enumChaosParts(); err != nil {
		return
	}

	const rbacFileName = "rbac.yaml"
	rbacFileSourcePath := filepath.Join(chaos.wd, ".config", rbacFileName)
	rbacFileKubemasterPath := filepath.Join("/home/ubuntu", rbacFileName)
	if err = chaos.k.LoadFile(rbacFileSourcePath, rbacFileKubemasterPath); err != nil {
		return merry.Prepend(err, "rbac.yaml copying")
	}

	const rbacApplyCommand = "kubectl apply -f %s"
	if err = chaos.k.ExecuteF(rbacApplyCommand, rbacFileKubemasterPath); err != nil {
		return merry.Prepend(err, "apply rbac.yaml")
	}

	if err = chaos.establishDashboardAvailability(); err != nil {
		return
	}

	llog.Infoln("chaos-mesh deployed successfully")
	return
}

func (chaos *workableController) ExecuteCommand(scenarioName string) (err error) {
	llog.Infof("now starting chaos '%s' scenario", scenarioName)

	scenario := createScenario(scenarioName, chaos.wd)
	if err = chaos.k.LoadFile(scenario.sourcePath, scenario.destinationPath); err != nil {
		return merry.Prepend(err, "load file failed")
	}
	llog.Debugf("full chaos command object is '%v'", scenario)

	if err = chaos.k.ExecuteF("kubectl apply -f %s", scenario.destinationPath); err != nil {
		return merry.Prepend(err, "scenario run failed")
	}

	chaos.runningScenariosLock.Lock()
	defer chaos.runningScenariosLock.Unlock()
	chaos.runningScenarios[scenario.scenarioName] = scenario

	return
}

func (chaos *workableController) Stop() {
	chaos.runningScenariosLock.Lock()
	defer chaos.runningScenariosLock.Unlock()

	var err error
	for _, s := range chaos.runningScenarios {
		if s.isRunning {
			llog.Infof("stopping chaos scenario '%s'\n", s.scenarioName)
			if err = chaos.k.ExecuteF("kubectl delete -f %s", s.destinationPath); err != nil {
				llog.Warnf("'%s' scenario not stopped: %v", s.destinationPath, err)
			}
		}
	}
}
