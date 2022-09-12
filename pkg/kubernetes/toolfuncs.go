/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gitlab.com/picodata/stroppy/pkg/engine"
	kubeEngine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	"gitlab.com/picodata/stroppy/pkg/state"

	"github.com/ansel1/merry"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	llog "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v3"
)

const (
	ConfigTemplate string = `
[defaults]
roles_path =third_party/.roles/
collections_paths=third_party/.collections/
`
	SSHUser string = "ubuntu"
)

const (
	grafanaVer        string = "0.17.0"
	nodeExporterVer   string = "2.0.0"
	kubernetesColVer  string = "2.0.1"
	grafanaColVer     string = "1.4.0"
	forceReinstallReq bool   = false //nolint
)

const (
	roAll         int = 0o644
	etcdNormalCnt int = 3
)

type Inventory struct {
	All All `yaml:"all"`
}

type All struct {
	Vars     map[string]interface{} `yaml:"vars"`
	Hosts    map[string]interface{} `yaml:"hosts"`
	Children map[string]interface{} `yaml:"children"`
}

type Host struct {
	AnsibleHost string `yaml:"ansible_host"`
	AccessIp    string `yaml:"access_ip"`
	Ip          string `yaml:"ip"`
}

var errPortCheck = errors.New("port check failed")

// TODO: unused function!
// скопировать на мастер-ноду private key для работы мастера с воркерами
// и файлы для развертывания мониторинга и postgres.
func (k *Kubernetes) loadFilesToMaster(shellState *state.State) (err error) { //nolint
	masterExternalIP := shellState.InstanceAddresses.GetFirstMaster().External
	llog.Infof("Connecting to master %v", masterExternalIP)

	if shellState.Settings.DeploymentSettings.Provider == provider.Yandex {
		/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
		если кластер пока не готов*/
		llog.Infoln("Checking status of port 22 on the cluster's master...")

		var masterPortAvailable bool

		for i := 0; i <= kubeEngine.ConnectionRetryCount; i++ {
			masterPortAvailable = engine.IsRemotePortOpen(masterExternalIP, 22)
			if !masterPortAvailable {
				llog.Infof("status of check the master's port 22:%v. Repeat #%v", errPortCheck, i)
				time.Sleep(kubeEngine.ExecTimeout * time.Second)
			} else {
				break
			}
		}
		if !masterPortAvailable {
			return merry.Prepend(errPortCheck, "master's port 22 is not available")
		}
	}

	metricsServerFilePath := filepath.Join(
		shellState.Settings.WorkingDirectory,
		"monitoring",
		"metrics-server.yaml",
	)
	if err = k.Engine.LoadFile(
		metricsServerFilePath,
		"/home/ubuntu/metrics-server.yaml",
		shellState,
	); err != nil {
		return
	}
	llog.Infoln("copying metrics-server.yaml: success")

	ingressGrafanaFilePath := filepath.Join(
		shellState.Settings.WorkingDirectory,
		"monitoring",
		"ingress-grafana.yaml",
	)
	if err = k.Engine.LoadFile(
		ingressGrafanaFilePath,
		"/home/ubuntu/ingress-grafana.yaml",
		shellState,
	); err != nil {
		return
	}
	llog.Infoln("copying ingress-grafana.yaml: success")

	grafanaDirectoryPath := filepath.Join(
		shellState.Settings.WorkingDirectory,
		"monitoring",
		"grafana-on-premise",
	)
	if err = k.Engine.LoadDirectory(grafanaDirectoryPath, "/home/ubuntu", shellState); err != nil {
		return
	}
	llog.Infoln("copying grafana-on-premise: success")

	commonShFilePath := filepath.Join(shellState.Settings.WorkingDirectory, "common.sh")
	if err = k.Engine.LoadFile(
		commonShFilePath,
		"/home/ubuntu/common.sh",
		shellState,
	); err != nil {
		return
	}
	llog.Infoln("copying common.sh: success")

	clusterDeploymentDirectoryPath := filepath.Join(shellState.Settings.WorkingDirectory, "cluster")
	if err = k.Engine.LoadDirectory(
		clusterDeploymentDirectoryPath,
		"/home/ubuntu",
		shellState,
	); err != nil {
		return
	}
	llog.Infoln("cluster directory copied successfully")

	return
}

