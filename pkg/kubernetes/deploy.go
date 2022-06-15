/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"context"
	"os"
	"path"
	"strings"
	"text/template"

	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	engineSsh "gitlab.com/picodata/stroppy/pkg/engine/ssh"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
)

const (
	SSHConfig string = `
StrictHostKeyChecking no

Host {{.PrivateSubnet}}
  ProxyJump bastion
  User ubuntu
  IdentityFile {{.SSHPrivateKey}}

Host bastion
  HostName {{.BastionPubIP}}
  User ubuntu
  IdentityFile {{.SSHPrivateKey}}
  ControlMaster auto
  ControlPath ansible-kubespray-%r@%h:%p
  ControlPersist 5m
`
	KubesprayInventoryPath string = "inventory/stroppy/inventory.yml"
	InventoryName string = "inventory.yml"
)

type SshK8SOpts struct {
	SSHPrivateKey string
	PrivateSubnet string
	BastionPubIP  string
}

/// Deploy kubernetes and other infrastructure
/// #steps:
/// 1. Create directory for ssh config if it is not exists
/// 2. Write ssh config to created in previous step directory
/// 3. Copy id_rsa to .ssh directory
/// 4. Generate ansible requirements
/// 5. Generate ansible cfg
/// 6. install ansible galaxy roles
/// 7. Generate inventory for grafana and deploy
/// 8. Generate inventory for kubespray and deploy
/// 9. Apply grafana manifests
/// 10. Deploy DB operator
/// 11. Deploy container with stroppy
func (k *Kubernetes) DeployAll(wd string) (err error) {
	// 1. Create ssh config file for kubespray
	err = os.Mkdir(path.Join(wd, ".ssh"), os.ModePerm)
	if err != nil {
		merry.Prepend(err, "Error then creating ssh config directory")
	}

	// 2. Create and template ssh config
	var file *os.File
	file, err = os.Create(path.Join(wd, ".ssh/config"))
	if err != nil {
		llog.Infoln("Error then creating ssh config file")
	}
	ssh_opts := SshK8SOpts{
		".ssh/id_rsa",
		strings.ReplaceAll(k.Engine.AddressMap["subnet"]["ip_v4"], "0/24", "*"),
		k.Engine.AddressMap["external"]["master"],
	}

	// replace template values to shh config variables
	tmpl, err := template.New("config").Parse(SSHConfig)
	if err != nil {
		merry.Prepend(err, "Error then parsing ssh config template")
	}
	err = tmpl.Execute(file, ssh_opts)
	if err != nil {
		merry.Prepend(err, "Error then templating ssh config")
	}

	// 3. copy id_rsa file
	if err = copyFileContents(
		path.Join(wd, "id_rsa"),
		path.Join(wd, ".ssh/id_rsa"),
	); err != nil {
		return merry.Prepend(err, "failed to deploy grafana")
	}

	// 4. generate ansible requirements
	if err = writeAnsibleRequirements(wd); err != nil {
		return merry.Prepend(err, "Error then generating ansible requirements")
	}

	// 5. generate ansible config
	if err = writeAnsibleConfig(wd); err != nil {
		return merry.Prepend(err, "Error then generating ansible config")
	}

	// 6. install ansible galaxy roles
	if err = installGalaxyRoles(); err != nil {
		return merry.Prepend(err, "failed to intall galaxy roles")
	}

	// 7. run grafana on premise ansible playbook
	if err = k.deployMonitoring(wd); err != nil {
		return merry.Prepend(err, "failed to deploy grafana")
	}

	// 8. generate inventory and run kubespray ansible playbook
	if err = k.deployKubernetes(wd); err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	// 9. generate inventory and run kubespray ansible playbook
	if err = k.finalizeDeployment(wd); err != nil {
		return merry.Prepend(err, "failed to finalie k8s deploy")
	}

	k.Engine.SetClusterConfigFile(path.Join(wd, ".kube/config"))
	if err = k.Engine.EditClusterURL(clusterK8sPort); err != nil {
		return merry.Prepend(err, "failed to edit cluster's url in kubeconfig")
	}

	k.KubernetesPort = k.Engine.OpenSecureShellTunnel(kubeengine.SshEntity, clusterK8sPort)
	if k.KubernetesPort.Err != nil {
		err = merry.Prepend(k.KubernetesPort.Err, "failed to create ssh tunnel")
		return
	}
	llog.Infoln("status of creating ssh tunnel for the access to k8s: success")

	if err = k.Engine.AddNodeLabels(kubeengine.ResourceDefaultNamespace); err != nil {
		return merry.Prepend(err, "failed to add labels to cluster nodes")
	}

	k.StroppyPod = stroppy.CreateStroppyPod(k.Engine)
	if err = k.StroppyPod.Deploy(); err != nil {
		err = merry.Prepend(err, "failed to deploy stroppy pod")
		return
	}

	llog.Infoln("status of stroppy pod deploy: success")
	return
}

func (k *Kubernetes) OpenPortForwarding() (err error) {
	k.MonitoringPort = k.Engine.OpenSecureShellTunnel(monitoringSshEntity, clusterMonitoringPort)
	if k.MonitoringPort.Err != nil {
		return merry.Prepend(k.MonitoringPort.Err, "cluster monitoring")
	}

	llog.Infoln("status of creating ssh tunnel for the access to monitoring: success")
	return
}

