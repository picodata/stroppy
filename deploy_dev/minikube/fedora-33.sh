sudo dnf -y update
# install soft
sudo dnf -y install git
# install docker ver. 20.10.3
sudo dnf -y install dnf-plugins-core
sudo dnf config-manager --add-repo  https://download.docker.com/linux/fedora/docker-ce.repo
sudo dnf -y install docker-ce docker-ce-cli containerd.io
sudo systemctl start docker
sudo systemctl enable docker
sudo usermod -aG docker ${USER}
# install minikube ver. 1.17.1
curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
chmod +x minikube
sudo mkdir -p /usr/local/bin/
sudo install minikube /usr/local/bin/
sudo rm minikube
# install kubectl ver. 1.20.2
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
sudo dnf -y install bash-completion
echo 'source <(kubectl completion bash)' >>~/.bashrc
# install helm ver. 3.5.2
sudo dnf -y install openssl
curl -fsSL -o get_helm.sh https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3
chmod 700 get_helm.sh
./get_helm.sh
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
