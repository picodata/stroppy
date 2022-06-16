- [Введение](#введение)
- [Основные возможности](#основные-возможности)
- [Пример использования](#пример-использования)
- [Компиляция и Сборка](#компиляция-и-сборка)
- [Деплой stroppy в minikube](#деплой-stroppy-в-minikube)
- [Команды](#команды)
- [Сценарий тестирования](#сценарий-тестирования)
- [Модель данных](#модель-данных)
- [Управляемые неисправности](#управляемые-неисправности)
- [Особенности использования](#особенности-использования)

# Введение

Stroppy - это фреймворк для тестирования различных баз данных. Он позволяет 
развернуть кластер в облаке, запустить нагрузочные тесты и имитировать, например, 
сетевую недоступность одной из нод в кластере.  

Как же все это позволяет проверить надёжность? Дело в том, что для проверки 
целостности данных существует весьма элегантный "банковский" тест. Мы заполняем 
БД рядом записей о неких "счетах" с деньгами. Затем имитируем серию переводов 
с одного счета на другой в рамках предоставляемых СУБД транзакций. В результате 
любого количества транзакций общая сумма денег на счетах не должна измениться.  

Чтобы усложнить задачу для СУБД, Stroppy может попытаться сломать кластер БД, 
ведь в реальном мире отказы случаются гораздо чаще, чем нам хочется. А для 
горизонтально масштабируемых БД это случается еще чаще, так как большее количество 
физических узлов дает больше точек отказа.  

На данный момент мы реализовали поддержку FoundationDB, MongoDB, CockroachDB, 
PostgreSQL, YandexDB (нужно же с чем-то сравнивать всё остальное).  
Кроме того, для того, чтобы было удобнее анализировать результаты тестов, 
stroppy интегрирован с Grafana и после каждого прогона в автоматическом режиме 
собирает архив с графиками мониторинга, масштабированными по времени прогона. 
Также для FoundationDB и MongoDB поддерживается сбор внутренней статистики с 
заданной периодичностью - для FoundationDB собираются данные консольной команды 
status json, для MongoDB - данные команды db.serverStatus().  

***Важное уточнение***:
Данная инструкция актуальна для использования на ОС Ubuntu >=18.04 и пока не 
проверялась на другиx операционных системах.


# Основные возможности  

- Развертывание кластера виртуальных машин в выбранном облаке через terraform. Поддерживается Yandex.Cloud и Oracle.Cloud
- Развертывание kubernetes кластера в развернутом кластере виртуальных машин
- Развертывание выбранной СУБД в этом кластере
- Сбор статистики из Grafana метрик k8s кластера и системных метрик виртуальных машин (CPU, RAM, storage и т.д.)
- Управление параметрами тестов и самого развертывания - от кол-ва VM до подаваемой нагрузки и управляемых неполадок
- Запуск тестов по команде из консоли
- Логирование хода теста - текущее и итоговое latency и RPS
- Удаление кластера виртуальных машин
- Развертывание нескольких кластеров с одной локальной машины с изолированным мониторингом и консолью запуска  

# Пример использования

Допустим, мы хотим проверить, какую нагрузку выдержит кластер FoundationDB, состоящий из 3 узлов по 1 ядру и 8 ГБ RAM на узел, при этом кластер будет развернут соответствующим [k8s оператором](https://github.com/FoundationDB/fdb-kubernetes-operator).

Предварительные действия:

## Запуск в docker
1) Запускаем docker образ со `stroppy` в интерактивном режиме
```shell
docker run -it docker.binary.picodata.io/stroppy:latest
```

2) Запускаем `stroppy` с параметрами для тестируемой базы данных
```shell
stroppy deploy --cloud yandex --flavor small --nodes 3 --dbtype ydb --log-level trace
```

## Запуск локально со сборкой из исходников
1) Клонируем себе репозиторий на локальную машину:
```shell
git clone git@github.com:picodata/stroppy.git
```

2) Устанавливаем клиент FoundationDB
```shell
curl -fLO "https://github.com/apple/foundationdb/releases/download/6.3.23/foundationdb-clients_6.3.23-1_amd64.deb"
sudo dpkg -i foundationdb-clients_6.3.23-1_amd64.deb
```

3) Устанавливаем `Ansible`
```shell
sudo apt update
sudo apt install -y python ansible
```

4) Переходим в директорию со `stroppy` и устанавливаем зависимости для корректной работы `kubespray`
```shell
cd stroppy
python3 -m venv stroppy
source stroppy/bin/activate
python3 -m pip install -f third_party/kubespray/requirements.txt
```

5) Устанаваливаем kubectl
```shell
curl -fsLO https://storage.googleapis.com/kubernetes-release/release/v1.24.0/bin/linux/amd64/kubectl
sudo chmod +x kubectl
sudo cp kubectl /usr/bin/kubectl
```

7) Запускаем сборку stroppy.
```shell
sudo go build -o /usr/bin/stroppy ./cmd/stroppy
```

7) апускаем `stroppy` с параметрами для тестируемой базы данных
```shell
stroppy deploy --cloud yandex --flavor small --nodes 3 --dbtype ydb --log-level trace
```

> **Note:** Для того что бы задеплоить кластер кубернетеса в облачной 
> инфраструктуре в корневой директории с проектом должны быть:  
> **Yandex.Cloud:**  
> **1.** Приватный и публичный ключи. Обязательно именовать их как 
> `id_rsa` и `id_rsa.pub` во избежание проблем. Создать ключи можно при 
> командой `ssh-keygen -b 4096 -f id_rsa`.  
> **2.** Файл с credentials и атрибутами для доступа к cloud, лучше назвать его 
> `vars.tf`, для гарантированной совместимости.
> **Oracle.Cloud:**  
> **1.** Приватный ключ `private_key.pem`. Данный ключ необходимо получить с 
> помощью веб-интерфейса провайдера.

## Варианты запуска stroppy

Что бы протестировать выбранную конфигурацию с помощью stroppy мы можем 
пойти двумя разными путями:
- Запустить тесты вручную - Развернуть виртуальные машины, кластер k8s и 
СУБД вручную, поднять рядом под со stroppy из манифеста, подложить ему в 
директорию /root файл fdb.cluster и запустить загрузку счетов и затем 
тест переводов, воспользовавшись командами из [Команды](#команды).
- Запустить тесты и развертывание автоматически

Вне зависимости от выбранного варианта нужно задать необходимые параметры 
через соответствующие флаги командной строки и файлы конфигурации:  

`third_party/terraform/vars.tf` - это параметры создаваемой `terraform` 
инфраструктуры. Параметры для инфры можно задать как непосредственно в файле, 
так и с помощью переменных окружения.

> **Note:** Список переменных можно посмотреть в [tfvars](third_party/terraform/tfvars)

`third_party/terraform/main.tf` - скрипт по созданию инфраструктуры для 
`terraform`. Не обязателен так как `stroppy` умеет создавать `hcl` 
скрипты сам основываясь лишь на аргументах командной строки.  
Но! Если в процессе деплоя строппи обнаружит скрипт, то он применит именно
его, лишь поменяв некоторые параметры на основе переданных в командной строке.

> **Note:**: Обратите внимаение, что VM в Oracle.Cloud, как и аналогичные ему,
> использует процессоры с мультитредингом, а k8s, в таком случае, при оценке
> заданных limits и requests ориентируется на кол-во виртуальных ядер (тредов),
> а не на физические. Поэтому указав cpu:2, мы фактически получили 4
> виртуальных ядра, 2 из которых отдадим FoundationDB.

`third_party/extra/manifests/` - в этой директории находятся манифесты
для k8s которые будут использованы в процессе деплоя. При желании их можно
изменять.

`third_party/extra/manifests/databases/` - здесь находятся манифесты для деплоя
тестируемых баз данных.

`third_party/tests/test_config.json` Файл с параметрами для будущих тестов. 
Пример файла ниже. Название и назначение параметров совпадает с параметрами 
для запуска тестов из раздела [Команды](#команды).

> **Note:** Файл не обязателен, и будет использован `stroppy` только в том 
> случае если будет отличаться от дефолтного.

```json
{
  "log_level": "info", 
  "banRangeMultiplier": 1.1,
  "database_type": [ 
    "fdb" 
  ],
  "cmd": [
    {
      "pop": { // здесь задаем параметры теста загрузки счетов
        "count": 5000 
      }
    },
    {
      "pay": { // здесь задаем параметры теста переводов
        "count": 100000, 
        "zipfian": false, 
        "oracle": false, 
        "check": true 
      }
    }
  ]
}
```

## Примеры запуска stroppy deploy

Для Oracle Cloud:
```shell
stroppy deploy --cloud oracle --flavor small --nodes 4 --dbtype ydb --log-level debug
stroppy deploy --cloud oracle --flavor major --nodes 10 --dbtype postrgress --log-level trace
```

для Yandex.Cloud:
```shell
stroppy deploy --cloud yandex --flavor small --nodes 1 --dbtype fdb --log-level debug
stroppy deploy --cloud yandex --flavor medium --nodes 3 --dbtype cockroach --log-level debug
```

> **Note:** Описание ключей команд можно найти в разделе - [Команды](#команды) 
> текущего руководства.

Результатом выполнения команды после вывода в консоль некоторого кол-ва 
отладочной информации и примерно получаса времени должно стать сообщение вида:

```sh
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:3000.
To access to kubernetes cluster in cloud use address localhost:6443.
Enter "quit" or "exit" to exit stroppy and destroy cluster.
Enter "pop" to start populating deployed DB with accounts.
Enter "pay" to start transfers test in deployed DB.
To use kubectl for access kubernetes cluster in another console
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"
>                               
```

***Важное замечание***: Указанные порты это порты по умолчанию для доступа к мониторингу (порт 3000) и доступа к API k8s кластера(6443). Т.к. stroppy поддерживает развертывание нескольких кластеров на одной локальной машине, то порты для кластеров, запущенных после первого, будут инкрементироваться.

***Важное замечание***: Для варианта с тестированием FoundationDB, который мы запланировали после успешного деплоя и вывода сообщения необходимо выполнить немного ручных манипуляций из пункта 2 раздела "Особенности использования".

Под сообщением будет открыта консоль для выбора команд. Для старта теста загрузки счетов необходимо ввести команду ```pop``` и дождаться выполнения, после чего мы вновь увидим консоль для ввода команд и можем вввести команду ```pay``` для старта теста переводов. Все команды будут запущены с теми параметрами, которые мы задали на этапе конфигурирования в файле test_config.json.

Результатом работы команд будет несколько файлов в корне директории с конфигурацией. Например, для нашего случая:

```pop_test_run_2021-10-15T16:09:51+04:00.log``` - файл с логами теста загрузки счетов  
```pay_test_run_2021-10-15T16:10:46+04:00.log``` - файл с логами теста переводов  
```monitoring/grafana-on-premise/fdb_pop_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz``` - архив с метриками теста загрузки счетов  
```monitoring/grafana-on-premise/fdb_pay_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz``` - архив с метриками теста переводов  

Если вместо сообщения в консоли возникает ошибка, которая не устраняется перезапуском (не более 3 повторов), то заводим issue с описанием ошибки в <https://github.com/picodata/stroppy/issues>.

Переповтор идемпотентен для кластера VM и кластера K8S, поэтому при переповторе не будут созданы новые виртуальные машины и кластер Kubernetes.

***Важное замечание***: stroppy пока не гарантирует идемпотентность в отношении развертывания выбранной СУБД. Такое поведение оставлено без изменений в том числе, чтобы дать возможность исправить ошибку конфигурирования БД без редеплоя всего кластера.

# Компиляция и сборка

- [Запуск готового контейнера](#запуск-готового-контейнера)
- [Сборка контейнера без компиляции](#сборка-контейнера-без-компиляции)
- [Компиляция Stroppy и сборка контейнера](#компиляция-stroppy-и-сборка-контейнера)

# Запуск готового контейнера

(!) Требуется заранее получить готовый исполняемый файл Stroppy.

При данном сценарии запуска будет использован готовый контейнер Stroppy из репозитория GitLab.

1.Установить зависимости.

```sh
# FoundationDB libs
wget https://www.foundationdb.org/downloads/6.3.15/ubuntu/installers/foundationdb-clients_6.3.15-1_amd64.deb
sudo dpkg -i foundationdb-clients_6.3.15-1_amd64.deb
# Terraform
curl -fsSL https://apt.releases.hashicorp.com/gpg | sudo apt-key add -
sudo apt-add-repository "deb [arch=amd64] https://apt.releases.hashicorp.com $(lsb_release -cs) main"
sudo apt-get update && sudo apt-get install terraform
```

2.Скопировать репозиторий.

```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@github.com:picodata/stroppy.git
```

3.Поместить готовый бинарный файл Stroppy в папку bin.

```sh
cd stroppy && mkdir bin
cp /path/to/stroppy bin/
```

4.Скопировать ключ private_key.pem в требуемую рабочую папку.

```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```

5.Запустить развертывание кластера, например, с конфигурацией из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

6.В случае ошибок развертывания, которые не устраняются переповтором, и мешают выводу консоли управления, нужно удалить кластер вручную, выполнив в корне папки с выбранной конфигурацией команду, например:

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

2.Скопировать репозиторий.

```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@github.com:picodata/stroppy.git
```

3.Собрать контейнер.

```sh
cd stroppy
docker build builder/ -t registry.gitlab.com/picodata/dockers/stroppy_builder:v3
docker build . -t USERNAME/stroppy
```

4.Сделать push собранного контейнера в свой репозиторий.

```sh
docker login -u USERNAME
docker push USERNAME/stroppy
```

5.Заменить контейнер Stroppy в файле `stroppy-manifest.yml`

```sh
sed -i 's/registry.github.com\/picodata\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' benchmark/deploy/stroppy-manifest.yaml
```

(!) Если планируется запуск примеров из папки doc/examples, то замену следует проводить в соответствующей папке, например:

```sh
sed -i 's/registry.github.com\/picodata\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage/stroppy-manifest.yaml
```

6.Скопировать ключ private_key.pem в требуемую рабочую папку

```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```

7.Запустить развертывание кластера, например, с конфигурацией из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

8.В случае ошибок развертывания, которые не устраняются переповтором, и мешают выводу консоли управления, нужно удалить кластер вручную, выполнив в корне папки с выбранной конфигурацией команду, например:

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

2.Скопировать репозиторий.

```sh
ssh-keygen -F gitlab.com || ssh-keyscan gitlab.com >> ~/.ssh/known_hosts
git clone git@github.com:picodata/stroppy.git
```

3.Скомилировать Stroppy.

```sh
make all
```

4.Далее можно продолжить с шага 3 [Сборки контейнера без компиляции](#сборка-контейнера-без-компиляции)

# Деплой stroppy в minikube
**1. Подготовка окружения**

установим minikube
```sh
curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 && chmod +x minikube
sudo mkdir -p /usr/local/bin/
sudo install minikube /usr/local/bin/
minikube version
```

установим kubectl
```sh
curl -LO https://storage.googleapis.com/kubernetes-release/release/`curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt`/bin/linux/amd64/kubectl
chmod +x ./kubectl
sudo mv ./kubectl /usr/local/bin/kubectl
kubectl version --client
```

настроим minikube
```sh
minikube config set memory 6144
minikube config set cpus 4
```

запустим minikube
```sh
minikube start
```

**2. скачаем репозиторий stroppy и соберём его**

Клонируем репозиторй stroppy и произведём подготовку к разворачиванию
```sh
git clone https://github.com/picodata/stroppy.git && cd stroppy
make all
chmod +x ./docs/examples/deploy-minikube-local/databases/postgres/deploy_operator.sh
```

Стартуем кластер, в данном случае мы используем postgres
```sh
kubectl apply -f docs/examples/deploy-minikube-local/cluster/stroppy-secret.yaml
kubectl apply -f docs/examples/deploy-minikube-local/cluster/stroppy-manifest.yaml
./docs/examples/deploy-minikube-local/databases/postgres/deploy_operator.sh
```

Проверяем как поднялся кластер, все ли поды перешли в состояние Running
```sh
minikube status && kubectl get pods && kubectl cluster-info
```
Если всё с кластером хорошо и он работает, должны увидеть нечто подобное
```
minikube
type: Control Plane
host: Running
kubelet: Running
apiserver: Running
kubeconfig: Configured

NAME                                READY   STATUS    RESTARTS   AGE
acid-postgres-cluster-0             1/1     Running   0          15m
acid-postgres-cluster-1             1/1     Running   0          14m
acid-postgres-cluster-2             1/1     Running   0          14m
postgres-operator-c8d5c8649-jqlbf   1/1     Running   0          16m
stroppy-client                      1/1     Running   0          16m
Kubernetes control plane is running at https://192.168.49.2:8443
CoreDNS is running at https://192.168.49.2:8443/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

Подключимся к контейнеру stroppy
```sh
kubectl exec --stdin --tty stroppy-client -- /bin/bash
```

Запустим тест
```sh
stroppy pop --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --count 5000 --run-as-pod --kube-master-addr=8.8.8.8  --dir .
stroppy pay --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --check --count=100000 --run-as-pod --kube-master-addr=8.8.8.8  --dir .
```

# Команды

## Общие базовые ключи для всех команд

```log-level``` - уровень логирования. Поддерживается trace, debug, info, warn, error, fatal, panic;  
```dbtype``` - наименование тестируемой СУБД. Поддерживается postgres (PostgreSQL), fdb(FoundationDB), mongodb(MongoDB), cocroach(cockroach);  
```url``` - строка подключения к тестируемой БД.
  
## Ключи команды развертывания кластера (```deploy```)

```cloud``` - имя выбранного облачного провайдера. Поддерживается yandex и oracle;  
```flavor``` - конфигурация из файла templates.yaml. Поддерживается small, standard, large, xlarge, xxlarge, maximum;  
```nodes``` - кол-во нод кластера виртуальных машин. Поддерживается ввод только числового значения. При указании обратите внимание на пункт 1 из раздела "Особенности использования";  
```dir``` - директория с файлами конфигурации.

**Пример запуска развертывания кластера в облаке**:

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

## Базовые ключи для команд запуска тестов
  
```count, n``` - кол-во загружаемых счетов, по умолчанию 100000;  
```workers, w``` - кол-во воркеров нагрузки (потоков-горутин), по умолчанию 4 * runtime.NumCPU();  
```banRangeMultiplier, r``` - коэффициент, определяющий сооотношение BIC/BAN в процессе генерации, подробности ниже;  
```stat-interval, s``` - интервал сбора статистики, по умолчанию 10 секунд;  
```pool-size``` - размер пула соединений к БД. Актуально для PostgreSQL, MongoDB и CocroachDB. Если ключ не задан, то размер пула равен кол-ву воркеров.
Для PostgreSQL и CocroachDB размер пула также может быть задан через параметр ```max_pool_size``` в строке подключения. В этом случае параметр ```pool-size``` игнорируется.

***Важное замечание***:

```banRangeMultiplier``` (далее brm) - это число, определяющее соотношение BAN (Идентификационный номер банка) к BIC (Идентификационный код банка).
Количество сгенерированных битов приблизительно равно квадратному корню из кол-ва счетов (параметр ```count```).  
Количество BAN определяется по следующей формуле:  Nban = (Nbic *brm)/square(count).  
Если Nban* Nbic > count, мы генерируем больше комбинаций (BIC, BAN), чем мы сохранили во время процесса заполнения БД (это достигается, если brm > 1).  
Рекомендуемый диапазон brm составляет от 1,01 до 1,1. Увеличение снижает кол-во not found на тесте переводов, но увеличивает кол-во dublicates на этапе загрузки счетов.  
Значение по умолчанию для параметра banRangeMultipluer равно 1.1.  
  
**Пример команды запуска теста загрузки счетов**:  

```sh
./bin/stroppy pop --url fdb.cluster --count 5000 --w 512 --dbtype=fdb
```  
  
Дополнительные ключи для команды ```pop```:  
```sharded``` - флаг использования шардирования при создании схемы данных. Актуально только для MongoDB, по умолчанию false;  

**Пример команды запуска теста переводов**:  

```sh
./bin/stroppy pay --url fdb.cluster --check --count=100000
```

Дополнительные ключи для команды ```pay```:  
```zipfian``` - флаг использования распределения данных по закону Ципфа, по умолчанию false;
```oracle``` - флаг внутренней проверки переводов. Пока не используется, указан для совместимости  
```check``` - флаг проверки результатов теста. Суть проверки - подсчет суммарного баланса счетов после теста и сравнение этого значения с сохраненным суммарным балансом после теста загрузки счетов. По умолчанию true. 

## Базовые ключи для команд запуска хаос-тестов
  
```kube-master-addr``` - внутренний ip-адрес мастер-ноды развернутого kubernetes-кластера.  
```chaos-parameter``` - имена файлов сценариев chaos-mesh, расположенных в папке deploy/databases/```имя тестируемой СУБД```/chaos. Указывается без расширения .yaml  
  
# Сценарий тестирования

Для того чтобы иметь возможность проверять как корректность менеджера транзакций, так и его производительность, нагрузочный тест имитирует серию банковских денежных переводов между счетами.
Ключевая идея, которая делает этот тест полезным для проверки целостности данных без прибегания к оракулу(то есть, без сравнения с каноническим результатом), заключается в том, что никакие денежные переводы не могут изменить общий баланс по всем счетам.  
Таким образом, тест состоит из трех основных этапов:  
  
1) Загрузка счетов. Общий баланс рассчитывается и сохраняется отдельно как канонический / ожидаемый результат.  
  
Для создания записей используется самописный генератор, который с течением времени может производить дубликаты в рамках теста. Но вставка счетов реализована таким образом, чтобы в БД сохранялись только уникальные записи и кол-во успешно загруженных записей совпадало с заданным.
  
2) Серия денежных переводов между счетами. Переводы выполняются параллельно и могут использовать одну и ту же исходную или целевую учетную запись.  
  
3) Подсчет суммарного баланса счетов и его сравнение с общим балансом, полученным на этапе загрузки счетов.  

Пример лога успешного завершения теста загрузки счетов:  

```sh
[Nov 17 15:23:07.334] Done 10000 accounts, 0 errors, 16171 duplicates 
[Nov 17 15:23:07.342] dummy chaos successfully stopped             
[Nov 17 15:23:07.342] Total time: 21.378s, 467 t/sec               
[Nov 17 15:23:07.342] Latency min/max/avg: 0.009s/0.612s/0.099s    
[Nov 17 15:23:07.342] Latency 95/99/99.9%: 0.187s/0.257s/0.258s    
[Nov 17 15:23:07.344] Calculating the total balance...             
[Nov 17 15:23:07.384] Persisting the total balance...              
[Nov 17 15:23:07.494] Total balance: 4990437743 
```

Пример лога успешного завершения теста переводов:  

```sh
[Oct 15 16:11:12.872] Total time: 26.486s, 377 t/sec             
[Oct 15 16:11:12.872] Latency min/max/avg: 0.001s/6.442s/0.314s    
[Oct 15 16:11:12.872] Latency 95/99/99.9%: 0.575s/3.268s/6.407s    
[Oct 15 16:11:12.872] dummy chaos successfully stopped             
[Oct 15 16:11:12.872] Errors: 0, Retries: 0, Recoveries: 0, Not found: 1756, Overdraft: 49 
[Oct 15 16:11:12.872] Calculating the total balance...             
[Oct 15 16:11:12.922] Final balance: 4930494048 
```

Пример окончания лога в случае расхождения итогового баланса:

```sh
Calculating the total balance...             
Check balance mismatch:
before: 748385757108.0000000000
after:  4999928088923.9300000000
```

В процессе выполнения тестов воркеры stroppy могут получать различные ошибки из-за проблем инфраструктуры или состояния СУБД. Для обеспечения устойчивости теста воркер, получивший ошибку из некоторого пула ошибок, выявленных на этапе отладки и тестов, останавливается на некоторый период (до 10 миллисекунд), увеличивает счетчик ```Retries``` - кол-во повторов, и выполняет операцию с новыми сгенерированным счетом. Пул состоит как из общих ошибок, так и специфических для тестируемой СУБД. Для изучения списка рекомендуется обратиться в [пакет payload](https://github.com/picodata/stroppy/tree/main/internal/payload).
Если воркер получает ошибку, которой нет в пуле, он останавливает свою работу с выводом в лог фатальной ошибки и увеличением счетчика ```Errors```.  

Также внутри stroppy определено несколько счетчиков для "логических" ошибок, которые является штатным поведением в общем смысле, но фиксируются отдельно от общего кол-ва операций:

```dublicates``` - кол-во операций, получивших ошибку дублирования данных. Актуально для теста загрузки счетов.  
```Not found``` - кол-во операций, завершившихся с ошибкой по причине того, что запись с переданным счетов не найдена в БД. Актуально для теста переводов.  
```Overdraft``` - кол-во операций, завершившихся с ошибкой по причине того, что баланс счета-источника недостаточен для перевода с переданной суммой. Т.е. stroppy не выполняет перевод, который может увести баланс счета-источника в минус.  

# Модель данных

На примере PostgreSQL:
| Таблица       | Столбец             | Значение и тип данных                     |
| ------------- |:------------------: | -----------------------------------------:|
| accounts      | bic                 | BIC счета, TEXT                           |
|               | ban                 | BAN счета, TEXT                           |
|               | balance             | баланс счета, DECIMAL                     |
| transfers     | transfer_id         | идентификатор перевода, UUID              |
|               | src_bic             | BIC счёта отправителя, TEXT               |
|               | src_ban             | BAN счёта отправителя, TEXT               |
|               | dst_bic             | BIC счёта получателя, TEXT                |
|               | dst_ban             | BAN счёта получателя, TEXT                |
|               | amount              | сумма перевода, DECIMAL                   |
| checksum      | name                | имя для хранения итогового баланса, TEXT  |
|               | amount              | значение итогового баланса, DECIMAL       |
| settings      | key                 | наименование настроечного параметра, TEXT |
|               | value               | значение настроечного параметра, TEXT     |

Первичным ключом таблицы accounts является пара значений BIC и BAN, таблицы transfer - transfer_id, значение которого генерируется пакетом [github.com/google/uuid](github.com/google/uuid). Для других СУБД используется аналогичная модель данных с учетом нюансов реализации самой СУБД. Также стоит отметить, что для PostgreSQL и MongoDB в методе, который выполняет перевод, реализована управление порядком блокировок для исключения дедлоков. Управление осуществляется путем лексикографического сравнения пар BIC и BAN счета-источника и счета-получателя.

# Хаос-тестирование

Использование управляемых неисправностей в stroppy реализовано с помощью [chaos-mesh](https://chaos-mesh.org/) - решения для управления хаос-тестами, которое вводит ошибки на каждом уровне системы Kubernetes.

**Пример запуска теста с использованием сценария chaos-mesh**:

```sh
./bin/stroppy pay --url fdb.cluster --check --count=100000 --kube-master-addr=10.1.20.109 --chaos-parameter=fdb-cont-kill-first
```

# Особенности использования

1. Запуск в Oracle.Cloud и Yandex.Cloud имеет различия:

- для деплоя трех машин-воркеров и одного мастера в yandex.cloud указываем nodes=3,
в Oracle.Cloud = 4, т.е. для деплоя в Oracle Cloud мастер учитывается в кол-ве создаваемых нод, в случае с Yandex.Cloud создается по умолчанию.
- в деплое Oracle.Cloud есть дополнительный шаг - монтирование отдельных network storages по протоколу ISCSI. В Yandex.Cloud используются локальные диски виртуальных машин.

**Oracle.Cloud имеет особенность, причины которой пока не установлены: при ручном удалении кластера через GUI нужно явно удалить block volumes в соответствующем разделе. Вместе с кластером они могут НЕ УДАЛИТЬСЯ!!!**

2. Для запуска тестов FoundationDB предварительно необходимо скопировать содержимое файла или сам файл fdb.cluster, расположенный в директории /var/dynamic-conf внутри пода sample-cluster-client (имя пода может иметь дополнительный цифро-буквенный постфикс), и  вставить его в директорию /root/ внутри пода stroppy-client. Это необходимо для доступа к кластеру и, на текущий момент, пока не автоматизировано.

3. Архив с графиками мониторинга создается на локальной машине, в директории monitoring/grafana-on-premise каталога с конфигурационными файлами. Среднее время создания архива - 30 минут (для Yandex больше, для Oracle меньше). Создание архива происходит после окончания любого из тестов.

4. Статистика status json для FoundationDB собирается в файл, который лежит внутри пода stroppy в кластере k8s, в директории /root/, имя файла генерится по маске status_json_время_старта_cбора_статистики.json. Сбор статистики запускается перед тестом и завершается вместе с его окончанием. Пока сбор статистики реализован только для FoundationDB, в дальнейшем может быть реализована поддержка сбора специфической статистики для других СУБД. Файлы статистики хранятся внутри пода stroppy, их копирование на рабочую машину пока не автоматизировано.

5. Для развертывания нескольких кластеров в облаке с одной локальной машины рекомендуется сделать несколько копий репозитория stroppy со своими директории файлов конфигурации. Это позволит избежать наложений и гибко управлять каждым из кластеров.

