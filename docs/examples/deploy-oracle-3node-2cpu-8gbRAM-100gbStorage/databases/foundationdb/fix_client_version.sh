#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
CLUSTER_ADDR="$(kubectl get po | grep sample-cluster-client | awk '{ print $1 }')"
run "copying fdbcli control utility to foundation cluster dispatcher node" \
kubectl cp "$SCRIPT_DIR/lib/fdbcli" "$CLUSTER_ADDR:/usr/bin/fdbcli"

run "try to run new fdbcli utility" \
kubectl exec --stdin --tty "$(kubectl get po | grep sample-cluster-client | awk '{ print $1 }')" -- chmod +x /usr/bin/fdbcli