// Generate monitoring inventory based on hosts.
func (k *Kubernetes) GenerateMonitoringInventory(shellState *state.State) ([]byte, error) {
	var (
		bytes []byte
		err   error
	)

	hosts := make(map[string]map[string]string)

	for name, node := range shellState.InstanceAddresses.GetWorkersAndMastersAddrPairs() {
		hosts[name] = map[string]string{"ansible_host": node.Internal}
	}

	//  vars:
	//  prometheus_targets:
	//    node:
	//    - targets:
	//      - master:9100
	//      labels:
	//        env: localhost

	//  grafana_security:
	//    admin_user: admin
	//    admin_password: admin
	//
	//  grafana_address: 0.0.0.0
	//  grafana_port: 3000
	inventory := map[string]map[string]interface{}{
		"all": {
			"vars": map[string]interface{}{
				"cloud_type":               "yandex",
				"ansible_user":             SSHUser,
				"ansible_ssh_common_args":  "-F .ssh/config",
				"promtail_force_reinstall": false,
				"nodeexp_force_reinstall":  false,
				"grafana_force_reinstall":  false,
				"loki_force_reinstall":     false,
				"grafana_security": map[string]interface{}{
					"admin_user":     "admin",
					"admin_password": "admin",
				},
				"grafana_datasources": []interface{}{
					map[string]interface{}{
						"name":      "Prometheus",
						"type":      "prometheus",
						"access":    "proxy",
						"url":       "http://localhost:9090",
						"basicAuth": false,
					},
					map[string]interface{}{
						"name":      "Loki",
						"type":      "loki",
						"access":    "proxy",
						"url":       "http://localhost:3100",
						"basicAuth": false,
					},
				},
			},
			"hosts": hosts,
		},
	}

	if bytes, err = yaml.Marshal(inventory); err != nil {
		return nil, merry.Prepend(err, "Error then serializing inventory")
	}

	llog.Tracef("Serialized monitoring inventory %v", string(bytes))

	return bytes, nil
}

// Generate kubespray inventory based on hosts addresses list.
func (k *Kubernetes) generateK8sInventory(shellState *state.State) ([]byte, error) { //nolint
	var (
		empty             map[string]interface{}
		supplementaryAddr []string
		bytes             []byte
		err               error
	)

	childrenAll := make(map[string]interface{})
	childrenETCD := make(map[string]interface{})
	childrenControlPlane := make(map[string]interface{})
	hostsAll := make(map[string]interface{})

	for name, node := range shellState.InstanceAddresses.GetWorkersAndMastersAddrPairs() {
		hostsAll[name] = Host{
			AnsibleHost: node.Internal,
			AccessIp:    node.Internal,
			Ip:          node.Internal,
		}
		childrenAll[name] = map[string]interface{}{}

		switch {
		// if [master-1, worker-1, worker-2, worker-3] -> etcd[worker-1, worker-2, worker-3]
		case strings.Contains(name, "worker") &&
			len(shellState.NodesInfo.Params) >= etcdNormalCnt &&
			len(childrenETCD) < etcdNormalCnt:
			llog.Tracef("node %s added to etcd hosts", name)

			childrenETCD[name] = map[string]interface{}{}

			continue

		case strings.Contains(name, "master"):
			childrenControlPlane[name] = map[string]interface{}{}

			supplementaryAddr = append(supplementaryAddr, node.Internal, node.External)

			// if [master-1] -> etcd[master-1]
		case len(shellState.NodesInfo.Params) == 1|2 &&
			len(childrenETCD) == 0:
			childrenETCD[name] = map[string]interface{}{}

			continue
		// if [master-1, worker-1, worker-2] -> etcd[master-1, worker-1, worker-2]
		case len(shellState.NodesInfo.Params) >= etcdNormalCnt:
			childrenETCD[name] = map[string]interface{}{}

			continue
		}
	}

	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"kube_version":                        "v1.23.7",
				"ansible_user":                        SSHUser,
				"ansible_ssh_common_args":             "-F .ssh/config",
				"ignore_assert_errors":                "yes",
				"docker_dns_servers_strict":           "no",
				"download_force_cache":                false,
				"download_run_once":                   false,
				"supplementary_addresses_in_ssl_keys": supplementaryAddr,
				"addons": map[string]interface{}{
					"ingress_nginx_enabled": false,
				},
				"node_taints": []string{},
			},
			Hosts: hostsAll,
			Children: map[string]interface{}{
				"kube_control_plane": map[string]interface{}{
					"hosts": childrenControlPlane,
				},
				"k8s_cluster": map[string]interface{}{
					"children": map[string]interface{}{
						"kube_control_plane": empty,
						"kube_node":          empty,
					},
				},
				"calico_rr": map[string]interface{}{
					"hosts": map[string]string{},
				},
				"etcd": map[string]interface{}{
					"hosts": childrenETCD,
				},
				"kube_node": map[string]interface{}{
					"hosts": childrenAll,
				},
			},
		},
	}

	if bytes, err = yaml.Marshal(inventory); err != nil {
		return nil, merry.Prepend(err, "Error then serializing inventory")
	}

	llog.Tracef("Serialized kubespray inventory %v", string(bytes))

	return bytes, nil
}

