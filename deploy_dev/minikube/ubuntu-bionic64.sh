sudo apt-get update
# install docker ver. 20.10.3
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu bionic stable"
sudo apt-get update
sudo apt-get install -y docker-ce
sudo usermod -aG docker ${USER}
# install minikube ver. 1.17.1
curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
chmod +x minikube
sudo mkdir -p /usr/local/bin/
sudo install minikube /usr/local/bin/
sudo rm minikube
# install kubectl ver. 1.20.2
sudo snap install kubectl --classic
echo 'source <(kubectl completion bash)' >>~/.bashrc
# install helm ver. 3.5.2
sudo snap install helm --classic
echo 'source <(helm completion bash)' >>~/.bashrc
# systemd unit
sudo tee /etc/systemd/system/minikube.service<<EOF
[Unit]
Description=Runs minikube on startup
After=vboxautostart-service.service vboxballoonctrl-service.service vboxdrv.service vboxweb-service.service docker.service

[Service]
ExecStart=/usr/local/bin/minikube start
ExecStop=/usr/local/bin/minikube stop
Type=oneshot
RemainAfterExit=yes
User=${USER}
Group=${USER}

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
sudo systemctl enable minikube
# to apply "sudo usermod -aG docker ${USER}" for all shells
# sudo reboot
