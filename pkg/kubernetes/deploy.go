/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"text/template"
	"time"

	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
)

const (
	//nolint // constant
	SSH_CONFIG string = `
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
	INVENTORY_NAME      string        = "inventory.yml" //nolint // constant
	GRAFANA_PORT        int           = 3000            //nolint // constant
	GRAFANA_REQ_TIMEOUT time.Duration = 10              //nolint // constant
	EXIT_CODE_127       int           = 127             //nolint // constant
)

type SshK8SOpts struct {
	SSHPrivateKey string
	PrivateSubnet string
	BastionPubIP  string
}

//nolint
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
func (k *Kubernetes) DeployK8S(wd string) error {
	var (
		file *os.File
		err  error
	)

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
	tmpl, err := template.New("config").Parse(SSH_CONFIG) //nolint:nosnakecase // constant
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
	if err = k.deploySelf(wd); err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	// 9. generate inventory and run kubespray ansible playbook
	if err = k.finalizeDeployment(wd); err != nil {
		return merry.Prepend(err, "failed to finalize k8s deploy")
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
		return merry.Prepend(err, "failed to deploy stroppy pod")
	}

	llog.Infoln("status of stroppy pod deploy: success")

	return nil
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

//nolint:funlen // deploy all monitoring components
// Deploy monitoring (grafana, node_exporter, promtail)
func (k *Kubernetes) deployMonitoring(workDir string) error {
	// create monitoring inventory
	var (
		file   *os.File
		length int
		bytes  []byte
		resp   *http.Response
		err    error
	)

	workDir = path.Join(workDir, "third_party/monitoring")

	if bytes, err = k.GenerateMonitoringInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing monitoring inventory")
	}

	if file, err = os.OpenFile(
		path.Join(workDir, INVENTORY_NAME), //nolint:nosnakecase // constant
		os.O_RDWR|os.O_CREATE|os.O_TRUNC,   //nolint:nosnakecase // constant
		os.FileMode(roAll),
	); err == nil {
		if length, err = file.Write(bytes); err != nil {
			return merry.Prepend(err, "Error then writing monitoring inventory")
		}

		llog.Tracef(
			"%v bytes successfully written to %v",
			length,
			path.Join(workDir, INVENTORY_NAME), //nolint:nosnakecase // constant
		)
	}

	//nolint:nosnakecase // constant
	// next variables is wrappers around ansible
	connOpts, PBOpts := createAnsibleOpts(path.Join(workDir, INVENTORY_NAME))

	ansiblePrivelegeOpts := options.AnsiblePrivilegeEscalationOptions{
		Become:        false,
		BecomeMethod:  "",
		BecomeUser:    "",
		AskBecomePass: false,
	}

	grafanaURL := url.URL{
		Scheme: "http",
		Opaque: "",
		User:   &url.Userinfo{},
		Host: net.JoinHostPort(
			k.Engine.AddressMap["external"]["master"],
			fmt.Sprintf("%d", GRAFANA_PORT), //nolint
		),
		Path:        "",
		RawPath:     "",
		ForceQuery:  false,
		RawQuery:    "",
		Fragment:    "",
		RawFragment: "",
	}

	//nolint:nosnakecase // constant
	// check that grafana is deployed
	llog.Tracef(
		"grafana uri %s",
		grafanaURL.String,
	)

	contextWithTimeout, closeFn := context.WithTimeout(
		context.Background(),
		GRAFANA_REQ_TIMEOUT*time.Second, //nolint // constant
	)

	if resp, err = sendGetWithContext(
		contextWithTimeout,
		nil,
		grafanaURL.String(),
	); err != nil || (resp.StatusCode > 500 && resp.StatusCode < 599) {
		llog.Infoln("Grafana is not deployed yet, run grafana playbook")

		llog.Tracef("Response: %v", resp)

		grafanaPlaybook := &playbook.AnsiblePlaybookCmd{
			Binary:                     "",
			Exec:                       nil,
			Playbooks:                  []string{path.Join(workDir, "grafana.yml")},
			Options:                    PBOpts,
			ConnectionOptions:          connOpts,
			PrivilegeEscalationOptions: &ansiblePrivelegeOpts,
			StdoutCallback:             "",
		}

		closeFn()

		if err = grafanaPlaybook.Run(context.TODO()); err != nil {
			return merry.Prepend(err, "Error then running monitoring playbooks")
		}
	} else {
		llog.Infoln("Grafana already deployed. skipping")

		closeFn()
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	nodePlaybook := &playbook.AnsiblePlaybookCmd{
		Binary:                     "",
		Exec:                       nil,
		Playbooks:                  []string{path.Join(workDir, "nodeexp.yml")},
		Options:                    PBOpts,
		ConnectionOptions:          connOpts,
		PrivilegeEscalationOptions: &ansiblePrivelegeOpts,
		StdoutCallback:             "",
	}

	if err = nodePlaybook.Run(context.TODO()); err != nil {
		return merry.Prepend(err, "Error then running monitoring playbooks")
	}

	return nil
}

// Deploy kubernetes cluster and all dependent software
// Function execution order
// 1. Check that kubernetes already deployed
// 2. Deploy kubernetes via kubespray
func (k *Kubernetes) deploySelf(workDir string) error {
	// create kubespray inventory
	var (
		file   *os.File
		length int
		bytes  []byte
		err    error
	)

	inventoryDir := path.Join(workDir, "third_party", "kubespray", "inventory", "stroppy")

	// run on bastion (master) host shell command `kubectl get pods`
	// if command returns something (0 or 127 exit code) kubernetes is deployed
	if k.checkMasterDeploymentStatus() {
		return nil
	}

	if bytes, err = k.generateK8sInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing kubespray inventory")
	}

	//nolint:nosnakecase // constant
	if file, err = os.Create(path.Join(inventoryDir, INVENTORY_NAME)); err != nil {
		return merry.Prepend(err, "Error then creating kubespray inventory file")
	}

	if length, err = file.Write(bytes); err != nil {
		return merry.Prepend(err, "Error then creating kubespray inventory")
	}

	llog.Tracef(
		"%v bytes successfully written to %v",
		length,
		path.Join(inventoryDir, INVENTORY_NAME), //nolint:nosnakecase // constant
	)

	//nolint:nosnakecase // constant
	// next variable is an ansible interaction objects
	connOpts, PBOpts := createAnsibleOpts(path.Join(inventoryDir, INVENTORY_NAME))

	ansiblePrivelegeOptions := &options.AnsiblePrivilegeEscalationOptions{
		Become:        true,
		BecomeMethod:  "",
		BecomeUser:    "",
		AskBecomePass: false,
	}

	workDir = path.Join(workDir, "third_party", "kubespray")

	playbook := &playbook.AnsiblePlaybookCmd{
		Binary:                     "",
		Exec:                       nil,
		Playbooks:                  []string{path.Join(workDir, "cluster.yml")},
		Options:                    PBOpts,
		ConnectionOptions:          connOpts,
		PrivilegeEscalationOptions: ansiblePrivelegeOptions,
		StdoutCallback:             "",
	}

	// run kubespray ansible playbook
	if err = playbook.Run(context.Background()); err != nil {
		return merry.Prepend(err, "Error then running kubespray playbook")
	}

	return nil
}

// Function for final k8s deployment steps
// 1. Get kube config from master node
// 2. Install grafana ingress into cluster
// 3. Install metrics server
func (k *Kubernetes) finalizeDeployment(workDir string) error {
	llog.Debugln("Run 'finalize deployment' playbooks")

	// create generic inventory
	var (
		file   *os.File
		length int
		bytes  []byte
		err    error
	)

	workDir = path.Join(workDir, "third_party/extra")

	if bytes, err = k.generateSimplifiedInventory(); err != nil {
		return merry.Prepend(err, "Error then serializing generic inventory")
	}

	//nolint:nosnakecase // constant
	if file, err = os.Create(path.Join(workDir, INVENTORY_NAME)); err != nil {
		return merry.Prepend(err, "Error then creating ansible inventory")
	}

	if length, err = file.Write(bytes); err != nil {
		return merry.Prepend(err, "Error then creating generic inventory")
	}

	//nolint:nosnakecase // constant
	llog.Tracef("%v bytes successfully written to %v", length, path.Join(workDir, INVENTORY_NAME))

	//nolint:nosnakecase // constant
	// next variables is wrappers around ansible
	connOpts, PBOpts := createAnsibleOpts(path.Join(workDir, INVENTORY_NAME))

	privOpts := options.AnsiblePrivilegeEscalationOptions{
		Become:        false,
		BecomeMethod:  "",
		BecomeUser:    "",
		AskBecomePass: false,
	}

	finalizePlaybook := &playbook.AnsiblePlaybookCmd{
		Binary:                     "",
		Exec:                       nil,
		Playbooks:                  []string{path.Join(workDir, "finalize.yml")},
		Options:                    PBOpts,
		ConnectionOptions:          connOpts,
		PrivilegeEscalationOptions: &privOpts,
		StdoutCallback:             "",
	}

	if err = finalizePlaybook.Run(context.Background()); err != nil {
		return merry.Prepend(err, "Error then finalizing deployment with finalize playbook")
	}

	manifestsPlaybook := &playbook.AnsiblePlaybookCmd{
		Binary:                     "",
		Exec:                       nil,
		Playbooks:                  []string{path.Join(workDir, "manifests.yml")},
		Options:                    PBOpts,
		ConnectionOptions:          connOpts,
		PrivilegeEscalationOptions: &privOpts,
		StdoutCallback:             "",
	}

	if err = manifestsPlaybook.Run(context.Background()); err != nil {
		return merry.Prepend(err, "Error then finalizing deployment with manifests playbook")
	}

	return nil
}

func (k *Kubernetes) Stop() {
	defer k.KubernetesPort.Tunnel.Close()
	llog.Infoln("status of ssh tunnel close: success")
}

// Run kubectl command and try to retrieve all not running pods
// If command executed with non zero (or 127) exit code then return false.
func (k *Kubernetes) checkMasterDeploymentStatus() bool {
	var (
		err    error
		cmd    *exec.Cmd
		stdout []byte
		json   map[string]interface{}
	)

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

		if cmd.ProcessState.ExitCode() != EXIT_CODE_127 { //nolint:nosnakecase // constant
			llog.Warnf("Error then retrieving k8s cluster status: %v", err)

			return false
		}
	}

	llog.Tracef("kubectl stdout:\n%v", string(stdout))

	if err = yaml.Unmarshal(stdout, &json); err != nil {
		llog.Warnf("Error then deserializing kubectl response: %v", err)

		return false
	}

	items, ok := json["items"].([]interface{})
	if !ok {
		return false
	}

	if len(items) > 0 {
		llog.Warnln("Cluster already deployed but not healthy")

		return false
	}

	llog.Infoln("Cluster already deployed and running")

	return true
}
