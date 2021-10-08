/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package kubernetes

const (
	clusterK8sPort = 6443

	// кол-во подов при успешном деплое k8s в master-ноде
	runningPodsCount = 27

	clusterMonitoringPort = 3000

	monitoringSshEntity = "monitoring"
)

const (
	clusterHostsIniTemplate = `echo \
"tee kubespray/inventory/local/hosts.ini<<EOF
[all]
%v
	
[kube-master]
master
	
[etcd]
master
%v
	
[kube-node]
%v
	
[k8s-cluster:children]
kube-master
kube-node
EOF" | tee -a deploy_kubernetes.sh
`
)
