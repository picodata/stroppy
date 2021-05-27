package kubernetes

const (
	clusterK8sPort        = 6443
	reserveClusterK8sPort = 6444

	// кол-во подов при успешном деплое k8s в master-ноде
	runningPodsCount = 41

	clusterMonitoringPort        = 8080
	reserveClusterMonitoringPort = 8081
)

const deployk8sFirstStepCmd = `echo \
"sudo apt-get update
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update
sudo apt-get install helm
#add by @nik_sav
sudo apt-get install dialog apt-utils
echo 'sudo debconf debconf/frontend select Noninteractive' | debconf-set-selections
#end add by @nik_sav
sudo apt-get update
git clone --branch v2.15.0 https://github.com/kubernetes-sigs/kubespray.git
cd kubespray
sudo pip3 install -r requirements.txt
rm inventory/local/hosts.ini" | tee deploy_kubernetes.sh
`

//nolint:lll
const deployk8sThirdStepCmd = `echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' \
inventory/local/group_vars/k8s-cluster/addons.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s-cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
kubectl create namespace monitoring
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install loki grafana/loki-stack --namespace monitoring
helm install grafana-stack prometheus-community/kube-prometheus-stack --namespace monitoring
kubectl apply -f /home/ubuntu/metrics-server.yaml
kubectl apply -f /home/ubuntu/ingress-grafana.yaml
kubectl apply -f \
https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml" \
 | tee -a deploy_kubernetes.sh
`

const deployk8sSecondStepTemplate = `echo \
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
