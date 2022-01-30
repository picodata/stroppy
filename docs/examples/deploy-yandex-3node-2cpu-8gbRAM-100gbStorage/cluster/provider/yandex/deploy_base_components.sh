#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../../common.sh"


export DEBIAN_FRONTEND='noninteractive'
run "installing ca-certs" sudo apt install ca-certificates

run "update system package cache state" sudo apt-get update -y

run "installing required system components" \
sudo apt-get install -y sshpass python3-pip git htop sysstat

run "adding baltocdn encryption key to system package manager" \
curl https://baltocdn.com/helm/signing.asc \| sudo apt-key add -

run "installing apt-transport-https" sudo apt-get install apt-transport-https --yes

run "prepare helm repository" \
echo \"deb https://baltocdn.com/helm/stable/debian/ all main\" \| sudo tee /etc/apt/sources.list.d/helm-stable-debian.list

run "secondly update system package cache state" sudo apt-get update -y

run "installing helm" sudo apt-get install helm -y

run "cloning kubespray from github" git clone https://github.com/kubernetes-incubator/kubespray

run "change directory to kubespray" cd kubespray

run "installing kubespray python3 requirements" sudo pip3 install -r requirements.txt

run "removing unnecessary hosts.ini file" rm inventory/local/hosts.ini
