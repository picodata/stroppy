#!/bin/bash


SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
source "$SCRIPT_DIR/../../../common.sh"

ADDR_LIST=
while [[ $# -gt 0 ]]; do
    key="$1"
    case $key in
    --pod-addresses)
      ADDR_LIST=$2
      shift 2
      ;;
    *)
      echo "unknown key $key"
      shift
      ;;
    esac
done

if [ -z "$ADDR_LIST" ]; then
    echo "cluster pods addresses is not specified"
    exit 1
fi

run "configure ssh client" \
echo \"IdentityFile /home/ubuntu/.ssh/private_key.pem\" \>\> ~/.ssh/config

run "flushing pf tables" sudo iptables --flush

IFS=','
read -ra NODESLIST <<< "$ADDR_LIST"

for node_addr in "${NODESLIST[@]}"; do
  ssh "$node_addr" -o StrictHostKeyChecking=no 'sudo iptables --flush'
done

run "update system package cache" sudo apt-get update

run "installing some system packages" \
sudo apt-get install -y sshpass python3-pip git htop sysstat

### /Oracle.Cloud
run "applying custom secure key" curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
run "installing apt-transport-https package" sudo apt-get install apt-transport-https --yes


run "configuring helm list file" \
echo \"deb https://baltocdn.com/helm/stable/debian/ all main\" \| sudo tee /etc/apt/sources.list.d/helm-stable-debian.list

run "secondly updating system package cache" sudo apt-get update
run "installing helm" sudo apt-get install helm

run "cloning kubespray" git clone https://github.com/kubernetes-incubator/kubespray
run "change dir to kubespray" cd kubespray

run "installing kubespray requirements" sudo pip3 install -r requirements.txt

run "deleting hosts.ini file" rm inventory/local/hosts.ini
