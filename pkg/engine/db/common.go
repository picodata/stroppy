package db

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/database/cluster"
	"gitlab.com/picodata/stroppy/pkg/engine"
	"gitlab.com/picodata/stroppy/pkg/engine/kubernetes"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/provider/ssh"
	k8s "k8s.io/client-go/kubernetes"
)

func createCommonCluster(sc engineSsh.Client, k *kubernetes.Kubernetes, wd, databaseTag string) (fc *commonCluster) {
	fc = &commonCluster{
		k:  k,
		sc: sc,
		wd: wd,
		tg: databaseTag,
	}
	return
}

type commonCluster struct {
	sc engineSsh.Client
	k  *kubernetes.Kubernetes
	wd string
	tg string
}

func (cc *commonCluster) deploy() (err error) {
	llog.Infof("Prepare deploy of %s\n", cc.tg)

	deployConfigDirectory := cc.wd
	if err = cc.k.LoadFile(deployConfigDirectory, fmt.Sprintf("/home/ubuntu/%s", cc.tg)); err != nil {
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
	stopPortForwardPostgres := make(chan struct{})
	readyPortForwardPostgres := make(chan struct{})

	var clientSet *k8s.Clientset
	if clientSet, err = cc.k.GetClientSet(); err != nil {
		return merry.Prepend(err, "failed to get clientSet for open port-forward of postgres")
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	reqURL := clientSet.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace("default").
		Name(name).
		SubResource("portforward").URL()

	err = cc.k.OpenPortForward(cluster.Postgres, portMap, reqURL,
		stopPortForwardPostgres, readyPortForwardPostgres)
	if err != nil {
		return merry.Prepend(err, "failed to started port-forward for postgres")
	}

	select {
	case <-readyPortForwardPostgres:
		llog.Infof("Port-forwarding for postgres is started success\n")
	}
	return
}
