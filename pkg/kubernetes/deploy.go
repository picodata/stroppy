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

	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/stroppy"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	cconfig "k8s.io/client-go/applyconfigurations/core/v1"
	sconfig "k8s.io/client-go/applyconfigurations/storage/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
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
	inventoryName string = "inventory.yml"
	grafanaPort   int    = 3000
	exitCode127   int    = 127
)

// helm repositories.
const (
	grafanaHelmRepoURL     = "https://grafana.github.io/helm-charts"
	grafanaHelmRepoName    = "grafana"
	prometheusHelmRepoURL  = "https://prometheus-community.github.io/helm-charts"
	prometheusHelmRepoName = "prometheus-community"
	nginxHelmRepoURL       = "https://kubernetes.github.io/ingress-nginx"
	nginxHelmRepoName      = "nginx"
)

type SshK8SOpts struct {
	SSHPrivateKey string
	PrivateSubnet string
	BastionPubIP  string
}

// Deploy kubernetes and other infrastructure
// #steps:
// 1. Create directory for ssh config if it is not exists
// 2. Write ssh config to created in previous step directory
// 3. Copy id_rsa to .ssh directory
// 4. Generate ansible requirements
// 5. Generate ansible cfg
// 6. install ansible galaxy roles
// 7. Generate inventory for grafana and deploy
// 8. Generate inventory for kubespray and deploy
// 9. Apply grafana manifests
// 10. Deploy DB operator
// 11. Open ssh port forwarding
// 12. Add node labels
// 13. Deploy container with stroppy
// 14. Deploy stroppy pod.
func (k *Kubernetes) DeployK8SWithInfrastructure(shellState *state.State) error { //nolint
	var (
		file *os.File
		err  error
	)

	if file, err = os.Create(
		path.Join(shellState.Settings.WorkingDirectory, ".ssh/config"),
	); err != nil {
		llog.Infoln("Error then creating ssh config file")
	}

	ssh_opts := SshK8SOpts{
		".ssh/id_rsa",
		strings.ReplaceAll(shellState.Subnet, "0/24", "*"),
		shellState.InstanceAddresses.GetFirstMaster().External,
	}

	// replace template values to shh config variables
	tmpl, err := template.New("config").Parse(SSH_CONFIG) //nolint
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
	if err = writeAnsibleRequirements(shellState.Settings.WorkingDirectory); err != nil {
		return merry.Prepend(err, "Error then generating ansible requirements")
	}

	// 5. generate ansible config
	if err = writeAnsibleConfig(shellState.Settings.WorkingDirectory); err != nil {
		return merry.Prepend(err, "Error then generating ansible config")
	}

	// 6. install ansible galaxy roles
	if err = installGalaxyRoles(); err != nil {
		return merry.Prepend(err, "failed to intall galaxy roles")
	}

	// 7. generate inventory and run kubespray ansible playbook
	if err = k.deploySelf(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy k8s")
	}

	// 8. set path variable to kubeconfig file
	// by default kubeconfig everytime in ~/.kube/config
	k.Engine.SetClusterConfigFile(fmt.Sprintf("%s/.kube/config", os.Getenv("HOME")))

	// 9. Add nodes labels
	if err = k.Engine.AddNodeLabels(shellState); err != nil {
		return merry.Prepend(err, "failed to add labels to cluster nodes")
	}

	// 10. Deploy ingress
	if err = k.deployIngress(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy ingress")
	}

	/// 11. Deploy storage class and persistent volume
	if err = k.deployStorageClassAndPV(shellState); err != nil {
		return merry.Prepend(err, "failed to create storageClass and PV")
	}

	// 12. run grafana on premise ansible playbook
	if err = k.deployGrafana(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy grafana")
	}

	// 12. run grafana on premise ansible playbook
	if err = k.deployLoki(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy loki")
	}

	// 13. run grafana on premise ansible playbook
	if err = k.deployPromtail(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy promtail")
	}

	// 14. run grafana on premise ansible playbook
	if err = k.deployNodeExporter(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy node-exporter")
	}

	// 15. run grafana on premise ansible playbook
	if err = k.deployPrometheus(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy prometheus")
	}

	// 16. Open port forwarding
	k.KubernetesPort = k.Engine.OpenSecureShellTunnel(
		kubeengine.SSHEntity,
		shellState.InstanceAddresses.GetFirstMaster().External,
		clusterK8sPort,
	)
	if k.KubernetesPort.Err != nil {
		return merry.Prepend(k.KubernetesPort.Err, "failed to create ssh tunnel")
	}
	llog.Infoln("Status of creating ssh tunnel for the access to k8s: success")

	// 17. Create stroppy deployment with one pod on master node
	k.StroppyPod = stroppy.CreateStroppyPod(k.Engine)
	if err = k.StroppyPod.DeployNamespace(shellState); err != nil {
		return merry.Prepend(err, "failed to create stroppy namespace")
	}

	// 18. Deploy stroppy pod
	if err = k.StroppyPod.DeployPod(shellState); err != nil {
		return merry.Prepend(err, "failed to deploy stroppy pod")
	}

	llog.Infoln("Status of stroppy pod deploy: success")

	return nil
}

