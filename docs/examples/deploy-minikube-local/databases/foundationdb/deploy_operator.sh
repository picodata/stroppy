#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

run "enable metallb minikube addon" minikube addons enable metallb

run "applying foundation cluster yaml" \
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml

run "applying foundation backups yaml" \
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml

run "applying foundation restores yaml" \
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml

run "applying deployment script" \
kubectl apply -f https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/v0.31.1/config/samples/deployment.yaml

run "run custom prepared script" kubectl apply -f "$SCRIPT_DIR/cluster_with_client.yaml"

echo "Waiting foundation deployment for 5 seconds..."
sleep 5
