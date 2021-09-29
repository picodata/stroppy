package kubernetes

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/ansel1/merry"
	llog "github.com/sirupsen/logrus"
	"gitlab.com/picodata/stroppy/pkg/engine"
	kube_engine "gitlab.com/picodata/stroppy/pkg/engine/kubeengine"
	"gitlab.com/picodata/stroppy/pkg/engine/provider"
)

var errPortCheck = errors.New("port check failed")

/* loadFilesToMaster
 * скопировать на мастер-ноду private key для работы мастера с воркерами
 * и файлы для развертывания мониторинга и postgres */
func (k *Kubernetes) loadFilesToMaster() (err error) {
	masterExternalIP := k.Engine.AddressMap["external"]["master"]
	llog.Infoln(masterExternalIP)

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

	metricsServerFilePath := filepath.Join(k.Engine.WorkingDirectory, "monitoring", "metrics-server.yaml")
	if err = k.Engine.LoadFile(metricsServerFilePath, "/home/ubuntu/metrics-server.yaml"); err != nil {
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
			masterAddressString = fmt.Sprintf("master ansible_host=%v ip=%v etcd_member_name=etcd1 \n",
				internalAddressMap["master"], internalAddressMap["master"])
		} else {
			workersAddressString += fmt.Sprintf("worker-%v ansible_host=%v ip=%v etcd_member_name=etcd%v \n",
				i, internalAddressMap[k], internalAddressMap[k], i+1)
			workersString += fmt.Sprintf("worker-%v \n", i)
		}
	}

	instancesString := masterAddressString + workersAddressString

	deployK8sSecondStep = fmt.Sprintf(clusterHostsIniTemplate, instancesString, workersString, workersString)
	return
}
