#!/bin/bash

source ../../../common.sh

echo 'IdentityFile /home/ubuntu/.ssh/private_key.pem' > ~/.ssh/config
sudo iptables --flush

foreach
ssh %v -o StrictHostKeyChecking=no 'sudo iptables --flush'

### /Oracle.Cloud
sudo apt-get update
sudo apt-get install -y sshpass python3-pip git htop sysstat
curl https://baltocdn.com/helm/signing.asc | sudo apt-key add -
sudo apt-get install apt-transport-https --yes
echo "deb https://baltocdn.com/helm/stable/debian/ all main" | sudo tee /etc/apt/sources.list.d/helm-stable-debian.list
sudo apt-get update
sudo apt-get install helm
git clone https://github.com/kubernetes-incubator/kubespray
cd kubespray
sudo pip3 install -r requirements.txt
rm inventory/local/hosts.ini
