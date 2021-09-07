package db

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func createCommonCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, databaseTag, dbURL string, dbPool int, addPool int) (fc *commonCluster) {
	fc = &commonCluster{
		k:                      k,
		sc:                     sc,
		wd:                     wd,
		tg:                     databaseTag,
		DBUrl:                  dbURL,
		portForwardControlChan: make(chan struct{}),
		clusterSpec: ClusterSpec{
			Pods: make([]*v1.Pod, 0, 10),
		},
		dbPool:  dbPool,
		addPool: addPool,
	}
	return
}

type commonCluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
	tg string

	clusterSpec            ClusterSpec
	portForwardControlChan chan struct{}

	DBUrl string

	dbPool int

	addPool int
}

func (cc *commonCluster) deploy() (err error) {
	llog.Infof("Prepare deploy of %s\n", cc.tg)

	deployConfigDirectory := cc.wd
	if err = cc.k.LoadDirectory(deployConfigDirectory, "/home/ubuntu/"); err != nil {
		return
	}
	llog.Infof("copying %s directory: success\n", cc.tg)

	var sshSession engineSsh.Session
	if sshSession, err = cc.sc.GetNewSession(); err != nil {
		return merry.Prependf(err, "failed to open ssh connection for deploy of %s", cc.tg)
	}

	// \todo: вынести в gracefulShutdown, если вообще в этом требуется необходимость, поскольку runtime при выходе закроет сам
	// defer sshSession.Close()

	llog.Infof("%s deploy started\n", cc.tg)
	deployCmd := fmt.Sprintf("chmod +x %s/deploy_operator.sh && ./%s/deploy_operator.sh", cc.tg, cc.tg)

	var stdout io.Reader
	if stdout, err = sshSession.StdoutPipe(); err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}
	go engine.HandleReader(bufio.NewReader(stdout))

	var textb []byte
	if textb, err = sshSession.CombinedOutput(deployCmd); err != nil {
		return merry.Prependf(err, "command execution failed, return text `%s`", string(textb))
	}

	llog.Infof("%s deploy finished", cc.tg)
	return
}

func (cc *commonCluster) examineCluster(tag, targetNamespace,
	clusterMainPodName, clusterWorkerPodName string) (err error) {

	var pods []v1.Pod
	if pods, err = cc.k.ListPods(kubernetes.ResourceDefaultNamespace); err != nil {
		err = merry.Prepend(err, "list pods")
		return
	}

	printPodContainers := func(pod *v1.Pod) {
		for _, c := range pod.Spec.Containers {
			llog.Debugf("\tfound (%s, `%s`, '%s') container in pod '%s'",
				c.Name, strings.Join(c.Args, " "), strings.Join(c.Command, " "), pod.Name)
		}
		llog.Debug("\t---------------------\n\n")
	}

	for i := 0; i < len(pods); i++ {
		pPod := &pods[i]

		llog.Debugf("examining pod: '%s'/'%s'", pPod.Name, pPod.GenerateName)
		if strings.HasPrefix(pPod.Name, clusterMainPodName) {
			llog.Infof("%s main pod is '%s'", tag, pPod.Name)
			printPodContainers(pPod)
			cc.clusterSpec.MainPod = pPod
		} else if strings.HasPrefix(pPod.Name, clusterWorkerPodName) {
			cc.clusterSpec.Pods = append(cc.clusterSpec.Pods, pPod)
			printPodContainers(pPod)
		}
	}

	if cc.clusterSpec.MainPod == nil {
		return fmt.Errorf("%s main pod does not exists", tag)
	}

	if cc.clusterSpec.MainPod.Status.Phase != v1.PodRunning {
		cc.clusterSpec.MainPod, err = cc.k.WaitPod(cc.clusterSpec.MainPod.Name,
			targetNamespace,
			kubernetes.PodWaitingWaitCreation,
			kubernetes.PodWaitingTime10Minutes)
		if err != nil {
			return merry.Prependf(err, "%s pod wait", tag)
		}
	}
	llog.Debugf("%s main pod '%s' in status '%s', okay",
		tag, cc.clusterSpec.MainPod.Name, cc.clusterSpec.MainPod.Status.Phase)
	return
}

func (cc *commonCluster) openPortForwarding(name string, portMap []string) (err error) {
	var reqURL *url.URL
	reqURL, err = cc.k.GetResourceURL(kubernetes.ResourcePodName,
		kubernetes.ResourceDefaultNamespace,
		name,
		kubernetes.SubresourcePortForwarding)
	if err != nil {
		return
	}

	err = cc.k.OpenPortForward(cc.tg, portMap, reqURL,
		cc.portForwardControlChan)
	if err != nil {
		return merry.Prependf(err, "failed to started port-forward for '%s'", cc.tg)
	}

	llog.Infoln("Port-forwarding for postgres is started success")
	return
}
