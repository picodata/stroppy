### PREPARE LOCALHOST:

sudo apt-get install -y unzip terraform

# or manual:
# curl -O https://releases.hashicorp.com/terraform/0.14.7/terraform_0.14.7_linux_amd64.zip
# unzip terraform_0.14.7_linux_amd64.zip && rm terraform_0.14.7_linux_amd64.zip
# sudo install terraform /usr/bin/terraform
# terraform -install-autocomplete

cd stroppy-deploy
# generate id_rsa for ssh sessions
ssh-keygen -q -t rsa -N '' -f id_rsa <<<y 2>&1 >/dev/null

# download yandex-cloud/yandex provider
terraform init
# deploy yandex_compute_instance_group (DO NOT EXECUTE)
terraform apply -auto-approve

cat terraform.tfstate | grep ip_address
                "ip_address": "172.16.1.13",
                "nat_ip_address": "130.193.50.201",
                    "ip_address": "172.16.1.7",
                    "nat_ip_address": "130.193.49.226",
                    "ip_address": "172.16.1.30",
                    "nat_ip_address": "84.201.156.125",
                    "ip_address": "172.16.1.29",
                    "nat_ip_address": "130.193.49.45",

# copy id_rsa to master for managing other nodes from master
scp -i id_rsa -o StrictHostKeyChecking=no id_rsa ubuntu@130.193.50.201:/home/ubuntu/.ssh
ssh -i id_rsa ubuntu@130.193.50.201


### REMOTE (~20 min):
tee deploy_kubernetes.sh<<EOO
sudo apt-get update
sudo apt-get install -y sshpass python3-pip git
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
node1 ansible_host=172.16.1.13 ip=172.16.1.13 etcd_member_name=etcd1
node2 ansible_host=172.16.1.7 ip=172.16.1.7 etcd_member_name=etcd2
node3 ansible_host=172.16.1.30 ip=172.16.1.30 etcd_member_name=etcd3
node4 ansible_host=172.16.1.29 ip=172.16.1.29 etcd_member_name=etcd4

[kube-master]
node1

[etcd]
node1
node2
node3
node4

[kube-node]
node2
node3
node4

[k8s-cluster:children]
kube-master
kube-node
EOF

sudo sed -i "s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g" inventory/local/group_vars/k8s-cluster/addons.yml
sudo sed -i "s/local_volume_provisioner_enabled: false/local_volume_provisioner_enabled: true/g" inventory/local/group_vars/k8s-cluster/addons.yml
sudo sed -i "s/local_volume_provisioner_enabled: false/local_volume_provisioner_enabled: true/g" inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
# nano inventory/local/group_vars/k8s-cluster/addons.yml (!!!)
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml
mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
chmod 600 $HOME/.kube/config
# monitoring
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install grafana-stack prometheus-community/kube-prometheus-stack
kubectl apply -f metrics-server.yaml
kubectl apply -f ingress-grafana.yaml
EOO
chmod +x deploy_kubernetes.sh
./deploy_kubernetes.sh


### LOCAL:
# ssh tunnel for kubectl
ssh -i id_rsa -o StrictHostKeyChecking=no -o ServerAliveInterval=60 -N -L 6443:localhost:6443 -N ubuntu@130.193.50.201

### LOCAL2:
# copy kube config from master
scp -i id_rsa -o StrictHostKeyChecking=no ubuntu@130.193.50.201:/home/ubuntu/.kube/config .
sed -i 's/172.16.1.13/localhost/g' config
export KUBECONFIG=$(pwd)/config

# GRAFANA ACCESS ON localhost:8080 (admin/prom-operator)
kubectl port-forward deployment/grafana-stack 8080:3000

### DESTROY CLUSTER (LOCAL)
# terraform destroy -force
