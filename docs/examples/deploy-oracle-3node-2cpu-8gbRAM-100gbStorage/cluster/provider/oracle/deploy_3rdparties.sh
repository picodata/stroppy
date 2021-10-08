#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../../common.sh"

run "run deploy_kubernetes.sh file" sh deploy_kubernetes.sh

run "change dir to kubespray" cd kubespray

run "enable ingress nginx setting in k8s cluster addons.yml file" \
sudo sed -i \'s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g\' inventory/local/group_vars/k8s_cluster/addons.yml

run "disabling docker_dns_servers_strict k8s cluster option" \
echo 'docker_dns_servers_strict: no' \>\> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml

run "set kubernetes calico network plugin to flannel state" \
sed -i \'s/kube_network_plugin: calico/kube_network_plugin: flannel/g\' inventory/local/group_vars/k8s_cluster/k8s-cluster.yml

run "disabling logging for download_file action" \
sudo sed -i \'s/no_log: true/no_log: false/g\' roles/download/tasks/download_file.yml

#sudo sed -i \'s/download_force_cache: false/download_force_cache: true/g\' extra_playbooks/roles/download/defaults/main.yml

run "set download_run_once option in playbook" \
sudo sed -i \'s/download_run_once: false/download_run_once: true/g\' extra_playbooks/roles/download/defaults/main.yml

run "applying ansible playbook" \
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml

run "make $HOME/.kube directory" mkdir -p $HOME/.kube

run "copying kube config file to it's home directory" \
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config

run "setting kube config file access rights" \
sudo chown $(id -u):$(id -g) $HOME/.kube/config && chmod 600 $HOME/.kube/config


# local-storage
run "applying local-path-storage option" \
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml


# monitoring - kube-prometheus-stack without Grafana
run "creating monitoring kubernetes namespace" kubectl create namespace monitoring
run "adding prometheus-community repository" \
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

run "update repo data" helm repo update

run "installing grafana stack" \
helm install grafana-stack prometheus-community/kube-prometheus-stack \
                            --set grafana.enables=false \
                            --set prometheus.prometheusSpec.retention=180d \
                            --namespace monitoring \
                            --version 16.8.0


# grafana-on-premise
run "change directory back to '$HOME'" cd
run "installing cloudalchemy" ansible-galaxy install cloudalchemy.grafana

run "installing grafana community" ansible-galaxy collection install community.grafana

run "cd grafana-on-premise" cd grafana-on-premise

export prometheus_cluster_ip=$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')

run "configure grafana-on-premise prometheus_cluster_ip option" \
sed -i "'s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/$prometheus_cluster_ip:9090/g'" grafana-on-premise.yml

run "applying grafana on premise yaml" ansible-playbook grafana-on-premise.yml


#деплой секрета для успешного получения stroppy из приватной репы
run "register user as docker user" sudo usermod -aG docker "${USER}"

run "change current user group id to docker group id" newgrp docker

run "restarting docker service" sudo service docker restart

run "change docker.sock file owner" sudo chown ubuntu:docker /var/run/docker.sock
