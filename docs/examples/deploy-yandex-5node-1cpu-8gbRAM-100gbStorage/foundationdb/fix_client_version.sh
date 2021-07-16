SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

kubectl cp "$SCRIPT_DIR/lib/fdbcli" $(kubectl get po | grep sample-cluster-client | awk '{ print $1 }'):/usr/bin/fdbcli
kubectl exec --stdin --tty $(kubectl get po | grep sample-cluster-client | awk '{ print $1 }') -- chmod +x /usr/bin/fdbcli
