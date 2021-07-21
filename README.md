**From simple to complex**
- [Run from ready container](#run-from-ready-container)
- [Build container without compilation](#build-container-without-compilation)
- [Compile Stroppy and build container](#compile-stroppy-and-build-container)
# Run from ready container
(!) Requires a compiled Stroppy executable.

In this case, the ready-made Stroppy container from the GitLab repository will be used.

1. Install dependencies.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
2. Clone the repository.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Place the compiled Stroppy binary in the bin folder.
```sh
cd stroppy && mkdir bin
cp /path/to/stroppy bin/
```
4. Copy the private_key.pem key to the required working folder.
```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```
5.  Run Postgres test from folder `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`
```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```
6. If the program interrupts unexpectedly or the connection is disconnected, you must manually remove the cluster.
```sh
cd docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
terraform apply -destroy --auto-approve
```

# Build container without compilation
(!) Requires a docker.io account or your own docker container repository.
1. Install dependencies.

Runtime dependencies.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
Container build dependencies.
```sh
# Docker
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
sudo apt-get update && sudo apt-get install -y docker-ce
sudo usermod -aG docker ${USER}
```
(!) After executing the command `sudo usermod -aG docker ${USER}` relog in the shell required.

2. Clone the repository.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Build container.
```sh
cd stroppy
docker build builder/ -t registry.gitlab.com/picodata/dockers/stroppy_builder:v3
docker build . -t USERNAME/stroppy
```
4. Push the builded container to your repository.
```sh
docker login -u USERNAME
docker push USERNAME/stroppy
```
5. Replace Stroppy container in file `stroppy-manifest.yml`
```sh
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' benchmark/deploy/stroppy-manifest.yaml
```
(!) If you plan to run examples from the doc/examples folder, then the replacement should be performed in the appropriate folder, for example:
```sh
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage/stroppy-manifest.yaml
```
6. Copy the private_key.pem key to the required working folder
```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```
7. Run Postgres test from folder `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`
```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```
8. If the program interrupts unexpectedly or the connection is disconnected, you must manually remove the cluster.
```sh
cd docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
terraform apply -destroy --auto-approve
```
# Compile Stroppy and build container
1. Install dependencies

Runtime dependencies.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
Compile and container build dependencies.
```sh
# Go & make & gcc
sudo apt install -y make gcc
wget https://golang.org/dl/go1.16.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.16.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
# Docker
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
sudo apt-get update && sudo apt-get install -y docker-ce
sudo usermod -aG docker ${USER}
```
2. Clone the repository.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Compile Stroppy
```sh
make all
```
4. Then you can continue from step 3 "Build container" in [Build container without compilation](#build-container-without-compilation)
