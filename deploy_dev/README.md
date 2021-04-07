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
* ubuntu-bionic64
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
sh deploy_operator.sh
```
Verify:

[http://192.168.49.2](http://192.168.49.2)

Login credentials: admin / prom-operator
## 3.1 Deploy FoundationDB
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
      - 192.168.49.50-192.168.49.99
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
cat /etc/os-release | grep PRETTY_NAME
PRETTY_NAME="Debian GNU/Linux 10 (buster)"
```
Operator monitoring status: [Issue: Expose all status fields via metrics #396](https://github.com/FoundationDB/fdb-kubernetes-operator/issues/396)
## 3.2 Deploy PosgreSQL
Deploy:

Check config [postgres/postgres-manifest.yaml](postgres/postgres-manifest.yaml)
```
cd deploy_dev/postgres/
sh deploy_operator.sh
```
Verify and change password for user stroppy:
```
kubectl exec --stdin --tty acid-postgres-cluster-0 -- /bin/su -- postgres
postgres@acid-postgres-cluster-0:~$ psql
psql (13.1 (Ubuntu 13.1-1.pgdg18.04+1))
Type "help" for help.

postgres=# \du
                                                                               List of roles
     Role name      |                         Attributes                         |                                        Member of                                         
--------------------+------------------------------------------------------------+------------------------------------------------------------------------------------------
 admin              | Create DB, Cannot login                                    | {stroppy,bar_owner,bar_owner_user}
 bar_data_owner     | Cannot login                                               | {bar_data_reader,bar_data_writer}
 bar_data_reader    | Cannot login                                               | {}
 bar_data_writer    | Cannot login                                               | {bar_data_reader}
 bar_history_owner  | Cannot login                                               | {bar_history_reader,bar_history_writer}
 bar_history_reader | Cannot login                                               | {}
 bar_history_writer | Cannot login                                               | {bar_history_reader}
 bar_owner          | Cannot login                                               | {bar_data_owner,bar_history_owner,bar_reader,bar_reader_user,bar_writer,bar_writer_user}
 bar_owner_user     |                                                            | {bar_owner}
 bar_reader         | Cannot login                                               | {}
 bar_reader_user    |                                                            | {bar_reader}
 bar_writer         | Cannot login                                               | {bar_reader}
 bar_writer_user    |                                                            | {bar_writer}
 postgres           | Superuser, Create role, Create DB, Replication, Bypass RLS | {}
 robot_zmon         | Cannot login                                               | {}
 standby            | Replication                                                | {}
 stroppy            | Superuser, Create DB                                       | {}
 zalandos           | Create DB, Cannot login                                    | {}

postgres=# \c stroppy
You are now connected to database "stroppy" as user "postgres".

stroppy=# ALTER USER stroppy PASSWORD 'stroppy';
ALTER ROLE
```
Info about acid-postgres-cluster OS:
```
cat /etc/os-release | grep PRETTY_NAME
PRETTY_NAME="Ubuntu 18.04.5 LTS"
```
Get password for manual connection for user: postgres, database: foo
```
kubectl get secret postgres.acid-postgres-cluster.credentials.postgresql.acid.zalan.do -o 'jsonpath={.data.password}' | base64 -d
```
Get local port via port-forward
```
export PGMASTER=$(kubectl get pods -o jsonpath={.items..metadata.name} -l application=spilo,cluster-name=acid-postgres-cluster,spilo-role=master -n default)
kubectl port-forward $PGMASTER 6432:5432 -n default
```
Connect to localhost:6432 db: stroppy user: postgres: password: <from kubectl get secret ...>

Run stroppy
```
kubectl run -i --tty stroppy-client --image=ghoru/stroppy -- /bin/bash
```
Run benchmark inside pod stroppy-client
```
bin/stroppy pop --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --count 5000
bin/stroppy pay --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --check --count=100000
```
Operator monitoring status: [Monitoring or tuning Postgres is not in scope of the operator in the current state](https://github.com/zalando/postgres-operator/blob/master/docs/index.md#scope)

### Enable pgboncer pooler on 6432 port:
In `postgres-manifest.yaml` set enableConnectionPooler
```
spec.enableConnectionPooler: true
```
Verify:
```
kubectl get po
NAME                                            READY   STATUS    RESTARTS   AGE
acid-postgres-cluster-0                         1/1     Running   0          35s
acid-postgres-cluster-1                         1/1     Running   0          65s
acid-postgres-cluster-pooler-6d7d69ffcf-vtmtp   1/1     Running   0          25s
acid-postgres-cluster-pooler-6d7d69ffcf-zptt4   1/1     Running   0          25s
postgres-operator-55f599cc9c-hmk9v              1/1     Running   0          4m44
```

## 3.3 Deploy MongoDB
Deploy:

Check config [mongodb/mongodb-cluster.yaml](mongodb/mongodb-cluster.yaml)
```
cd deploy_dev/mongodb/
sh deploy_operator.sh
```

Verify:
```
kubectl get mongodbcommunity
NAME              PHASE     VERSION
mongodb-cluster   Running
```
```
kubectl get pod -o wide
NAME                                           READY   STATUS    RESTARTS   AGE   IP           NODE       NOMINATED NODE   READINESS GATES
mongodb-client                                 1/1     Running   0          54m   172.17.0.5   minikube   <none>           <none>
mongodb-cluster-0                              2/2     Running   0          66m   172.17.0.2   minikube   <none>           <none>
mongodb-cluster-1                              2/2     Running   0          66m   172.17.0.3   minikube   <none>           <none>
mongodb-cluster-2                              2/2     Running   0          65m   172.17.0.4   minikube   <none>           <none>
mongodb-kubernetes-operator-7cddf7cbd4-n5bnf   1/1     Running   1          23h   172.17.0.8   minikube   <none>           <none>
```
Localhost port access. User/pass: mongo/mongo. Database: admin.
```
kubectl port-forward mongodb-cluster-0 27017:27017
```
Client-in-pod access
```
export MGMASTER=$(kubectl get pod mongodb-cluster-0 -o wide --no-headers | awk '{ print $6}')
kubectl exec -it mongodb-client -- /usr/bin/mongo -u mongo -p mongo $MGMASTER:27017/admin
```
Verify access (check that `PRIMARY` in prompt):
```
mongodb-cluster:PRIMARY> show dbs
admin   0.000GB
config  0.000GB
local   0.000GB
```
[Force a Member to be Primary Using Database Commands](https://docs.mongodb.com/manual/tutorial/force-member-to-be-primary/#force-a-member-to-be-primary-using-database-commands)
