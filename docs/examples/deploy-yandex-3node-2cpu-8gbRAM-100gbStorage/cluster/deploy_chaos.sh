#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../common.sh"

run "add chaos repository" helm repo add chaos-mesh https://charts.chaos-mesh.org
run "activating chaos repository" helm search repo chaos-mesh

run "create chaos namespace" kubectl create ns chaos-testing

run "perf chaos installation" helm install chaos-mesh chaos-mesh/chaos-mesh --namespace=chaos-testing
run "perf chaos update installation" elm upgrade chaos-mesh chaos-mesh/chaos-mesh \
  --namespace=chaos-testing \
  --set dashboard.create=true
