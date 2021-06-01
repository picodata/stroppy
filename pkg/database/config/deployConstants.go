package config

const (
	Deployk8sFirstStepYandexCMD = `echo \
	"sudo apt-get update -y
	sudo apt-get install -y sshpass python3-pip git htop sysstat
	curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
	sudo apt-get install apt-transport-https --yes
	echo \"deb https://baltocdn.com/helm/stable/debian/ all main\" \
	| sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
	sudo apt-get update -y
	sudo apt-get install helm
	#add by @nik_sav
	sudo apt-get install dialog -y apt-utils -y
	echo 'debconf debconf/frontend select Noninteractive' | sudo debconf-set-selections
	#end add by @nik_sav
	sudo apt-get update
	git clone  https://github.com/kubernetes-sigs/kubespray.git
	cd kubespray
	sudo pip3 install -r requirements.txt
	rm inventory/local/hosts.ini" | tee deploy_kubernetes.sh
	`

	//nolint:lll
	Deployk8sThirdStepYandexCMD = `echo \
"sudo sed -i \'s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g\' \
inventory/local/group_vars/k8s-cluster/addons.yml
echo \"docker_dns_servers_strict: no\" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
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
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml
kubectl apply -f \
https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml
kubectl apply -f \
https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/v0.31.1/config/samples/deployment.yaml
echo "Waiting foundationdb deploy for 60 seconds..."
sleep 60
kubectl apply -f /home/ubuntu/cluster_with_client.yaml
kubectl apply -f /home/ubuntu/metrics-server.yaml
kubectl apply -f /home/ubuntu/ingress-grafana.yaml
kubectl apply -f \
https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml" \
 | tee -a deploy_kubernetes.sh
`

	INIfile = `echo \
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

	Deployk8sFirstStepOracleCMD = ` echo \
"echo 'IdentityFile /home/ubuntu/.ssh/private_key.pem' > ~/.ssh/config
sudo iptables --flush
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'
### /Oracle.Cloud
export DEBIAN_FRONTEND="noninteractive"
sudo apt-get -y update
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo 'deb https://baltocdn.com/helm/stable/debian/ all main' | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get -y update
sudo apt-get -y install helm
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
# nano inventory/local/group_vars/k8s_cluster/addons.yml (!!!)
sed -i 's/kube_network_plugin: calico/kube_network_plugin: flannel/g' inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
yes | sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config 
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
kubectl create namespace monitoring
helm repo add grafana https://grafana.github.io/helm-charts
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install loki grafana/loki-stack --namespace monitoring
helm install grafana-stack prometheus-community/kube-prometheus-stack --namespace monitoring
kubectl apply -f /home/ubuntu/metrics-server.yaml
kubectl apply -f /home/ubuntu/ingress-grafana.yaml " | tee -a deploy_kubernetes.sh
`
)
