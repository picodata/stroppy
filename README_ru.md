**От простого к сложному**
- [Запуск готового контейнера](#запуск-готового-контейнера)
- [Сборка контейнера без компиляции](#сборка-контейнера-без-компиляции)
- [Компиляция Stroppy и сборка контейнера](#компиляция-stroppy-и-сборка-контейнера)

Передача рабочей папки через ключ `--dir docs/examples` использована для примера. Директорией по-умолчанию является `benchmark/deploy`, в ней содержатся YAML-конфиги с ресурасми kubenetes и `test_config.json` с настройками для команд pop и pay.

Команда `pop` служит для создания счетов, в конце подсчитывается суммарный баланс.
```json
"pop": {
        "count": 5000
      }
```
Пример итога баланса `pop`:
```
Calculating the total balance...
Total balance: 5000041405784.0000000000
```
Команда `pay` создаёт нагрузку на СУБД, выполняя транзакции между счетами. В конце работы команды выводится итог по балансу.
```json
"pay": {
        "count": 100000,
        "zipfian": false,
        "oracle": false,
        "check": true
      }
```
Пример итога баланса `pay`, в случае нормальной работы:
```
Calculating the total balance...
Final balance: 50000259354485.0000000000 
```
Пример итога баланса `pay` при проблемах во время транзакций:
```
Calculating the total balance...             
Check balance mismatch:
before: 748385757108.0000000000
after:  4999928088923.9300000000
```

# Запуск готового контейнера
(!) Требуется заранее получить готовый исполняемый файл Stroppy.

При данном сценарии запуска будет использован готовый контейнер Stroppy из репозитория GitLab.

1. Установить зависимости.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
2. Скопировать репозиторий.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Поместить готовый бинарный файл Stroppy в папку bin.
```sh
cd stroppy && mkdir bin
cp /path/to/stroppy bin/
```
4. Скопировать ключ private_key.pem в требуемую рабочую папку.
```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```
5.  Запустить тест Postgres из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`
```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```
6. В случае преждевременного прерывания работы программы или разрыве соединения нужно удалить кластер вручную.
```sh
cd docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
terraform apply -destroy --auto-approve
```

# Сборка контейнера без компиляции
(!) Требуется аккаунт на docker.io или свой репозиторий docker-контейнеров.
1. Установить зависимости.

Зависимости для запуска.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
Зависимости сборки контейнера.
```sh
# Docker
sudo apt-get install -y apt-transport-https ca-certificates curl software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo apt-key add -
sudo add-apt-repository "deb [arch=amd64] https://download.docker.com/linux/ubuntu focal stable"
sudo apt-get update && sudo apt-get install -y docker-ce
sudo usermod -aG docker ${USER}
```
(!) После выполнения команды `sudo usermod -aG docker ${USER}` необходим релог в shell.

2. Скопировать репозиторий.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Собрать контейнер.
```sh
cd stroppy
docker build builder/ -t registry.gitlab.com/picodata/dockers/stroppy_builder:v3
docker build . -t USERNAME/stroppy
```
4. Сделать push собранного контейнера в свой репозиторий.
```sh
docker login -u USERNAME
docker push USERNAME/stroppy
```
5. Заменить контейнер Stroppy в файле `stroppy-manifest.yml`
```sh
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' benchmark/deploy/stroppy-manifest.yaml
```
(!) Если планируется запуск примеров из папки doc/examples, то замену следует проводить в соответствующей папке, например:
```sh
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage/stroppy-manifest.yaml
```
6. Скопировать ключ private_key.pem в требуемую рабочую папку
```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```
7. Запустить тест Postgres из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`
```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```
8. В случае преждевременного прерывания работы программы или разрыве соединения нужно удалить кластер вручную.
```sh
cd docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
terraform apply -destroy --auto-approve
```
# Компиляция Stroppy и сборка контейнера
1. Установить зависимости.

Зависимости для запуска.
```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```
Компилятор и зависимости сборки контейнера.
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
2. Скопировать репозиторий.
```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@gitlab.com:picodata/openway/stroppy.git
```
3. Откомилировать Stroppy.
```sh
make all
```
4. Далее можно продолжить с шага 3 [Сборки контейнера без компиляции](#сборка-контейнера-без-компиляции)
