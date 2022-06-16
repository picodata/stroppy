/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

const (
	SSHConfig string = `
StrictHostKeyChecking no
ConnectTimeout 120
UserKnownHostsFile=/dev/null

Host {{.PrivateSubnet}}
  ProxyJump master
  User ubuntu
  IdentityFile {{.SSHPrivateKey}}

Host master
  HostName {{.BastionPubIP}}
  User ubuntu
  IdentityFile {{.SSHPrivateKey}}
  ControlMaster auto
  ControlPath ansible-kubespray-%r@%h:%p
  ControlPersist 5m
`
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

	// Force colored output for ansible playbooks
	options.AnsibleForceColor()

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
		return merry.Prepend(err, "failed to deploy monitoring")
	}

	// 8. generate inventory and run kubespray ansible playbook
	if err = k.deployKubernetes(wd); err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	// 9. generate inventory and run kubespray ansible playbook
	if err = k.finalizeDeployment(wd); err != nil {
		return merry.Prepend(err, "failed to finalie k8s deploy")
	}

	// 10. set path variable to kubeconfig file
	// by default kubeconfig everytime in ~/.kube/config
	k.Engine.SetClusterConfigFile(fmt.Sprintf("%s/.kube/config", os.Getenv("HOME")))

	// 11. Add nodes labels
	if err = k.Engine.AddNodeLabels(kubeengine.ResourceDefaultNamespace); err != nil {
		return merry.Prepend(err, "failed to add labels to cluster nodes")
	}

	// 12. Create stroppy deployment with one pod on master node
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

	// create monitoring inventory
	var file *os.File
	var l int
	var b []byte
	var resp *http.Response

	if b, err = k.GenerateMonitoringInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing monitoring inventory")
	}
	if file, err = os.OpenFile(path.Join(wd, InventoryName), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644); err == nil {
		if l, err = file.Write(b); err != nil {
			return merry.Prepend(err, "Error then writing monitoring inventory")
		}
		llog.Tracef("%v bytes successfully written to %v", l, path.Join(wd, InventoryName))
	}
	// next variables is wrappers around ansible
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection: "ssh",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(wd, InventoryName),
	}

	// check that grafana is deployed
	resp, err = http.DefaultClient.Get(
		fmt.Sprintf("http://%s:%v", k.Engine.AddressMap["external"]["master"], 3000),
	)
	llog.Tracef("grafana uri http://%s:%v", k.Engine.AddressMap["external"]["master"], 3000)
	if err != nil || resp.StatusCode != 200 {
		llog.Infoln("Grafana is not deployed yet, run grafana playbook")
		pb := &playbook.AnsiblePlaybookCmd{
			Playbooks:         []string{path.Join(wd, "grafana.yml")},
			ConnectionOptions: ansible_connection_options,
			Options:           ansible_playbook_opts,
		}

		if err = pb.Run(context.TODO()); err != nil {
			return merry.Prepend(err, "Error then running monitoring playbooks")
		}
	} else {
		llog.Infoln("Grafana already deployed. skipping")
	}

	pb := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "nodeexp.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	if err = pb.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then running monitoring playbooks")
	}

	return
}

