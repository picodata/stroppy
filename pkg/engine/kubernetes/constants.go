package kubernetes

import "time"

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
	secretStroppyFile       = "stroppy-secret.yaml"

	kubernetesSshEntity = "kubernetes"
	monitoringSshEntity = "monitoring"

	stroppyPodName      = "stroppy-client"
	stroppyFieldManager = "stroppy-deploy"
	kubeConfigLocate    = ".kube/config"
)

// Externally avail constants
const (
	ResourcePodName           = "pods"
	ResourceService           = "svc"
	ResourceDefaultNamespace  = "default"
	SubresourcePortForwarding = "portforward"
	SubresourceExec           = "exec"
	PodWaitingWaitCreation    = true
	PodWaitingNotWaitCreation = false

	PodWaitingTime10Minutes = 2 * time.Minute
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

	deployK8sThirdStepYandexCMD = `echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' inventory/local/group_vars/k8s_cluster/addons.yml
echo 'docker_dns_servers_strict: no' >> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s_cluster/addons.yml (!!!)
sudo sed -i 's/no_log: true/no_log: false/g' kubespray/roles/download/tasks/download_file.yml
sudo sed -i 's/download_force_cache: false/download_force_cache: true/g' extra_playbooks/roles/download/defaults/main.yml
sudo sed -i 's/download_run_once: false/download_run_once: true/g' extra_playbooks/roles/download/defaults/main.yml
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp  /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# local-storage
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
# monitoring: kube-prometheus-stack
kubectl create namespace monitoring
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update -y
helm install grafana-stack prometheus-community/kube-prometheus-stack \
                            --set grafana.enables=false \
                            --set prometheus.prometheusSpec.retention=180d \
                            --namespace monitoring \
                            --version 16.8.0
# monitoring: grafana-on-premise
cd 
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy collection install community.grafana
cd grafana-on-premise
prometheus_cluster_ip=\$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')
sed -i \"s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/\$prometheus_cluster_ip:9090/g\" grafana-on-premise.yml
ansible-playbook grafana-on-premise.yml
#деплой секрета для успешного получения stroppy из приватной репы
sudo usermod -aG docker \"\${USER}\"
echo 'Added user in docker group'
newgrp docker
echo 'registered user in docker group'
sudo service docker restart
echo 'Restarted docker service'
docker login -u gitlab+deploy-token-489111 -p bzbGz3jwf1JsTrxvzN7x registry.gitlab.com
echo 'Logged in repository'
sudo chown ubuntu:docker /var/run/docker.sock
echo "change owner for /var/run/docker.sock"
" \
| tee -a deploy_kubernetes.sh
`

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

	deployK8sFirstStepOracleTemplate = ` echo \
"echo 'IdentityFile /home/ubuntu/.ssh/private_key.pem' > ~/.ssh/config
sudo iptables --flush
%v
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

	deployK8sThirdStepOracleCMD = ` echo \
"sudo sed -i 's/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g' inventory/local/group_vars/k8s_cluster/addons.yml
echo 'docker_dns_servers_strict: no' >> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
sed -i 's/kube_network_plugin: calico/kube_network_plugin: flannel/g' inventory/local/group_vars/k8s_cluster/k8s-cluster.yml
sudo sed -i 's/no_log: true/no_log: false/g' roles/download/tasks/download_file.yml
#sudo sed -i 's/download_force_cache: false/download_force_cache: true/g' extra_playbooks/roles/download/defaults/main.yml
sudo sed -i 's/download_run_once: false/download_run_once: true/g' extra_playbooks/roles/download/defaults/main.yml
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
helm install grafana-stack prometheus-community/kube-prometheus-stack \
                            --set grafana.enables=false \
                            --set prometheus.prometheusSpec.retention=180d \
                            --namespace monitoring \
                            --version 16.8.0
# grafana-on-premise
cd
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy collection install community.grafana
cd grafana-on-premise
prometheus_cluster_ip=\$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')
sed -i \"s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/\$prometheus_cluster_ip:9090/g\" grafana-on-premise.yml
ansible-playbook grafana-on-premise.yml
#деплой секрета для успешного получения stroppy из приватной репы
/bin/bash -c \"sudo usermod -aG docker "${USER}"\"
echo 'Added user in docker group'
/bin/bash -c \"newgrp docker\"
echo 'registered user in docker group'
/bin/bash -c \"sudo service docker restart\"
echo 'Restarted docker service'
sudo chown ubuntu:docker /var/run/docker.sock
echo 'change owner for /var/run/docker.sock'
" | tee -a deploy_kubernetes.sh
`
)

const (
	dockerRepLoginCmd = "docker login -u stroppy_deploy -p k3xG2_xe_SDjyYDREML3 registry.gitlab.com"
)
