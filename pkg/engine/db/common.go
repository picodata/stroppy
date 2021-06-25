package db

import (
	"bufio"
	"fmt"
	"io"
	"net/url"

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
	}
	return
}

type commonCluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
	tg string

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

	llog.Infof("Starting deploy of %s\n", cc.tg)
	deployCmd := fmt.Sprintf("chmod +x %s/deploy_operator.sh && ./%s/deploy_operator.sh", cc.tg, cc.tg)

	var stdout io.Reader
	if stdout, err = sshSession.StdoutPipe(); err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}
	go engine.HandleReader(bufio.NewReader(stdout))

	if _, err = sshSession.CombinedOutput(deployCmd); err != nil {
		return merry.Prepend(err, "command execution failed")
	}

	llog.Infof("Finished deploy of %s\n", cc.tg)
	return
}

func (cc *commonCluster) openPortForwarding(name string, portMap []string) (err error) {
<<<<<<< HEAD
=======
	stopPortForwardPostgres := make(chan struct{})
	readyPortForwardPostgres := make(chan struct{})
	errorPortForwardPostgres := make(chan error)

>>>>>>> fix(deploy): return executing OpenPortForward() for pg by goroutine and add pg url for CreateBasePayload()
	var reqURL *url.URL
	reqURL, err = cc.k.GetResourceURL(kubernetes.ResourcePodName,
		kubernetes.ResourceDefaultNamespace,
		name,
		kubernetes.ResourcePortForwarding)
	if err != nil {
		return
	}

<<<<<<< HEAD
	err = cc.k.OpenPortForward(cluster.Postgres, portMap, reqURL,
		cc.portForwardControlChan)
	if err != nil {
		return merry.Prepend(err, "failed to started port-forward for foundationdb")
	}

	llog.Infoln("Port-forwarding for postgres is started success")
	return
=======
	go cc.k.OpenPortForward("postgres", []string{"6432:5432"}, reqURL,
		stopPortForwardPostgres, readyPortForwardPostgres, errorPortForwardPostgres)

	select {
	case <-readyPortForwardPostgres:
		return nil
	case errPortForwardPostgres := <-errorPortForwardPostgres:
		llog.Errorf("Port-forwarding for postgres is started failed\n")
		return merry.Prepend(errPortForwardPostgres, "failed to started port-forward for postgres")
	}

>>>>>>> fix(deploy): return executing OpenPortForward() for pg by goroutine and add pg url for CreateBasePayload()
}
