/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

import (
	"errors"
	"fmt"
	"io"
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
	"gopkg.in/yaml.v3"
)

const ConfigTemplate string = `
[defaults]
roles_path =third_party/.roles/
collections_paths=third_party/.collections/
host_key_checking = False

[ssh_connection]
ssh_args = -o ConnectTimeout=120 -o StrictHostKeyChecking=no
`

type Inventory struct {
	All     All     `yaml:"all"`
	Bastion Bastion `yaml:"bastion"`
}

type All struct {
	Vars     map[string]interface{} `yaml:"vars"`
	Hosts    map[string]interface{} `yaml:"hosts"`
	Children map[string]interface{} `yaml:"children"`
}

type Bastion struct {
	AnsibleHost    string `yaml:"ansible_host"`
	AnsibleSshPort string `yaml:"ansible_ssh_port"`
}

type Host struct {
	AnsibleHost string `yaml:"ansible_host"`
	AccessIp    string `yaml:"access_ip"`
	Ip          string `yaml:"ip"`
}

var errPortCheck = errors.New("port check failed")

/* loadFilesToMaster
 * скопировать на мастер-ноду private key для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
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

	ingressGrafanaFilePath := filepath.Join(k.Engine.WorkingDirectory, "monitoring", "ingress-grafana.yaml")
	if err = k.Engine.LoadFile(ingressGrafanaFilePath, "/home/ubuntu/ingress-grafana.yaml"); err != nil {
		return
	}
	llog.Infoln("copying ingress-grafana.yaml: success")

	grafanaDirectoryPath := filepath.Join(k.Engine.WorkingDirectory, "monitoring", "grafana-on-premise")
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
// craftClusterDeploymentScript - получить атрибуты для заполнения файла hosts.ini для использования при деплое k8s кластера
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

// Generate monitoring inventory based on hosts
func (k *Kubernetes) GenerateMonitoringInventory() (result []byte) {
	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"ansible_user":            "stroppy",
				"ansible_ssh_common_args": "-F inventory/yandex/.ssh/config",
			},
			Hosts: map[string]interface{}{
				"master": map[string]interface{}{
					"ansible_host": k.Engine.AddressMap["external"]["master"],
				},
			},
		},
	}

	result, err := yaml.Marshal(inventory)
	if err != nil {
		fmt.Println(err)
	}

	llog.Debugln(string(result))
	return
}

// Generate kubespray inventory based on hosts addresses list
func (k *Kubernetes) generateK8sInventory() (result []byte) {
	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"kube_version":              "v1.23.7",
				"ansible_user":              "stroppy",
				"ansible_ssh_common_args":   "-F inventory/yandex/.ssh/config",
				"ignore_assert_errors":      "yes",
				"docker_dns_servers_strict": "no",
				"download_force_cache":      false,
				"download_run_once":         false,
				"addons":                    map[string]interface{}{"ingress_nginx_enabled": false},
			},
			Hosts: make(map[string]interface{}),
			Children: map[string]interface{}{
				"kube_control_plane": map[string]interface{}{
					"hosts": map[string]interface{}{
						"master": nil,
					},
				},
				"k8s_kluster": map[string]interface{}{
					"children": map[string]interface{}{
						"kube_control_plane": nil,
						"kube_node":          nil,
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
		hosts[k] = nil
	}
	inventory.All.Children["kube_node"] = hosts
	inventory.All.Children["etcd"] = hosts
	inventory.Bastion = Bastion{k.Engine.AddressMap["external"]["master"], "22"}

	result, err := yaml.Marshal(inventory)
	if err != nil {
		fmt.Println(err)
	}

	llog.Debugf("%v\n", string(result))
	return
}

// Generate `default` inventory for any another actions related with ansible
func (k *Kubernetes) generateSimpleInventory() (result []byte) {
	hosts := make(map[string]interface{})
	for k, v := range k.Engine.AddressMap["internal"] {
		hosts[k] = v
	}
	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"cloud_type":              "yandex",
				"ansible_user":            "ubuntu",
				"ansible_ssh_common_args": "-F .ssh/config",
			},
			Hosts: hosts,
		},
	}

	result, err := yaml.Marshal(inventory)
	if err != nil {
		fmt.Println(err)
	}

	llog.Debugf("%v\n", string(result))
	return
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src string, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}

/// Write andible config from const
/// TODO replace const to template
func writeAnsibleConfig(wd string) (err error) {
	var f *os.File
	f, err = os.Create(path.Join(wd, "ansible.cfg"))
	if err != nil {
		merry.Prepend(err, "Error then creating ansible.cfg config file")
	}
	var l int
	if l, err = f.WriteString(ConfigTemplate); err != nil {
		merry.Prepend(err, "Error then writing ansible config")
	} else {
		llog.Tracef("%v bytes successfully writed", l)
	}
	return
}

/// Write andible requrements file
/// TODO replace const to template
func writeAnsibleRequirements(wd string) (err error) {
	var f *os.File
	f, err = os.Create(path.Join(wd, "requrements.yml"))
	if err != nil {
		merry.Prepend(err, "Error then creating requrements.yml file")
	}

	requirements := map[string]interface{}{
		"roles": []map[string]string{
			{
				"name":    "cloudalchemy.grafana",
				"version": "0.17.0",
			},
			{
				"name":    "cloudalchemy.node_exporter",
				"version": "2.0.0",
			},
		},
		"collections": []map[string]string{
			{
				"name":    "community.kubernetes",
				"version": "2.0.1",
			},
		},
	}

	var result []byte
	result, err = yaml.Marshal(requirements)
	if err != nil {
		merry.Prepend(err, "Error then marshaling requirements to yaml format")
	}
	var l int
	if l, err = f.Write(result); err != nil {
		return merry.Prepend(err, "Error then writing requirements.yml")
	}
	llog.Tracef("%v bytes successfully writed to requirements.yml\n", l)

	return
}

/// Install ansible galaxy reqiurements
func installGalaxyRoles() (err error) {
    cmd := exec.Command("ansible-galaxy", "install", "-fr", "requrements.yml")
    var stdout []byte 
    stdout, err = cmd.Output()
    if err != nil {
        return merry.Prepend(err, "Error then installing ansible requrements")
    }
    llog.Debug(string(stdout))
    return
}
