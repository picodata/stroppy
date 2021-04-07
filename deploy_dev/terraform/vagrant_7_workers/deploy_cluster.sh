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
kubectl apply -f ~/deploy_dev/monitoring/metrics-server.yaml
kubectl apply -f ~/deploy_dev/monitoring/ingress-grafana.yaml
kubectl apply -f https://raw.githubusercontent.com/rancher/local-path-provisioner/master/deploy/local-path-storage.yaml
