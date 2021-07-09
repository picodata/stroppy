package db

import (
	"bufio"
	"fmt"
	"io"
	"net/url"

	v1 "k8s.io/api/core/v1"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
)

func createCommonCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, databaseTag string) (fc *commonCluster) {
	fc = &commonCluster{
		k:                      k,
		sc:                     sc,
		wd:                     wd,
		tg:                     databaseTag,
		portForwardControlChan: make(chan struct{}),
		clusterSpec: ClusterSpec{
			Pods: make([]*v1.Pod, 0, 10),
		},
	}
	return
}

type commonCluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
	tg string

	clusterSpec ClusterSpec

	portForwardControlChan chan struct{}
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
