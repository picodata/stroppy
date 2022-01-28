#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

run "add tarantool operator's helm chart" \
helm repo add tarantool https://tarantool.github.io/tarantool-operator

run "install operator" \
helm install tarantool-operator tarantool/tarantool-operator --namespace default 

run "waiting for 120 seconds of operator deployment " \
sleep 120

run "install application" \
helm install -f databases/cartridge/values.yaml stroppy-test-app tarantool/cartridge

run "waiting for 120 seconds of application deployment " \
sleep 120