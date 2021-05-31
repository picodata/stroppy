### PREPARE LOCALHOST:

sudo apt-get install -y unzip terraform

# or manual:
# curl -O https://releases.hashicorp.com/terraform/0.14.7/terraform_0.14.7_linux_amd64.zip
# unzip terraform_0.14.7_linux_amd64.zip && rm terraform_0.14.7_linux_amd64.zip
# sudo install terraform /usr/bin/terraform
# terraform -install-autocomplete

########################################################################################################################
# YANDEX CLOUD
cd stroppy-deploy-yc
# generate id_rsa for ssh sessions
ssh-keygen -q -t rsa -N '' -f id_rsa <<<y 2>&1 >/dev/null

# download yandex-cloud/yandex provider
terraform init
terraform apply -auto-approve

cat terraform.tfstate | grep ip_address
                "ip_address": "172.16.1.10",
                "nat_ip_address": "130.193.39.175",
                    "ip_address": "172.16.1.6",
                    "nat_ip_address": "84.201.173.126",
                    "ip_address": "172.16.1.23",
                    "nat_ip_address": "130.193.48.181",
                    "ip_address": "172.16.1.7",
                    "nat_ip_address": "130.193.38.140",

# copy id_rsa to master for managing other nodes from master
scp -i id_rsa -o StrictHostKeyChecking=no id_rsa ubuntu@130.193.39.175:/home/ubuntu/.ssh
scp -i id_rsa -o StrictHostKeyChecking=no ../metrics-server.yaml ubuntu@130.193.39.175:/home/ubuntu/metrics-server.yaml
scp -i id_rsa -o StrictHostKeyChecking=no ../ingress-grafana.yaml ubuntu@130.193.39.175:/home/ubuntu/ingress-grafana.yaml
scp -i id_rsa -o StrictHostKeyChecking=no ../postgres-manifest.yaml ubuntu@130.193.39.175:/home/ubuntu/postgres-manifest.yaml
scp -i id_rsa -o StrictHostKeyChecking=no -R grafana-on-premise ubuntu@130.193.39.175:/home/ubuntu/
ssh -i id_rsa ubuntu@130.193.39.175


### REMOTE (~20 min):
tee deploy_kubernetes.sh<<EOO
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

tee inventory/local/hosts.ini<<EOF
[all]
master ansible_host=172.16.1.10 ip=172.16.1.10 etcd_member_name=etcd1
worker-1 ansible_host=172.16.1.6 ip=172.16.1.6 etcd_member_name=etcd2
worker-2 ansible_host=172.16.1.23 ip=172.16.1.23 etcd_member_name=etcd3
worker-3 ansible_host=172.16.1.7 ip=172.16.1.7 etcd_member_name=etcd4

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
EOF

sudo sed -i "s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g" inventory/local/group_vars/k8s-cluster/addons.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s-cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
kubectl create namespace monitoring
# helm repo add grafana https://grafana.github.io/helm-charts
# helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
# helm repo update
# helm install loki grafana/loki-stack --namespace monitoring
# helm install grafana-stack prometheus-community/kube-prometheus-stack --namespace monitoring
kubectl apply -f /home/ubuntu/metrics-server.yaml
# kubectl apply -f /home/ubuntu/ingress-grafana.yaml
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
# grafana-on-premise
ansible-galaxy install cloudalchemy.prometheus
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy install cloudalchemy.node_exporter
ansible-galaxy collection install community.grafana
cd ../grafana-on-premise
ansible-playbook grafana-on-premise.yml
ansible-playbook node_exporter.yml
echo '  - worker-1:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
echo '  - worker-2:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
echo '  - worker-3:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
EOO
chmod +x deploy_kubernetes.sh
./deploy_kubernetes.sh


### LOCAL:
# ssh tunnel for kubectl
ssh -i id_rsa -o StrictHostKeyChecking=no -o ServerAliveInterval=60 -N -L 6443:localhost:6443 -N ubuntu@130.193.39.175

### LOCAL2:
# copy kube config from master
scp -i id_rsa -o StrictHostKeyChecking=no ubuntu@130.193.39.175:/home/ubuntu/.kube/config .
sed -i 's/172.16.1.10/localhost/g' config
export KUBECONFIG=$(pwd)/config

