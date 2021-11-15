#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"


run "applying crd for the operator" \
kubectl apply -f "$SCRIPT_DIR/deploy_config/crd/bases/crdb.cockroachlabs.com_crdbclusters.yaml"

sleep 10

run "installing operator" \
kubectl apply -f "$SCRIPT_DIR/install/operator.yaml"


# ==============
run "creating client operator" kubectl create -f "$SCRIPT_DIR/client-operator.yaml"

run "instantiating cocroachdb" kubectl apply -f "$SCRIPT_DIR/crdb.yaml"


# ==============
run "applying rbac rules" \
kubectl apply -f "$SCRIPT_DIR/deploy_config/rbac/database.yaml"

# === monitoring ====
run "labeling cockroachdb svc for prometheus" \
kubectl label svc cockroachdb prometheus=cockroachdb

run "applying cockroachdb prometheus config" \
kubectl apply -f "$SCRIPT_DIR/monitoring/prometheus.yaml"
