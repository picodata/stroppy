package kubernetes

const (
	clusterK8sPort = 6443

	// кол-во подов при успешном деплое k8s в master-ноде
	runningPodsCount = 27

	clusterMonitoringPort = 3000

	monitoringSshEntity = "monitoring"
)

const (
	deployK8sSecondStepTemplate = `echo \
"tee inventory/local/hosts.ini<<EOF
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
