#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

run "enable metallb minikube addon" minikube addons enable metallb

run "add tarantool operator's helm chart" \
helm repo add tarantool https://tarantool.github.io/tarantool-operator

run "install operator" \
helm install tarantool-operator tarantool/tarantool-operator --namespace default 

sleep 5