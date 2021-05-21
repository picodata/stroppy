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
master   ansible_host=172.16.1.10 ip=172.16.1.10 etcd_member_name=etcd1
worker-1 ansible_host=172.16.1.11 ip=172.16.1.11 etcd_member_name=etcd2
worker-2 ansible_host=172.16.1.12 ip=172.16.1.12 etcd_member_name=etcd3
worker-3 ansible_host=172.16.1.13 ip=172.16.1.13 etcd_member_name=etcd4
worker-4 ansible_host=172.16.1.14 ip=172.16.1.14 etcd_member_name=etcd5
worker-5 ansible_host=172.16.1.15 ip=172.16.1.15 etcd_member_name=etcd6
worker-6 ansible_host=172.16.1.16 ip=172.16.1.16 etcd_member_name=etcd7
worker-7 ansible_host=172.16.1.17 ip=172.16.1.17 etcd_member_name=etcd8

[kube-master]
master

[etcd]
master
worker-1
worker-2
worker-3
worker-4
worker-5
worker-6
worker-7

[kube-node]
worker-1
worker-2
worker-3
worker-4
worker-5
worker-6
worker-7

[k8s-cluster:children]
kube-master
kube-node
EOF

sudo sed -i "s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g" inventory/local/group_vars/k8s-cluster/addons.yml
echo "docker_dns_servers_strict: no" >> inventory/local/group_vars/k8s-cluster/k8s-cluster.yml
