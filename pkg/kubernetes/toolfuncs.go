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
	"sort"
	"time"

	"github.com/ansel1/merry"
	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	kube_engine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
	yaml "gopkg.in/yaml.v3"
)

const (
	ConfigTemplate string = `
[defaults]
roles_path =third_party/.roles/
collections_paths=third_party/.collections/
`
	SSHUser           string = "ubuntu"
	grafanaVer        string = "0.17.0"
	nodeExporterVer   string = "2.0.0"
	kubernetesColVer  string = "2.0.1"
	grafanaColVer     string = "1.4.0"
	forceReinstallReq bool   = false //nolint
	roAll             int    = 0o644
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

// скопировать на мастер-ноду private key для работы мастера с воркерами
// и файлы для развертывания мониторинга и postgres.
func (k *Kubernetes) loadFilesToMaster() (err error) {
	masterExternalIP := k.Engine.AddressMap["external"]["master"]
	llog.Infof("Connecting to master %v", masterExternalIP)

	if k.provider.Name() == provider.Yandex {
		/* проверяем доступность порта 22 мастер-ноды, чтобы не столкнуться с ошибкой копирования ключа,
		если кластер пока не готов*/
		llog.Infoln("Checking status of port 22 on the cluster's master...")
		var masterPortAvailable bool
		for i := 0; i <= kube_engine.ConnectionRetryCount; i++ {
			masterPortAvailable = engine.IsRemotePortOpen(masterExternalIP, 22)
			if !masterPortAvailable {
				llog.Infof("status of check the master's port 22:%v. Repeat #%v", errPortCheck, i)
				time.Sleep(kube_engine.ExecTimeout * time.Second)
			} else {
				break
			}
		}
		if !masterPortAvailable {
			return merry.Prepend(errPortCheck, "master's port 22 is not available")
		}
	}

	metricsServerFilePath := filepath.Join(
		k.Engine.WorkingDirectory,
		"monitoring",
		"metrics-server.yaml",
	)
	if err = k.Engine.LoadFile(
		metricsServerFilePath,
		"/home/ubuntu/metrics-server.yaml",
	); err != nil {
		return
	}
	llog.Infoln("copying metrics-server.yaml: success")

	ingressGrafanaFilePath := filepath.Join(
		k.Engine.WorkingDirectory,
		"monitoring",
		"ingress-grafana.yaml",
	)
	if err = k.Engine.LoadFile(
		ingressGrafanaFilePath,
		"/home/ubuntu/ingress-grafana.yaml",
	); err != nil {
		return
	}
	llog.Infoln("copying ingress-grafana.yaml: success")

	grafanaDirectoryPath := filepath.Join(
		k.Engine.WorkingDirectory,
		"monitoring",
		"grafana-on-premise",
	)
	if err = k.Engine.LoadDirectory(grafanaDirectoryPath, "/home/ubuntu"); err != nil {
		return
	}
	llog.Infoln("copying grafana-on-premise: success")

	commonShFilePath := filepath.Join(k.Engine.WorkingDirectory, "common.sh")
	if err = k.Engine.LoadFile(commonShFilePath, "/home/ubuntu/common.sh"); err != nil {
		return
	}
	llog.Infoln("copying common.sh: success")

	clusterDeploymentDirectoryPath := filepath.Join(k.Engine.WorkingDirectory, "cluster")
	if err = k.Engine.LoadDirectory(clusterDeploymentDirectoryPath, "/home/ubuntu"); err != nil {
		return
	}
	llog.Infoln("cluster directory copied successfully")

	return
}

// "Deprecated: deployment script replaced to ansible inventory, and go-ansible wrapper"
// craftClusterDeploymentScript - получить атрибуты для заполнения файла hosts.ini
// для использования при деплое k8s кластера.
func (k *Kubernetes) craftClusterDeploymentScript() (deployK8sSecondStep string) {
	var workersAddressString string
	var masterAddressString string
	var workersString string

	internalAddressMap := k.Engine.AddressMap["internal"]

	var keys []string
	for k := range k.Engine.AddressMap["internal"] {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for i, k := range keys {
		if i == 0 {
			masterAddressString = fmt.Sprintf(
				"master ansible_host=%v ip=%v etcd_member_name=etcd1 \n",
				internalAddressMap["master"],
				internalAddressMap["master"],
			)
		} else {
			workersAddressString += fmt.Sprintf(
				"worker-%v ansible_host=%v ip=%v etcd_member_name=etcd%v \n", i,
				internalAddressMap[k],
				internalAddressMap[k], i+1,
			)
			workersString += fmt.Sprintf("worker-%v \n", i)
		}
	}

	instancesString := masterAddressString + workersAddressString
	llog.Debugln(instancesString)

	deployK8sSecondStep = fmt.Sprintf(
		clusterHostsIniTemplate,
		instancesString,
		workersString,
		workersString,
	)
	return
}

// Generate monitoring inventory based on hosts.
func (k *Kubernetes) GenerateMonitoringInventory() ([]byte, error) {
	var (
		bytes []byte
		err   error
	)

	hosts := make(map[string]map[string]string)

	for k, v := range k.Engine.AddressMap["internal"] {
		hosts[k] = map[string]string{"ansible_host": v}
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
func (k *Kubernetes) generateK8sInventory() ([]byte, error) {
	var (
		empty map[string]interface{}
		bytes []byte
		err   error
	)

	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"kube_version":              "v1.23.7",
				"ansible_user":              SSHUser,
				"ansible_ssh_common_args":   "-F .ssh/config",
				"ignore_assert_errors":      "yes",
				"docker_dns_servers_strict": "no",
				"download_force_cache":      false,
				"download_run_once":         false,
				"supplementary_addresses_in_ssl_keys": []string{
					k.Engine.AddressMap["internal"]["master"],
					k.Engine.AddressMap["external"]["master"],
				},
				"addons": map[string]interface{}{"ingress_nginx_enabled": false},
                "node_taints": []string{},
			},
			Hosts: make(map[string]interface{}),
			Children: map[string]interface{}{
				"kube_control_plane": map[string]interface{}{
					"hosts": map[string]interface{}{
						"master": empty,
					},
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
			},
		},
	}

	inventory.All.Hosts["master"] = Host{
		k.Engine.AddressMap["internal"]["master"],
		k.Engine.AddressMap["internal"]["master"],
		k.Engine.AddressMap["internal"]["master"],
	}

	hosts := make(map[string]interface{})

	for k, v := range k.Engine.AddressMap["internal"] {
		inventory.All.Hosts[k] = Host{v, v, v}
		hosts[k] = empty
	}

	inventory.All.Children["kube_node"] = map[string]interface{}{
		"hosts": hosts,
	}

	inventory.All.Children["etcd"] = map[string]interface{}{
		"hosts": hosts,
	}

	if bytes, err = yaml.Marshal(inventory); err != nil {
		return nil, merry.Prepend(err, "Error then serializing inventory")
	}

	llog.Tracef("Serialized kubespray inventory %v", string(bytes))

	return bytes, nil
}

// Generate `default` inventory for any another actions related with ansible.
func (k *Kubernetes) generateSimplifiedInventory() ([]byte, error) {
	var (
		bytes []byte
		err   error
	)

	hosts := make(map[string]map[string]string)

	for k, v := range k.Engine.AddressMap["internal"] {
		hosts[k] = map[string]string{"ansible_host": v}
	}

	inventory := map[string]map[string]interface{}{
		"all": {
			"vars": map[string]interface{}{
				"cloud_type":              "yandex",
				"ansible_user":            SSHUser,
				"ansible_ssh_common_args": "-F .ssh/config",
				"kube_master_ext_ip":      k.Engine.AddressMap["external"]["master"],
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
