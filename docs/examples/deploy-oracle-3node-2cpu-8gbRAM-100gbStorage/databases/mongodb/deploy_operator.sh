#!/bin/bash

source ./../../common.sh


run "applying bundle.yaml" "kubectl apply -f mongodb/bundle.yaml"
run "applying secrets.yaml" "kubectl apply -f mongodb/secrets.yaml"
run "applying cr.yaml" "kubectl apply -f mongodb/cr.yaml"