func (k *Kubernetes) Shutdown() {
	k.MonitoringPort.Tunnel.Close()
}

// Deploy monitoring (grafana, node_exporter, promtail)
func (k *Kubernetes) deployMonitoring(wd string) (err error) {
	wd = path.Join(wd, "third_party/monitoring")

	// create kubespray inventory
	if _, err = os.Stat(path.Join(wd, InventoryName)); err != nil {
		llog.Traceln(err)
		var inv *os.File
		inv, err = os.Create(path.Join(wd, InventoryName))
		if err != nil {
			return merry.Prepend(err, "Error then creating monitoring inventory")
		}
		inv.Write(k.generateSimpleInventory())
	}

	// next variables is wrappers around ansible
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection:   "ssh",
		SSHExtraArgs: "-F .ssh/config",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(wd, InventoryName),
	}
	playbook := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "grafana.yml"), path.Join(wd, "node.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	if err = playbook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then running monitoring playbooks")
	}

	return
}

// Deploy kubernetes cluster and all dependent software
// Function execution order
// 1. Check that kubernetes already deployed
// 2. Deploy kubernetes via kubespray
func (k *Kubernetes) deployKubernetes(wd string) (err error) {
	wd = path.Join(wd, "third_party/kubespray")

	// run on bastion (master) host shell command `kubectl get pods`
	// if command returns something (0 or 127 exit code) kubernetes is deployed
	var isDeployed bool
	if isDeployed, err = k.checkMasterDeploymentStatus(); err != nil {
		return merry.Prepend(err, "failed to Check deploy k8s in master node")
	}

	// early return if cluster already deployed
	if isDeployed {
		llog.Infoln("k8s already success deployed")
		return
	}

	// create kubespray inventory
	if _, err = os.Stat(path.Join(wd, KubesprayInventoryPath)); err != nil {
		llog.Traceln(err)
		var inv *os.File
		inv, err = os.Create(path.Join(wd, KubesprayInventoryPath))
		if err != nil {
			return merry.Prepend(err, "Error then creating kubespray inventory")
		}
		inv.Write(k.generateK8sInventory())
	}

	// next variables is an ansible interaction objects
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection:   "ssh",
		SSHExtraArgs: "-F .ssh/config",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(wd, KubesprayInventoryPath),
	}
	playbook := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "cluster.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	// run kubespray ansible playbook
	if err = playbook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then running kubespray playbook")
	}

	return
}

// Function for final k8s deployment steps
// 1. Get kube config from master node
// 2. Install grafana ingress into cluster
// 3. Install metrics server
func (k *Kubernetes) finalizeDeployment(wd string) (err error) {
	wd = path.Join(wd, "third_party/finalize")

	// create kubespray inventory
	if _, err = os.Stat(path.Join(wd, InventoryName)); err != nil {
		llog.Traceln(err)
		var inv *os.File
		inv, err = os.Create(path.Join(wd, InventoryName))
		if err != nil {
			return merry.Prepend(err, "Error then creating `finalize` inventory")
		}
		inv.Write(k.generateSimpleInventory())
	}

	// next variables is wrappers around ansible
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection:   "ssh",
		SSHExtraArgs: "-F .ssh/config",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(wd, InventoryName),
	}
	playbook := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "finalize.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	if err = playbook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then finalizing deployment with finalize playbook")
	}

	return
}

func (k *Kubernetes) Stop() {
	defer k.KubernetesPort.Tunnel.Close()
	llog.Infoln("status of ssh tunnel close: success")
}

// checkMasterDeploymentStatus
// проверяет, что все поды k8s в running, что подтверждает успешность разворачивания k8s
func (k *Kubernetes) checkMasterDeploymentStatus() (bool, error) {
	masterExternalIP := k.Engine.AddressMap["external"]["master"]

	commandClientType := engineSsh.RemoteClient
	if k.Engine.UseLocalSession {
		commandClientType = engineSsh.LocalClient
	}

	sshClient, err := engineSsh.CreateClient(k.Engine.WorkingDirectory,
		masterExternalIP,
		k.provider.Name(),
		commandClientType)
	if err != nil {
		return false, merry.Prependf(
			err,
			"failed to establish ssh client to '%s' address",
			masterExternalIP,
		)
	}

	checkSession, err := sshClient.GetNewSession()
	if err != nil {
		return false, merry.Prepend(err, "failed to open ssh connection for Check deploy")
	}

	const checkCmd = "kubectl get pods --all-namespaces"
	resultCheckCmd, err := checkSession.CombinedOutput(checkCmd)
	if err != nil {
		e, ok := err.(*ssh.ExitError)
		if !ok {
			return false, merry.Prepend(err, "failed сheck deploy k8s")
		}

		// если вернулся not found(код 127), это норм, если что-то другое - лучше проверить
		const sshNotFoundCode = 127
		if e.ExitStatus() == sshNotFoundCode {
			return false, nil
		}
	}

	countPods := strings.Count(string(resultCheckCmd), "Running")
	if countPods < runningPodsCount {
		return false, nil
	}

	_ = checkSession.Close()
	return true, nil
}
