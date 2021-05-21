### REQUIREMENTS
* vagrant
* 32 Gb free RAM
### UP & RUN
```sh
vagrant up
vagrant ssh master
cd kubespray
sh /home/vagrant/deploy_dev/terraform/vagrant_7_workers/deploy_cluster.sh
```
#### VERIFY
```sh
kubectl top nodes
NAME       CPU(cores)   CPU%   MEMORY(bytes)   MEMORY%   
master     184m         4%     2155Mi          64%       
worker-1   106m         2%     1582Mi          44%       
worker-2   164m         4%     1824Mi          50%       
worker-3   128m         3%     1589Mi          44%       
worker-4   122m         3%     1590Mi          44%       
worker-5   122m         3%     1573Mi          43%       
worker-6   125m         3%     1540Mi          43%       
worker-7   113m         2%     1613Mi          45%
```
### DEPLOY ZALANDO POSTGRES OPERATOR
```sh
cd ~/deploy_dev/postgres/
./deploy_operator.sh
```
#### VERIFY
```sh
kubectl get po
NAME                                 READY   STATUS    RESTARTS   AGE
acid-postgres-cluster-0              1/1     Running   0          3m16s
acid-postgres-cluster-1              1/1     Running   0          2m
postgres-operator-84f56f9695-24rqg   1/1     Running   0          4m19s
```
