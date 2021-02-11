## 1. Install docker, minikube, kubectl, helm, minikube systemd service
Install:

```
cd deploy_dev/minikube/
sh YOUR_DISTRO.sh
sudo reboot
```
Verify (after reboot + 5 min):
```
kubectl get pod --all-namespaces 
NAMESPACE     NAME                               READY   STATUS    RESTARTS   AGE
kube-system   coredns-74ff55c5b-njn98            1/1     Running   0          87s
kube-system   etcd-minikube                      1/1     Running   0          96s
kube-system   kube-apiserver-minikube            1/1     Running   0          96s
kube-system   kube-controller-manager-minikube   1/1     Running   0          96s
kube-system   kube-proxy-nhw6r                   1/1     Running   0          87s
kube-system   kube-scheduler-minikube            1/1     Running   0          96s
kube-system   storage-provisioner                1/1     Running   0          101s
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
## 3. Deploy FoundationDB
Deploy:
```
cd deploy_dev/foundationdb/
sh deploy_operator.sh
kubectl edit configmap config -n metallb-system
```
*Note for archlinux:*
```
sudo pacman -S nano --noconfirm
KUBE_EDITOR="nano" kubectl edit configmap config -n metallb-system
```
```yaml
apiVersion: v1
data:
  config: |
    address-pools:
    - name: default
      protocol: layer2
      addresses:
      - 192.168.99.50-192.168.99.99
```
Check sample-cluster-client pod state is 'Running' (in 3~6 min):
```
kubectl get pod | grep sample-cluster-client
sample-cluster-client-c49c885b8-4sns5                         1/1     Running   0          6m41s
```
Fix client version:
```
sh fix_client_version.sh
```
Verify:
```
kubectl exec --stdin --tty $(kubectl get po | grep sample-cluster-client | awk '{ print $1 }') -- /bin/bash
fdbcli
```
```
Using cluster file `/var/dynamic-conf/fdb.cluster'.

The database is available.

Welcome to the fdbcli. For help, type `help'.
fdb> status

Using cluster file `/var/dynamic-conf/fdb.cluster'.

Configuration:
  Redundancy mode        - double
  Storage engine         - ssd-2
  Coordinators           - 3
  Desired Proxies        - 3
  Desired Resolvers      - 1
  Desired Logs           - 3

Cluster:
  FoundationDB processes - 7
  Zones                  - 7
  Machines               - 7
  Memory availability    - 117.6 GB per process on machine with least available
  Fault Tolerance        - 1 machine
  Server time            - 02/11/21 04:11:38

Data:
  Replication health     - Healthy
  Moving data            - 0.000 GB
  Sum of key-value sizes - 0 MB
  Disk space used        - 629 MB

Operating space:
  Storage server         - 156.1 GB free on most full server
  Log server             - 156.1 GB free on most full server

Workload:
  Read rate              - 10 Hz
  Write rate             - 0 Hz
  Transactions started   - 5 Hz
  Transactions committed - 0 Hz
  Conflict rate          - 0 Hz

Backup and DR:
  Running backups        - 0
  Running DRs            - 0

Client time: 02/11/21 04:11:38
```
Info about sample-cluster-client OS:
```
cat /etc/os-release 
PRETTY_NAME="Debian GNU/Linux 10 (buster)"
NAME="Debian GNU/Linux"
VERSION_ID="10"
VERSION="10 (buster)"
VERSION_CODENAME=buster
ID=debian
HOME_URL="https://www.debian.org/"
SUPPORT_URL="https://www.debian.org/support"
BUG_REPORT_URL="https://bugs.debian.org/"
```
