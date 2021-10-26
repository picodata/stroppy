#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

errorless_run "creating role bindings" \
kubectl create clusterrolebinding ubuntu-cluster-admin-binding --clusterrole=cluster-admin

run "applying crd for the operator" \
kubectl apply -f "$SCRIPT_DIR/operator/crds.yaml"

sleep 10

run "installing operator" \

kubectl apply -f "$SCRIPT_DIR/operator/operator.yaml"

sleep 10


# ==============
run "instantiating cocroachdb" kubectl apply -f "$SCRIPT_DIR/crdb.yaml"

run "creating client" kubectl create -f "$SCRIPT_DIR/client-operator.yaml"


# == monitoring ==============
run "labeling cockroachdb svc for prometheus" \
kubectl label svc cockroachdb prometheus=cockroachdb

run "applying cockroachdb prometheus config" \
kubectl apply -f "$SCRIPT_DIR/monitoring/prometheus.yaml"

