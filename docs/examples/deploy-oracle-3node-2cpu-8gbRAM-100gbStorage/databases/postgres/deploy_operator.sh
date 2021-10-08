#!/bin/bash

# Tested, didn't work for PG-13
# curl -sL https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.17.0/install.sh | bash -s v0.17.0
# kubectl create -f https://operatorhub.io/install/postgres-operator.yaml
# kubectl apply -f operator-postgres-manifest.yaml 
# The postgresql "acid-postgres-cluster" is invalid: spec.postgresql.version: Unsupported value: "13": supported values: "9.3", "9.4", "9.5", "9.6", "10", "11", "12"


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../common.sh"

# git clone https://github.com/zalando/postgres-operator.git

run "downloading postgres-operator archive" \
wget https://github.com/zalando/postgres-operator/archive/refs/tags/v1.6.0.zip

run "unpacking operator archive" unzip v1.6.0.zip && mv postgres-operator-1.6.0 postgres-operator

sleep 10

run "trying to install postgres operator" \
helm install postgres-operator postgres-operator/charts/postgres-operator

echo "Waiting postgres operator for 60 seconds..."
sleep 60

run "applying operator manifest file" \
"SCRIPT_DIR=\"$( cd \"$( dirname \"${BASH_SOURCE[0]}\" )\" &> /dev/null && pwd )\"
kubectl apply -f \"$SCRIPT_DIR/postgres-manifest.yaml\""

rm -rf postgres-operator v1.6.0.zip
