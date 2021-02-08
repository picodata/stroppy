## 1. Install docker, minikube, kubectl, helm, minikube systemd service
Install:

```
cd deploy_dev/minikube/
sh YOUR_DISTRO.sh
sudo reboot
```
Verify (after reboot):
```
kubectl top pods
NAME                                                     CPU(cores)   MEMORY(bytes)   
alertmanager-grafana-stack-kube-prometh-alertmanager-0   1m           15Mi            
grafana-stack-5c7d68f97-hb6gt                            3m           98Mi            
grafana-stack-kube-prometh-operator-b6479499c-8vflx      1m           29Mi            
grafana-stack-kube-state-metrics-77f6cc9c4b-7p62n        1m           10Mi            
grafana-stack-prometheus-node-exporter-zfmjw             3m           10Mi            
prometheus-grafana-stack-kube-prometh-prometheus-0       23m          281Mi
```

Ready for:
* ubuntu-focal64
* fedora-33
* archlinux

Start / stop:
```
sudo systemctl start minikube
sudo systemctl stop minikube
```
## 2. Deploy Grafana and Prometheus monitoring stack, expose Grafana via Ingress
Deploy:
```
cd deploy_dev/monitoring/
sh k8s-grafana-stack.sh
```
Verify:

http://192.168.49.2

Login credentials: admin / prom-operator
