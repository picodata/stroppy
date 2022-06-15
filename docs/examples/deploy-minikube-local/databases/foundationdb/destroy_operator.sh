#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"


run "deleting custom cluster definitions" kubectl delete -f "$SCRIPT_DIR/cluster_with_client.yaml"

run "deleting foundation cluster definitions" \
kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml

run "deleting foundation backups definitions" \
kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml

run "deleting foundation restores definitions" \
kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml

run "deleting deployment definitions" \
kubectl delete -f https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/master/config/samples/deployment.yaml
