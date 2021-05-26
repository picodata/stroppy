package postgres

import (
	"bufio"
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

// deployPostgres - развернуть postgres в кластере
func deployPostgres() error {
	llog.Infoln("Prepare deploy of postgres")
	mapIP, err := getIPMapping()
	if err != nil {
		return merry.Prepend(err, "failed to map IP addresses in terraform.tfstate")
	}
	masterExternalIP := mapIP.masterExternalIP

	sshClient, err := getClientSSH(masterExternalIP)
	if err != nil {
		return merry.Prepend(err, "failed to create ssh connection for deploy of postgres")
	}

	sshSession, err := sshClient.NewSession()
	if err != nil {
		return merry.Prepend(err, "failed to open ssh connection for deploy of postgres")
	}
	defer sshSession.Close()

	llog.Infoln("Starting deploy of postgres")
	deployCmd := "chmod +x deploy_operator.sh && ./deploy_operator.sh"
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		return merry.Prepend(err, "failed creating command stdoutpipe")
	}
	stdoutReader := bufio.NewReader(stdout)
	go handleReader(stdoutReader)
	_, err = sshSession.CombinedOutput(deployCmd)
	if err != nil {
		return merry.Wrap(err)
	}
	llog.Infoln("Finished deploy of postgres")
	return nil
}

func openPostgresPortForward() error {
	stopPortForwardPostgres := make(chan struct{})
	readyPortForwardPostgres := make(chan struct{})
	errorPortForwardPostgres := make(chan error)

	clientset, err := getClientSet()
	if err != nil {
		return merry.Prepend(err, "failed to get clientset for open port-forward of postgres")
	}

	// reqURL - текущий url запроса к сущности k8s в runtime
	reqURL := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace("default").
		Name("acid-postgres-cluster-0").
		SubResource("portforward").URL()

	go openKubePortForward("postgres", []string{"6432:5432"}, reqURL,
		stopPortForwardPostgres, readyPortForwardPostgres, errorPortForwardPostgres)

	select {
	case <-readyPortForwardPostgres:
		llog.Infof("Port-forwarding for postgres is started success\n")
		return nil
	case errPortForwardPostgres := <-errorPortForwardPostgres:
		llog.Errorf("Port-forwarding for postgres is started failed\n")
		return merry.Prepend(errPortForwardPostgres, "failed to started port-forward for postgres")
	}
}
