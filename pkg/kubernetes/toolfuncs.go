/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"time"

	"github.com/ansel1/merry"
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
// craftClusterDeploymentScript - получить атрибуты для заполнения файла hosts.ini для использования при деплое k8s кластера.
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

	result, err := yaml.Marshal(inventory)
	llog.Tracef("Serialized monitoring inventory %v", string(result))
	return result, err
}

// Generate kubespray inventory based on hosts addresses list.
func (k *Kubernetes) generateK8sInventory() ([]byte, error) {
	var empty map[string]interface{}
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
		if k == "master" {
			continue
		}
		inventory.All.Hosts[k] = Host{v, v, v}
		hosts[k] = empty
	}
	inventory.All.Children["kube_node"] = map[string]interface{}{
		"hosts": hosts,
	}
	inventory.All.Children["etcd"] = map[string]interface{}{
		"hosts": hosts,
	}

	result, err := yaml.Marshal(inventory)
	llog.Tracef("Serialized kubespray inventory %v", string(result))
	return result, err
}

// Generate `default` inventory for any another actions related with ansible.
func (k *Kubernetes) generateSimpleInventory() ([]byte, error) {
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

	result, err := yaml.Marshal(inventory)
	llog.Tracef("Serialized generic inventory %v\n", string(result))
	return result, err
}

// Write ansible config from const.
func writeAnsibleConfig(wd string) error {
	var length int

	file, err := os.OpenFile(path.Join(wd, "ansible.cfg"), os.O_WRONLY, fs.FileMode(roAll))
	if err == nil {
		if length, err = file.WriteString(ConfigTemplate); err != nil {
			return merry.Prepend(err, "Error then creating ansible.cfg file")
		}
		llog.Tracef("%v bytes successfully written to %v", length, path.Join(wd, "ansible.cfg"))
		return merry.Prepend(err, "Error then writing ansible.cfg file")
	}
	if file, err = os.Create(path.Join(wd, "ansible.cfg")); err != nil {
		return merry.Prepend(err, "Error then creating ansible.cfg file")
	}
	if length, err = file.WriteString(ConfigTemplate); err != nil {
		return merry.Prepend(err, "Error then writing ansible.cfg file")
	}
	llog.Tracef("%v bytes successfully written to %v", length, path.Join(wd, "ansible.cfg"))
	return err
}

// Write ansible requirements file.
func writeAnsibleRequirements(wd string) (err error) {
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

	var result []byte
	result, err = yaml.Marshal(requirements)
	if err != nil {
		merry.Prepend(err, "Error then marshaling requirements to yaml format")
	}
	var f *os.File
	f, err = os.OpenFile(path.Join(wd, "requirements.yml"), os.O_WRONLY, fs.FileMode(roAll))
	if err == nil {
		var l int
		if l, err = f.Write(result); err != nil {
			return merry.Prepend(err, "Error then creating requirements.yml file")
		}
		llog.Tracef("%v bytes successfully written to %v", l, path.Join(wd, "requirements.yml"))
		return
	}
	if f, err = os.Create(path.Join(wd, "requirements.yml")); err != nil {
		return merry.Prepend(err, "Error then creating requirements.yml file")
	}
	var l int
	if l, err = f.Write(result); err != nil {
		return merry.Prepend(err, "Error then creating requirements.yml file")
	}
	llog.Tracef("%v bytes successfully written to %v", l, path.Join(wd, "requirements.yml"))

	return
}

// Install ansible galaxy requirements
func installGalaxyRoles() error {
	var err error
	var stdout []byte
	var cmd *exec.Cmd

	cmd = exec.Command("ansible-galaxy", "install", "-fr", "requirements.yml")
	stdout, err = cmd.Output()
	if err != nil {
		return merry.Prepend(err, "Error then installing ansible requirements")
	}
	llog.Tracef("roles installation stdout:\n%s", string(stdout))

	cmd = exec.Command("ansible-galaxy", "collection", "install", "-fr", "requirements.yml")
	stdout, err = cmd.Output()
	if err != nil {
		return merry.Prepend(err, "Error then installing ansible requirements")
	}
	llog.Tracef("collection installation stdout:\n%s", string(stdout))
	return err
}