func (k *Kubernetes) OpenPortForwarding(shellState *state.State) error {
	k.MonitoringPort = k.Engine.OpenSecureShellTunnel(
		monitoringSshEntity,
		shellState.NodesInfo.IPs.FirstMasterIP.External,
		clusterMonitoringPort,
	)
	if k.MonitoringPort.Err != nil {
		return merry.Prepend(k.MonitoringPort.Err, "cluster monitoring")
	}

	llog.Infoln("Status of creating ssh tunnel for the access to monitoring: success")

	return nil
}

func (k *Kubernetes) Shutdown() {
	k.MonitoringPort.Tunnel.Close()
}

// Deploy kubernetes cluster and all dependent software
// Function execution order
// 1. Check that kubernetes already deployed
// 2. Deploy kubernetes via kubespray.
func (k *Kubernetes) deploySelf(shellState *state.State) error {
	// create kubespray inventory
	var (
		file   *os.File
		length int
		bytes  []byte
		err    error
	)

	inventoryDir := path.Join(
		shellState.Settings.WorkingDirectory,
		"third_party",
		"kubespray",
		"inventory",
		"stroppy",
	)

	// run on bastion (master) host shell command `kubectl get pods`
	// if command returns something (0 or 127 exit code) kubernetes is deployed
	if k.checkMasterDeploymentStatus() {
		return nil
	}

	if bytes, err = k.generateK8sInventory(shellState); err != nil {
		return merry.Prepend(err, "Error then serializing kubespray inventory")
	}

	if file, err = os.Create(path.Join(inventoryDir, inventoryName)); err != nil {
		return merry.Prepend(err, "Error then creating kubespray inventory file")
	}

	if length, err = file.Write(bytes); err != nil {
		return merry.Prepend(err, "Error then creating kubespray inventory")
	}

	llog.Tracef(
		"%v bytes successfully written to %v",
		length,
		path.Join(inventoryDir, inventoryName),
	)

	// next variable is an ansible interaction objects
	connOpts, PBOpts := createAnsibleOpts(path.Join(inventoryDir, inventoryName), shellState)

	ansiblePrivelegeOptions := &options.AnsiblePrivilegeEscalationOptions{ //nolint
		Become:        true,
		AskBecomePass: false,
	}

	workDir := path.Join(shellState.Settings.WorkingDirectory, "third_party", "kubespray")

	playbook := &playbook.AnsiblePlaybookCmd{ //nolint
		Playbooks: []string{
			path.Join(workDir, "cluster.yml"),
			path.Join(workDir, "kubeconfig.yml"),
			path.Join(workDir, "cluster_additional.yml"),
		},
		Options:                    PBOpts,
		ConnectionOptions:          connOpts,
		PrivilegeEscalationOptions: ansiblePrivelegeOptions,
	}

	ansibleContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	// run kubespray ansible playbook
	if err = playbook.Run(ansibleContext); err != nil {
		return merry.Prepend(err, "Error then running kubespray playbook")
	}

	return nil
}

