package kubernetes

const (
	clusterK8sPort        = 6443
	reserveClusterK8sPort = 6444

	// кол-во подов при успешном деплое k8s в master-ноде
	runningPodsCount = 27

	clusterMonitoringPort        = 3000
	reserveClusterMonitoringPort = 3001

	// задержка для случаев ожидания переповтора или соблюдения порядка запуска
	execTimeout = 5

	deployConfigStroppyFile = "stroppy-manifest.yaml"
)

// Externally avail constants
const (
	ResourcePodName          = "pods"
	ResourcePortForwarding   = "portforward"
	ResourceDefaultNamespace = "default"
)

const (
	deployK8sFirstStepYandexCMD = `echo \
"export DEBIAN_FRONTEND='noninteractive'
sudo apt-get update -y
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update -y
sudo apt-get install helm -y
git clone https://github.com/kubernetes-incubator/kubespray
cd kubespray
sudo pip3 install -r requirements.txt
rm inventory/local/hosts.ini
" | tee deploy_kubernetes.sh
`

	//nolint:lll
	deployK8sThirdStepYandexCMD = `echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' inventory/local/group_vars/k8s_cluster/addons.yml
echo 'docker_dns_servers_strict: no' >> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s_cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# local-storage
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
# monitoring: kube-prometheus-stack
kubectl create namespace monitoring
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update -y
helm install grafana-stack prometheus-community/kube-prometheus-stack --set grafana.enables=false --namespace monitoring
# monitoring: grafana-on-premise
cd 
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy collection install community.grafana
cd grafana-on-premise
prometheus_cluster_ip=\$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')
sed -i \"s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/\$prometheus_cluster_ip:9090/g\" grafana-on-premise.yml
ansible-playbook grafana-on-premise.yml
" \
| tee -a deploy_kubernetes.sh
`

	deployK8sSecondStepTemplate = `echo \
"tee inventory/local/hosts.ini<<EOF
[all]
master ansible_host=%v ip=%v etcd_member_name=etcd1
worker-1 ansible_host=%v ip=%v etcd_member_name=etcd2
worker-2 ansible_host=%v ip=%v etcd_member_name=etcd3
worker-3 ansible_host=%v ip=%v etcd_member_name=etcd4
	
[kube-master]
master
	
[etcd]
master
worker-1
worker-2
worker-3
	
[kube-node]
worker-1
worker-2
worker-3
	
[k8s-cluster:children]
kube-master
kube-node
EOF" | tee -a deploy_kubernetes.sh
`

	Deployk8sFirstStepOracleTemplate = ` echo \
"echo 'IdentityFile /home/ubuntu/.ssh/private_key.pem' > ~/.ssh/config
sudo iptables --flush
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
### /Oracle.Cloud
sudo apt-get update
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update
sudo apt-get install helm
git clone https://github.com/kubernetes-incubator/kubespray
cd kubespray
sudo pip3 install -r requirements.txt
rm inventory/local/hosts.ini

" | tee  deploy_kubernetes.sh
`
	//nolint:lll
	Deployk8sThirdStepOracleCMD = ` echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' inventory/local/group_vars/k8s_cluster/addons.yml
echo 'docker_dns_servers_strict: no' >> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
sed -i 's/kube_network_plugin: calico/kube_network_plugin: flannel/g' inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# local-storage
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
# monitoring - kube-prometheus-stack without Grafana
kubectl create namespace monitoring
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install grafana-stack prometheus-community/kube-prometheus-stack --set grafana.enables=false --namespace monitoring
# grafana-on-premise
cd
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy collection install community.grafana
cd grafana-on-premise
prometheus_cluster_ip=\$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')
sed -i \"s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/\$prometheus_cluster_ip:9090/g\" grafana-on-premise.yml
ansible-playbook grafana-on-premise.yml
" | tee -a deploy_kubernetes.sh
`
)
