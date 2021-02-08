sudo pacman -Syu --noconfirm
# install soft
sudo pacman -S git --noconfirm
# install docker  ver. 20.10.3
sudo pacman -S docker --noconfirm
sudo systemctl start docker.service
sudo systemctl enable docker.service
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
sudo pacman -S bash-completion --noconfirm
echo 'source <(kubectl completion bash)' >>~/.bashrc
# install helm ver. 3.5.2
sudo pacman -S helm --noconfirm
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
