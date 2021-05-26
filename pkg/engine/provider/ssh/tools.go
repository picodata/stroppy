package ssh

import (
	"fmt"
	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/sshtunnel"
	"golang.org/x/crypto/ssh"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
)

// copyConfigFromMaster - скопировать файд kube config c мастер-инстанса кластера и применить для использования
func copyConfigFromMaster() error {
	mapIP, err := getIPMapping()
	if err != nil {
		return merry.Prepend(err, "failed to get IP addresses for copy from master")
	}

	connectCmd := fmt.Sprintf("ubuntu@%v:/home/ubuntu/.kube/config", mapIP.masterExternalIP)
	copyFromMasterCmd := exec.Command("scp", "-i", "id_rsa", "-o", "StrictHostKeyChecking=no", connectCmd, ".")
	llog.Infoln(copyFromMasterCmd.String())
	copyFromMasterCmd.Dir = terraformWorkDir

	_, err = copyFromMasterCmd.CombinedOutput()
	if err != nil {
		return merry.Prepend(err, "failed to execute command copy from master")
	}

	// подменяем адрес кластера, т.к. будет открыт туннель по порту 6443 к мастеру
	clusterURL := "https://localhost:6443"
	if err = editClusterURL(clusterURL); err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	return nil
}

// openSSHTunnel - открыть ssh-соединение и передать указатель на него вызывающему коду для управления
func openSSHTunnel(sshTunnelChan chan sshResult) {
	mapIP, err := getIPMapping()
	if err != nil {
		log.Printf("failed to get IP addresses for open ssh tunnel:%v ", err)
		sshTunnelChan <- sshResult{0, nil, err}
	}
	mastersConnectionString := fmt.Sprintf("ubuntu@%v", mapIP.masterExternalIP)

	/*	проверяем доступность портов для postgres на локальной машине */
	llog.Infoln("Checking the status of port 6443 of the localhost for k8s...")
	k8sPort := clusterK8sPort
	if !isLocalPortOpen(k8sPort) {
		llog.Infoln("Checking the status of port 6444 of the localhost for k8s...")
		// проверяем резервный порт в случае недоступности основного
		k8sPort = reserveClusterK8sPort
		if !isLocalPortOpen(k8sPort) {
			sshTunnelChan <- sshResult{0, nil, merry.Prepend(errPortCheck, "ports 6443 and 6444 are not available")}
		}

		// подменяем порт в kubeconfig на локальной машине
		clusterURL := fmt.Sprintf("https://localhost:%v", reserveClusterK8sPort)
		if err = editClusterURL(clusterURL); err != nil {
			llog.Infof("failed to replace port: %v", err)
			sshTunnelChan <- sshResult{0, nil, err}
		}
	}

	authMethod, err := sshtunnel.PrivateKeyFile("benchmark/deploy/id_rsa")
	if err != nil {
		llog.Infof("failed to use private key file: %v", err)
		sshTunnelChan <- sshResult{0, nil, err}
	}
	// Setup the tunnel, but do not yet start it yet.
	destinationServerString := fmt.Sprintf("localhost:%v", k8sPort)
	tunnel, err := sshtunnel.NewSSHTunnel(
		mastersConnectionString,
		destinationServerString,
		k8sPort,
		authMethod,
	)
	if err != nil {
		sshTunnelChan <- sshResult{0, nil, merry.Prepend(err, "failed to create tunnel")}
	}

	// You can provide a logger for debugging, or remove this line to
	// make it silent.
	tunnel.Log = log.New(os.Stdout, "SSH tunnel ", log.Flags())

	tunnelStartedChan := make(chan error, 1)
	go tunnel.Start(tunnelStartedChan)
	tunnelStarted := <-tunnelStartedChan
	close(tunnelStartedChan)

	if tunnelStarted != nil {
		sshTunnelChan <- sshResult{0, nil, merry.Prepend(err, "failed to start tunnel")}
		return
	}

	sshTunnelChan <- sshResult{k8sPort, tunnel, nil}
}

func getClientSSH(ipAddress string) (*ssh.Client, error) {
	privateKeyFile := fmt.Sprintf("%s/id_rsa", terraformWorkDir)
	privateKeyRaw, err := ioutil.ReadFile(privateKeyFile)
	if err != nil {
		return nil, merry.Prepend(err, "failed to get id_rsa for ssh client")
	}

	signer, err := ssh.ParsePrivateKey(privateKeyRaw)
	if err != nil {
		return nil, merry.Prepend(err, "failed to parse id_rsa for ssh client")
	}
	// линтер требует указания всех полей структуры при присвоении переменной
	//nolint:exhaustivestruct
	config := &ssh.ClientConfig{
		User: "ubuntu",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		//nolint:gosec
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	addr := fmt.Sprintf("%s:%d", ipAddress, 22)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, merry.Prepend(err, "failed to start ssh connection for ssh client")
	}
	return client, nil
}
