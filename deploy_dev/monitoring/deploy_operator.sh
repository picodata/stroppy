helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install grafana-stack prometheus-community/kube-prometheus-stack
minikube addons enable ingress
kubectl apply -f metrics-server.yaml
echo "Waiting grafana deploy for 60 seconds..."
sleep 60
kubectl apply -f ingress-grafana.yaml