func (k *Kubernetes) deployStorageClassAndPV(shellState *state.State) error {
	var err error

	storageClass := sconfig.StorageClass("local-storage")

	manifestDir := path.Join(
		shellState.Settings.WorkingDirectory, "third_party", "extra", "manifests",
	)

	if err = k.Engine.ToEngineObject(
		path.Join(manifestDir, "local-storage-class.yml"),
		&storageClass,
	); err != nil {
		return merry.Prepend(err, "failed to cast manifest into storageClass")
	}

	kubeContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	objectApplyFunc := func(clientSet *kubernetes.Clientset) error {
		if _, err = clientSet.StorageV1().StorageClasses().Apply(
			kubeContext,
			storageClass,
			k.Engine.GenerateDefaultMetav1(),
		); err != nil {
			return merry.Prepend(err, "failed to apply storageClass manifest")
		}

		return nil
	}

	if err = k.Engine.DeployObject(kubeContext, objectApplyFunc); err != nil {
		return merry.Prepend(err, fmt.Sprintln("Error then deploying storageClass"))
	}

	llog.Infoln("Local storage class creation: success")

	if err = k.deployPV(
		"local-pv-mon",
		"default",
		path.Join(manifestDir, "local-pv.yml"),
		kubeengine.NodeNameMonitoring,
		path.Join("/data", "volumes", "local-pv-mon"),
		"2Gi", //nolint
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to deploy PV local-pv-mon")
	}

	for name, node := range shellState.NodesInfo.NodesParams {
		if err = k.deployPV(
			fmt.Sprintf("local-pv-database-%d", node.Index),
			"default",
			path.Join(manifestDir, "local-pv.yml"),
			kubeengine.NodeNameDBMS,
			path.Join("/data", "volumes", "local-pv-database"),
			fmt.Sprintf("%dGi", shellState.NodesInfo.GetFirstWorker().Resources.Disk-5), //nolint
			shellState,
		); err != nil {
			return merry.Prepend(err, fmt.Sprintf("failed to deploy PV %s-%d", name, node.Index))
		}
	}

	return nil
}

func (k *Kubernetes) deployPV(
	name, namespace, manifestPath, nodeName, filesystemPath string,
	storageQantity string,
	shellState *state.State,
) error {
	var (
		err          error
		diskQuantity resource.Quantity
	)

	if err = yaml.Unmarshal([]byte(storageQantity), &diskQuantity); err != nil {
		return merry.Prepend(err, "failed to deserialize resources into Quantity")
	}

	persistentVolume := cconfig.PersistentVolume(name)

	if err = k.Engine.ToEngineObject(
		manifestPath,
		&persistentVolume,
	); err != nil {
		return merry.Prepend(err, "failed to cast manifest into persistentVolume")
	}

	inSelector := v1.NodeSelectorOpIn

	persistentVolume.Name = &name
	persistentVolume.Namespace = &namespace
	persistentVolume.Spec.Capacity = &v1.ResourceList{
		v1.ResourceName("storage"): diskQuantity,
	}
	persistentVolume.Spec.Local.Path = &filesystemPath
	persistentVolume.Spec.NodeAffinity.Required.NodeSelectorTerms = []cconfig.
		NodeSelectorTermApplyConfiguration{
		{
			MatchExpressions: []cconfig.NodeSelectorRequirementApplyConfiguration{
				{
					Key:      &nodeName,
					Operator: &inSelector,
					Values:   []string{"true"},
				},
			},
		},
	}

	kubeContext, ctxCloseFn := context.WithCancel(context.Background())
	defer ctxCloseFn()

	objectApplyFunc := func(clientSet *kubernetes.Clientset) error {
		if _, err = clientSet.CoreV1().PersistentVolumes().Apply(
			kubeContext,
			persistentVolume,
			k.Engine.GenerateDefaultMetav1(),
		); err != nil {
			return merry.Prepend(err, "failed to apply persistentVolume manifest")
		}

		return nil
	}

	if err = k.Engine.DeployObject(
		kubeContext, objectApplyFunc,
	); err != nil {
		return merry.Prepend(err, fmt.Sprintln("Error then deploying persistentVolume"))
	}

	llog.Infof("Local persistent volume %s creation: success", name)

	return nil
}

