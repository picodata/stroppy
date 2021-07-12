package chaos

import (
	"net/url"
	"path/filepath"
	"sync"

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
}

func (chaos *workableController) Deploy() (err error) {
	llog.Infoln("Starting chaos-mesh deployment...")

	if err = chaos.k.Execute(deployChaosMesh); err != nil {
		return merry.Prepend(err, "chaos-mesh deployment failed")
	}
	llog.Debugln("chaos-mesh prepared successfully")

	// прокидываем порты, что бы можно было открыть веб-интерфейс
	var reqURL *url.URL
	reqURL, err = chaos.k.GetResourceURL(kubernetes.ResourceService,
		chaosNamespace,
		chaosDashboardResourceName,
		kubernetes.SubresourcePortForwarding)
	if err != nil {
		return merry.Prepend(err, "failed to get url")
	}
	llog.Debugf("received next url for chaos port-forward: '%s'", reqURL.String())

	_err := chaos.k.OpenPortForward(chaosDashboardResourceName,
		[]string{"2333:2333"},
		reqURL,
		chaos.portForwardStopChan)
	if _err != nil {
		llog.Warn(merry.Prepend(_err, "chaos dashboard port-forwarding"))
		// return merry.Prepend(err, "port-forward is not established")
	}

	// \todo: вынести в gracefulShutdown, если вообще в этом требуется необходимость, поскольку runtime при выходе закроет сам
	// defer sshSession.Close()

	_ = chaos.k.OpenSecureShellTunnel(chaosDashboardResourceName, 2333, 2334)

	llog.Infoln("chaos-mesh deployed successfully")
	return
}

func (chaos *workableController) ExecuteCommand(scenarioName string) (err error) {
	llog.Infof("now starting chaos '%s' scenario\n", scenarioName)

	scenario := createScenario(scenarioName, chaos.wd)
	if err = chaos.k.LoadFile(scenario.sourcePath, scenario.destinationPath); err != nil {
		return merry.Prepend(err, "load file failed")
	}
	llog.Debugf("full chaos command object is '%v'\n", scenario)

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
