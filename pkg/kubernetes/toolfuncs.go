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
	"github.com/sethvargo/go-password/password"
	llog "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	ConfigTemplate string = `
[defaults]
roles_path =third_party/.roles/
collections_paths=third_party/.collections/
`
	SSHUser string = "ubuntu"
)

// monitoring.
const (
	prometheusHostname string = "prometheus.cluster.picodata.io"
	grafanaVer         string = "0.17.0"
	grafanaColVer      string = "1.4.0"
	lokiPort           uint16 = 3100
)

// another consts.
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
	masterExternalIP := shellState.NodesInfo.IPs.FirstMasterIP.External
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
		bytes      []byte
		err        error
		grPassword string
	)

	if grPassword, err = password.Generate(20, 7, 0, false, true); err != nil { //nolint
		return nil, merry.Prepend(err, "failed to generate password")
	}

	shellState.Settings.DeploymentSettings.GrPassword = grPassword

	inventory := map[string]map[string]interface{}{
		"all": {
			"vars": map[string]interface{}{
				"cloud_type":              "yandex",
				"ansible_user":            SSHUser,
				"ansible_ssh_common_args": "-F .ssh/config",
				"grafana_auth": map[string]interface{}{
					"basic": map[string]interface{}{
						"enabled": true,
					},
				},
				"grafana_security": map[string]interface{}{
					"admin_user":     "stroppy",
					"admin_password": grPassword,
				},
				"grafana_address": shellState.NodesInfo.IPs.FirstMasterIP.Internal,
				"grafana_port":    grafanaPort,
				"grafana_datasources": []interface{}{
					map[string]interface{}{
						"name":   "Prometheus",
						"type":   "prometheus",
						"access": "proxy",
						"url": fmt.Sprintf( //nolint
							"http://%s:%d",
							prometheusHostname,
							shellState.Settings.DeploymentSettings.PromPort,
						),
						"basicAuth": false,
					},
					map[string]interface{}{
						"name":   "Loki",
						"type":   "loki",
						"access": "proxy",
						"url": fmt.Sprintf( //nolint
							"http://%s:%d",
							shellState.NodesInfo.IPs.FirstMasterIP.Internal,
							lokiPort,
						),
						"basicAuth": false,
					},
				},
			},
			"hosts": map[string]interface{}{
				"master": map[string]interface{}{
					"ansible_host": shellState.NodesInfo.IPs.FirstMasterIP.Internal,
				},
			},
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
		case strings.Contains(name, "master"):
			childrenControlPlane[name] = map[string]interface{}{}

			supplementaryAddr = append(supplementaryAddr, node.Internal, node.External)

			switch {
			case shellState.NodesInfo.WorkersCnt < 3 &&
				shellState.NodesInfo.WorkersCnt+shellState.NodesInfo.MastersCnt >= 3 &&
				len(childrenETCD) < 3:
				childrenETCD[name] = map[string]interface{}{}
			case shellState.NodesInfo.WorkersCnt == 0 &&
				shellState.NodesInfo.MastersCnt >= 3 &&
				len(childrenETCD) < 3:
				childrenETCD[name] = map[string]interface{}{}
			case shellState.NodesInfo.WorkersCnt == 0 &&
				shellState.NodesInfo.MastersCnt < 3 &&
				len(childrenETCD) == 0:
				childrenETCD[name] = map[string]interface{}{}
			}

		case strings.Contains(name, "worker"):
			switch {
			case shellState.NodesInfo.WorkersCnt >= 3 &&
				len(childrenETCD) < 3:
				childrenETCD[name] = map[string]interface{}{}
			case shellState.NodesInfo.MastersCnt+shellState.NodesInfo.WorkersCnt >= 3 &&
				len(childrenETCD) < 3:
				childrenETCD[name] = map[string]interface{}{}
			}
		}
	}

	llog.Debugf("%#v", shellState.NodesInfo.IPs)

	inventory := Inventory{
		All: All{
			Vars: map[string]interface{}{
				"kube_version":                        "v1.23.7",
				"kube_database_disk_device_name":      "virtio-database",
				"kube_mon_disk_device_name":           "virtio-system",
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
				"node_taints":           []string{},
				"control_plane_port":    6443, //nolint
				"control_plane_address": shellState.NodesInfo.IPs.FirstMasterIP.External,
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
		},
		"collections": []map[string]string{
			{
				"name":    "community.grafana",
				"version": grafanaColVer,
			},
		},
	}

	if bytes, err = yaml.Marshal(requirements); err != nil {
		return merry.Prepend(err, "Error then marshaling requirements to yaml format")
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
func getServerStatus(body io.Reader, url string) (*http.Response, error) {
	var (
		req *http.Request
		res *http.Response
		err error
	)

	reqContext, ctxCloseFn := context.WithTimeout(context.Background(), time.Second*3) //nolint
	defer ctxCloseFn()

	if req, err = http.NewRequestWithContext(reqContext, http.MethodGet, url, body); err != nil {
		return nil, merry.Prepend(err, "Error then constructing request")
	}

	if res, err = http.DefaultClient.Do(req); err != nil {
		return nil, merry.Prepend(err, fmt.Sprintf("Error then executing reqest to %s", url))
	}

	return res, nil
}

func createAnsibleOpts(
	inventoryPath string,
	shellState *state.State,
) (*options.AnsibleConnectionOptions, *playbook.AnsiblePlaybookOptions) {
	var verbose bool

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

	switch shellState.Settings.DeploymentSettings.AnsibleVerbosity {
	case "v", "vv", "vvv", "vvvv", "vvvvv", "vvvvvv":
		verbose = true
	default:
		verbose = false
	}

	ansiblePlaybookOptions := &playbook.AnsiblePlaybookOptions{ //nolint
		Inventory: inventoryPath,
		Verbose:   verbose,
	}

	return ansibleConnectionOpts, ansiblePlaybookOptions
}
