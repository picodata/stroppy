kubectl cp lib/fdbcli $(kubectl get po | grep sample-cluster-client | awk '{ print $1 }'):/usr/bin/fdbcli
kubectl exec --stdin --tty $(kubectl get po | grep sample-cluster-client | awk '{ print $1 }') -- chmod +x /usr/bin/fdbcli