# GRAFANA ACCESS ON localhost:8080 (admin/admin)
ssh -i id_rsa -o StrictHostKeyChecking=no -o ServerAliveInterval=60 -L 3000:localhost:3000 ubuntu@130.193.39.175

### DESTROY CLUSTER (LOCAL)
# terraform destroy -force

########################################################################################################################
# ORACLE CLOUD
cd stroppy-deploy-oc

# download oracle.cloud provider
terraform init
terraform apply -auto-approve

# copy picodata.pem to master for managing other nodes from master

scp -i picodata.pem -o StrictHostKeyChecking=no picodata.pem ubuntu@130.61.16.109:/home/ubuntu/.ssh
scp -i picodata.pem -o StrictHostKeyChecking=no ../metrics-server.yaml ubuntu@130.61.16.109:/home/ubuntu/metrics-server.yaml
# scp -i picodata.pem -o StrictHostKeyChecking=no ../ingress-grafana.yaml ubuntu@130.61.16.109:/home/ubuntu/ingress-grafana.yaml
scp -i picodata.pem -o StrictHostKeyChecking=no ../postgres-manifest.yaml ubuntu@130.61.16.109:/home/ubuntu/postgres-manifest.yaml
scp -i id_rsa -o StrictHostKeyChecking=no -R grafana-on-premise ubuntu@130.193.39.175:/home/ubuntu/
ssh -i picodata.pem -o ServerAliveInterval=60 ubuntu@130.61.16.109

### REMOTE (~20 min):

### Oracle.Cloud
tee deploy_kubernetes.sh<<EOO
echo 'IdentityFile /home/ubuntu/.ssh/picodata.pem' > ~/.ssh/config
sudo iptables --flush
ssh 10.1.20.3 -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh 10.1.20.25 -o StrictHostKeyChecking=no 'sudo iptables --flush'
ssh 10.1.20.12 -o StrictHostKeyChecking=no 'sudo iptables --flush'
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

tee inventory/local/hosts.ini<<EOF
[all]
master ansible_host=10.1.20.156 ip=10.1.20.156 etcd_member_name=etcd1
worker-1 ansible_host=10.1.20.3 ip=10.1.20.3 etcd_member_name=etcd2
worker-2 ansible_host=10.1.20.25 ip=10.1.20.25 etcd_member_name=etcd3
worker-3 ansible_host=10.1.20.12 ip=10.1.20.12 etcd_member_name=etcd4

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
EOF

sudo sed -i "s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g" inventory/local/group_vars/k8s-cluster/addons.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s-cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
#kubectl create namespace monitoring
# helm repo add grafana https://grafana.github.io/helm-charts
# helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
# helm repo update
# helm install loki grafana/loki-stack --namespace monitoring
# helm install grafana-stack prometheus-community/kube-prometheus-stack --namespace monitoring
kubectl apply -f /home/ubuntu/metrics-server.yaml
# kubectl apply -f /home/ubuntu/ingress-grafana.yaml
# grafana-on-premise
ansible-galaxy install cloudalchemy.prometheus
ansible-galaxy install cloudalchemy.grafana
ansible-galaxy install cloudalchemy.node_exporter
ansible-galaxy collection install community.grafana
cd ../grafana-on-premise
ansible-playbook grafana-on-premise.yml
ansible-playbook node_exporter.yml
echo '  - worker-1:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
echo '  - worker-2:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
echo '  - worker-3:9100' | sudo tee -a /etc/prometheus/file_sd/node.yml
EOO
chmod +x deploy_kubernetes.sh
./deploy_kubernetes.sh

### LOCAL:
# ssh tunnel for kubectl
ssh -i picodata.pem -o StrictHostKeyChecking=no -o ServerAliveInterval=60 -N -L 6443:localhost:6443 -N ubuntu@130.61.16.109

### LOCAL2:
# copy kube config from master
scp -i picodata.pem -o StrictHostKeyChecking=no ubuntu@130.61.16.109:/home/ubuntu/.kube/config .
sed -i 's/10.1.20.156/localhost/g' config
export KUBECONFIG=$(pwd)/config

# GRAFANA ACCESS ON localhost:8080 (admin/admin)
ssh -i picodata.pem -o StrictHostKeyChecking=no -o ServerAliveInterval=60 -L 3000:localhost:3000 ubuntu@130.61.229.200

### DESTROY CLUSTER (LOCAL)
# terraform destroy -force
