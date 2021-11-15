#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"


run "removing cockroachdb monitoring facility" \
run kubectl delete -f "$SCRIPT_DIR/monitoring/prometheus.yaml"

run "deleting cockroachdb cluster" \
kubectl delete -f "$SCRIPT_DIR/crdb.yaml"

run "removing cockroachdb operator" \
kubectl delete -f "$SCRIPT_DIR/install/operator.yaml"
