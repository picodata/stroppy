#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../../common.sh"

run "run deploy_kubernetes.sh file" sh deploy_kubernetes.sh

run "change dir to kubespray" cd kubespray
run "checkout kubespray version 2.17.1" git checkout -b v2.17.1 tags/v2.17.1

run "enabling ingress_nginx_enabled kluster option" \
sudo sed -i \'s/ingress_nginx_enabled: false/ingress_nginx_enabled: true/g\' inventory/local/group_vars/k8s_cluster/addons.yml

run "disabling docker strict dns option" \
echo 'docker_dns_servers_strict: no' \>\> inventory/local/group_vars/k8s_cluster/k8s-cluster.yml

# nano inventory/local/group_vars/k8s_cluster/addons.yml (!!!)
run "disabling no_log download option" \
sudo sed -i \'s/no_log: true/no_log: false/g\' roles/download/tasks/download_file.yml

run "enable download_force_cache option" \
sudo sed -i \'s/download_force_cache: false/download_force_cache: true/g\' extra_playbooks/roles/download/defaults/main.yml

run "enable download_run_once option" \
sudo sed -i \'s/download_run_once: false/download_run_once: true/g\' extra_playbooks/roles/download/defaults/main.yml

run "run ansible-playbook" \
ansible-playbook -b -e ignore_assert_errors=yes -i inventory/local/hosts.ini cluster.yml

run "create kube config directory" mkdir -p $HOME/.kube

run "copying kube config file" \
sudo cp /etc/kubernetes/admin.conf $HOME/.kube/config

run "set right owner for kube config file" sudo chown \$\(id -u\):\$\(id -g\) $HOME/.kube/config

run "setting up right access rights for kube config file" chmod 600 $HOME/.kube/config

# local-storage
run "applying local-path-storage" \
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml

run "set default for local-path-provisioner" \
kubectl patch storageclass local-path -p '{"metadata": {"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'

# monitoring: kube-prometheus-stack
run "creating monitoring namespace" kubectl create namespace monitoring
run "adding prometheus-community repository to system" \
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts

run "preparing prometheus-community repo" helm repo update 

run "installing grafana stack" \
helm install grafana-stack prometheus-community/kube-prometheus-stack \
                            --set grafana.enables=false \
                            --set prometheus.prometheusSpec.retention=180d \
                            --namespace monitoring # \
                            # --version 16.8.0


# monitoring: grafana-on-premise
run "change directory to home" cd

run "installing grafana cloud alchemy" ansible-galaxy install cloudalchemy.grafana
run "perf additional installation steps" ansible-galaxy collection install community.grafana

run "change grafana dir" cd grafana-on-premise

export prometheus_cluster_ip=$(kubectl get svc -n monitoring | grep grafana-stack-kube-prometh-prometheus | awk '{ print$ 3 }')

run "change cluster address in grafana-on-premise definitions file" \
sed -i "'s/      ds_url: http:\/\/localhost:9090/      ds_url: http:\/\/$prometheus_cluster_ip:9090/g'" grafana-on-premise.yml

run "run grafana-on-premise file" ansible-playbook grafana-on-premise.yml

#деплой секрета для успешного получения stroppy из приватной репы
run "register user as docker user" sudo usermod -aG docker "${USER}"

#run "change current user group id to docker group id" newgrp docker

run "restarting docker service" sudo service docker restart

run "login in registry" docker login -u gitlab+deploy-token-489111 -p bzbGz3jwf1JsTrxvzN7x registry.gitlab.com

run "change docker.sock file owner" sudo chown ubuntu:docker /var/run/docker.sock