// Deploy monitoring (grafana, node_exporter, promtail)
//
//nolint:funlen // deploy all monitoring components
func (k *Kubernetes) deployGrafana(shellState *state.State) error {
	// create monitoring inventory
	var (
		file   *os.File
		length int
		bytes  []byte
		resp   *http.Response
		err    error
	)

	workDir := path.Join(shellState.Settings.WorkingDirectory, "third_party/monitoring")

	if bytes, err = k.GenerateMonitoringInventory(shellState); err != nil {
		return merry.Prepend(err, "Error then serializing monitoring inventory")
	}

	if file, err = os.OpenFile(
		path.Join(workDir, inventoryName),
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, //nolint
		os.FileMode(roAll),
	); err == nil {
		if length, err = file.Write(bytes); err != nil {
			return merry.Prepend(err, "Error then writing monitoring inventory")
		}

		llog.Tracef(
			"%v bytes successfully written to %v",
			length,
			path.Join(workDir, inventoryName),
		)
	}

	// next variables is wrappers around ansible
	connOpts, PBOpts := createAnsibleOpts(path.Join(workDir, inventoryName), shellState)

	ansiblePrivelegeOpts := options.AnsiblePrivilegeEscalationOptions{ //nolint
		Become:        false,
		AskBecomePass: false,
	}

	grafanaURL := url.URL{ //nolint
		Scheme: "http",
		Host: net.JoinHostPort(
			shellState.NodesInfo.IPs.FirstMasterIP.External,
			fmt.Sprintf("%d", shellState.Settings.DeploymentSettings.GrPort),
		),
	}

	// check that grafana is deployed
	llog.Tracef(
		"grafana uri %s",
		grafanaURL.String(),
	)

	if resp, err = getServerStatus(
		nil,
		grafanaURL.String(),
	); err != nil || (resp.StatusCode > 500 && resp.StatusCode < 599) {
		llog.Infoln("Grafana is not deployed yet, run grafana playbook")

		llog.Tracef("Response: %v", resp)

		grafanaPlaybook := &playbook.AnsiblePlaybookCmd{ //nolint
			Playbooks: []string{
				path.Join(workDir, "grafana.yml"),
				path.Join(workDir, "grafana_additional.yml"),
			},
			Options:                    PBOpts,
			ConnectionOptions:          connOpts,
			PrivilegeEscalationOptions: &ansiblePrivelegeOpts,
		}

		runCtx, grCtxCloseFn := context.WithCancel(context.Background())
		defer grCtxCloseFn()

		if err = grafanaPlaybook.Run(runCtx); err != nil {
			return merry.Prepend(err, "failed to run grafana playbook")
		}
	} else {
		llog.Infoln("Grafana deploy status: skipping")
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	return nil
}

func (k *Kubernetes) deployLoki(shellState *state.State) error {
	var (
		resp *http.Response
		err  error
	)

	monDir := path.Join(shellState.Settings.WorkingDirectory, "third_party", "monitoring")

	// next variables is wrappers around ansible
	connOpts, PBOpts := createAnsibleOpts(path.Join(monDir, "inventory.yml"), shellState)

	ansiblePrivelegeOpts := options.AnsiblePrivilegeEscalationOptions{ //nolint
		Become:        false,
		AskBecomePass: false,
	}

	lokiUrl := url.URL{ //nolint
		Scheme: "http",
		Host: net.JoinHostPort(
			shellState.NodesInfo.IPs.FirstMasterIP.External,
			fmt.Sprintf("%d", lokiPort),
		),
	}

	// check that grafana is deployed
	llog.Tracef(
		"loki uri %s",
		lokiUrl.String(),
	)

	if resp, err = getServerStatus(
		nil,
		lokiUrl.String(),
	); err != nil || (resp.StatusCode > 500 && resp.StatusCode < 599) {
		lokiPlaybook := &playbook.AnsiblePlaybookCmd{ //nolint
			Playbooks:                  []string{path.Join(monDir, "loki.yml")},
			Options:                    PBOpts,
			ConnectionOptions:          connOpts,
			PrivilegeEscalationOptions: &ansiblePrivelegeOpts,
		}

		runCtx, lokiCtxCloseFn := context.WithCancel(context.Background())
		defer lokiCtxCloseFn()

		if err = lokiPlaybook.Run(runCtx); err != nil {
			return merry.Prepend(err, "failed to run loki playbook")
		}
	} else {
		llog.Infof("Loki deploy status: skipping")
	}

	if resp != nil {
		defer resp.Body.Close()
	}

	return nil
}

func (k *Kubernetes) deployPromtail(shellState *state.State) error { //nolint
	var (
		err   error
		bytes []byte
	)

	if bytes, err = kubeengine.GetPromtailValues(shellState); err != nil {
		return merry.Prepend(err, "failed to get promtail values")
	}

	if err = k.Engine.DeployChart(
		&kubeengine.InstallOptions{ //nolint
			ChartName:      path.Join(grafanaHelmRepoName, "promtail"),
			ChartNamespace: "default",
			ReleaseName:    "promtail",
			RepositoryURL:  grafanaHelmRepoURL,
			RepositoryName: grafanaHelmRepoName,
			ValuesYaml:     string(bytes),
		},
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to deploy promtail chart")
	}

	return nil
}

func (k *Kubernetes) deployNodeExporter(shellState *state.State) error {
	var err error

	if err = k.Engine.DeployChart(
		&kubeengine.InstallOptions{ //nolint
			ChartName:      path.Join(prometheusHelmRepoName, "prometheus-node-exporter"),
			ChartNamespace: "default",
			ReleaseName:    "node-exporter",
			RepositoryURL:  prometheusHelmRepoURL,
			RepositoryName: prometheusHelmRepoName,
		},
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to deploy node-exporter chart")
	}

	return nil
}

func (k *Kubernetes) deployPrometheus(shellState *state.State) error { //nolint
	var (
		err   error
		bytes []byte
	)

	if bytes, err = kubeengine.GetPrometheusValues(shellState); err != nil {
		return merry.Prepend(err, "failed to get prometheus values")
	}

	if err = k.Engine.DeployChart(
		&kubeengine.InstallOptions{ //nolint
			ChartName:      path.Join(prometheusHelmRepoName, "prometheus"),
			ChartNamespace: "default",
			ReleaseName:    "prometheus",
			RepositoryURL:  prometheusHelmRepoURL,
			RepositoryName: prometheusHelmRepoName,
			ValuesYaml:     string(bytes),
		},
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to deploy node-exporter chart")
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

		if cmd.ProcessState.ExitCode() != exitCode127 {
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

func (k *Kubernetes) deployIngress(shellState *state.State) error { //nolint
	var (
		err   error
		bytes []byte
	)

	if bytes, err = kubeengine.GetIngressValues(shellState); err != nil {
		return merry.Prepend(err, "failed to get ingress values")
	}

	if err = k.Engine.DeployChart(
		&kubeengine.InstallOptions{ //nolint
			ChartName:      path.Join(nginxHelmRepoName, "ingress-nginx"),
			ChartNamespace: "default",
			ReleaseName:    "ingress-nginx",
			RepositoryURL:  nginxHelmRepoURL,
			RepositoryName: nginxHelmRepoName,
			ValuesYaml:     string(bytes),
		},
		shellState,
	); err != nil {
		return merry.Prepend(err, "failed to deploy nginx ingress")
	}

	return nil
}