// Generate `default` inventory for any another actions related with ansible.
func (k *Kubernetes) generateSimplifiedInventory(shellState *state.State) ([]byte, error) {
	var (
		bytes []byte
		err   error
	)

	hosts := make(map[string]map[string]string)

	for name, node := range shellState.InstanceAddresses.GetWorkersAndMastersAddrPairs() {
		hosts[name] = map[string]string{"ansible_host": node.Internal}
	}

	inventory := map[string]map[string]interface{}{
		"all": {
			"vars": map[string]interface{}{
				"cloud_type":              "yandex",
				"ansible_user":            SSHUser,
				"ansible_ssh_common_args": "-F .ssh/config",
				"kube_master_ext_ip":      shellState.InstanceAddresses.GetFirstMaster().External,
				"kube_apiserver_port":     6443, //nolint //TODO: End-to-end inventory configuration for kubespray #issue97
			},
			"hosts": hosts,
		},
	}

	if bytes, err = yaml.Marshal(inventory); err != nil {
		return nil, merry.Prepend(err, "Error then serializing inventory")
	}

	llog.Tracef("Serialized generic inventory %v\n", string(bytes))

	return bytes, nil
}

// Write ansible config from const.
func writeAnsibleConfig(workDir string) error {
	var (
		file   *os.File
		length int
		err    error
	)

	if file, err = os.Create(path.Join(workDir, "ansible.cfg")); err != nil {
		return merry.Prepend(err, "Error then creating ansible.cfg file")
	}

	if length, err = file.WriteString(ConfigTemplate); err != nil {
		return merry.Prepend(err, "Error then writing ansible.cfg file")
	}

	llog.Tracef("%v bytes successfully written to %v", length, path.Join(workDir, "ansible.cfg"))

	return nil
}

// Write ansible requirements file.
func writeAnsibleRequirements(workDir string) error {
	var (
		file   *os.File
		bytes  []byte
		length int
		err    error
	)

	requirements := map[string]interface{}{
		"roles": []map[string]string{
			{
				"name":    "cloudalchemy.grafana",
				"version": grafanaVer,
			},
			{
				"name":    "cloudalchemy.node_exporter",
				"version": nodeExporterVer,
			},
		},
		"collections": []map[string]string{
			{
				"name":    "community.kubernetes",
				"version": kubernetesColVer,
			},
			{
				"name":    "community.grafana",
				"version": grafanaColVer,
			},
		},
	}

	if bytes, err = yaml.Marshal(requirements); err != nil {
		merry.Prepend(err, "Error then marshaling requirements to yaml format")
	}

	if file, err = os.Create(path.Join(workDir, "requirements.yml")); err != nil {
		return merry.Prepend(err, "Error then creating file requirements.yml")
	}

	if length, err = file.Write(bytes); err != nil {
		return merry.Prepend(err, "Error then creating requirements.yml file")
	}

	llog.Tracef(
		"%v bytes successfully written to %v",
		length,
		path.Join(workDir, "requirements.yml"),
	)

	return nil
}

// Install ansible galaxy requirements.
func installGalaxyRoles() error {
	var (
		err    error
		stdout []byte
	)

	command := exec.Command("ansible-galaxy", "install", "-fr", "requirements.yml")
	if stdout, err = command.Output(); err != nil {
		return merry.Prepend(err, "Error then installing ansible requirements")
	}

	llog.Tracef("roles installation stdout:\n%s", string(stdout))

	command = exec.Command("ansible-galaxy", "collection", "install", "-fr", "requirements.yml")
	if stdout, err = command.Output(); err != nil {
		return merry.Prepend(err, "Error then installing ansible requirements")
	}

	llog.Tracef("collection installation stdout:\n%s", string(stdout))

	return nil
}

// Create request and return response.
func sendGetWithContext(ctx context.Context, body io.Reader, url string) (*http.Response, error) {
	var (
		req *http.Request
		res *http.Response
		err error
	)

	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, body); err != nil {
		return nil, merry.Prepend(err, "Error then constructing request")
	}

	if res, err = http.DefaultClient.Do(req); err != nil {
		return nil, merry.Prepend(err, fmt.Sprintf("Error then executing reqest to %s", url))
	}

	return res, nil
}

func createAnsibleOpts(
	inventoryPath string,
) (*options.AnsibleConnectionOptions, *playbook.AnsiblePlaybookOptions) {
	ansibleConnectionOpts := &options.AnsibleConnectionOptions{
		AskPass:       false,
		Connection:    "ssh",
		PrivateKey:    "",
		SCPExtraArgs:  "",
		SFTPExtraArgs: "",
		SSHCommonArgs: "",
		SSHExtraArgs:  "",
		Timeout:       0,
		User:          "",
	}

	ansiblePlaybookOptions := &playbook.AnsiblePlaybookOptions{
		AskVaultPassword:  false,
		Check:             false,
		Diff:              false,
		ExtraVars:         map[string]interface{}{},
		ExtraVarsFile:     []string{},
		FlushCache:        false,
		ForceHandlers:     false,
		Forks:             "",
		Inventory:         inventoryPath,
		Limit:             "",
		ListHosts:         false,
		ListTags:          false,
		ListTasks:         false,
		ModulePath:        "",
		SkipTags:          "",
		StartAtTask:       "",
		Step:              false,
		SyntaxCheck:       false,
		Tags:              "",
		VaultID:           "",
		VaultPasswordFile: "",
		Verbose:           false,
		Version:           false,
	}

	return ansibleConnectionOpts, ansiblePlaybookOptions
}