// Deploy kubernetes cluster and all dependent software
// Function execution order
// 1. Check that kubernetes already deployed
// 2. Deploy kubernetes via kubespray
func (k *Kubernetes) deployKubernetes(wd string) (err error) {
	inventoryDir := path.Join(wd, "third_party", "kubespray", "inventory", "stroppy")

	// create kubespray inventory
	var file *os.File
	var l int
	var b []byte

	// run on bastion (master) host shell command `kubectl get pods`
	// if command returns something (0 or 127 exit code) kubernetes is deployed
	if k.checkMasterDeploymentStatus() {
		return
	}

	if b, err = k.generateK8sInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing kubespray inventory")
	}
	file, err = os.OpenFile(
		path.Join(inventoryDir, InventoryName),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,
		0o644,
	)
	if err == nil {
		if l, err = file.Write(b); err != nil {
			return merry.Prepend(err, "Error then creating kubespray inventory")
		}
		llog.Tracef(
			"%v bytes successfully written to %v",
			l,
			path.Join(inventoryDir, InventoryName),
		)
	} else {
		if file, err = os.Create(path.Join(inventoryDir, InventoryName)); err != nil {
			return merry.Prepend(err, "Error then creating kubespray inventory")
		}
		if l, err = file.Write(b); err != nil {
			return merry.Prepend(err, "Error then writing kubespray inventory")
		}
		llog.Tracef("%v bytes successfully written to %v", l, path.Join(inventoryDir, InventoryName))
	}

	// next variables is an ansible interaction objects
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection: "ssh",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(inventoryDir, InventoryName),
	}
	ansible_privilege_opts := &options.AnsiblePrivilegeEscalationOptions{
		Become: true,
	}

	wd = path.Join(wd, "third_party", "kubespray")
	playbook := &playbook.AnsiblePlaybookCmd{
		Playbooks:                  []string{path.Join(wd, "cluster.yml")},
		ConnectionOptions:          ansible_connection_options,
		Options:                    ansible_playbook_opts,
		PrivilegeEscalationOptions: ansible_privilege_opts,
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
	llog.Debugln("Run 'finalize deployment' playbooks")
	wd = path.Join(wd, "third_party/extra")

	// create generic inventory
	var file *os.File
	var l int
	var b []byte
	if b, err = k.generateSimpleInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing generic inventory")
	}
	file, err = os.OpenFile(path.Join(wd, InventoryName), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	if err == nil {
		if l, err = file.Write(b); err != nil {
			return merry.Prepend(err, "Error then creating generic inventory")
		}
		llog.Tracef("%v bytes successfully written to %v", l, path.Join(wd, InventoryName))
	} else {
		if file, err = os.Create(path.Join(wd, InventoryName)); err != nil {
			return merry.Prepend(err, "Error then creating generic inventory")
		}
		if l, err = file.Write(b); err != nil {
			return merry.Prepend(err, "Error then creating generic inventory")
		}
		llog.Tracef("%v bytes successfully written to %v", l, path.Join(wd, InventoryName))
	}

	// next variables is wrappers around ansible
	ansible_connection_options := &options.AnsibleConnectionOptions{
		Connection:   "ssh",
		SSHExtraArgs: "-F .ssh/config",
	}
	ansible_playbook_opts := &playbook.AnsiblePlaybookOptions{
		Inventory: path.Join(wd, InventoryName),
	}
	ansPlaybook := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "finalize.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	if err = ansPlaybook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then finalizing deployment with finalize playbook")
	}

	ansPlaybook = &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{path.Join(wd, "manifests.yml")},
		ConnectionOptions: ansible_connection_options,
		Options:           ansible_playbook_opts,
	}

	if err = ansPlaybook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then finalizing deployment with manifests playbook")
	}

	return
}

func (k *Kubernetes) Stop() {
	defer k.KubernetesPort.Tunnel.Close()
	llog.Infoln("status of ssh tunnel close: success")
}

// Run kubectl command and try to retrieve all not running pods
// If command executed with non zero (or 127) exit code then return false.
func (k *Kubernetes) checkMasterDeploymentStatus() bool {
	var err error
	var cmd *exec.Cmd
	var stdout []byte
	var json map[string]interface{}

	cmd = exec.Command(
		"kubectl",
		"get",
		"pods",
		"--field-selector",
		"status.phase!=Running",
		"--namespace",
		"kube-system",
		"--output",
		"json",
	)
	llog.Tracef("kubectl command '%s'", cmd)
	if stdout, err = cmd.Output(); err != nil {
		llog.Warnln("kubectl command has non zero exit code")
		if cmd.ProcessState.ExitCode() != 127 {
			llog.Warnf("Error then retrieving k8s cluster status: %v", err)
			return false
		}
	}
	llog.Tracef("kubectl stdout:\n%v", string(stdout))

	if err = yaml.Unmarshal(stdout, &json); err != nil {
		llog.Warnf("Error then deserializing kubectl response: %v", err)
		return false
	}

	if len(json["items"].([]interface{})) > 0 {
		llog.Warnln("Cluster already deployed but not healthy")
		return false
	}
	llog.Infoln("Cluster already deployed and running")

	return true
}
