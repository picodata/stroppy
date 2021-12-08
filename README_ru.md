- [Введение](#введение)
- [Основные возможности](#основные-возможности)
- [Типичный пример использования](#типичный-пример-использования)
- [Компиляция и Сборка](#компиляция-и-сборка)
- [Команды](#команды)
- [Управляемые неисправности](#управляемые-неисправности)
- [Особенности использования](#особенности-использования)

# Введение

Stroppy - это фреймворк для тестирования различных баз данных. Он позволяет развернуть кластер в облаке, запустить нагрузочные тесты и
имитировать, например, сетевую недоступность одной из нод в кластере.
Как же все это позволяет проверить надёжность? Дело в том, что для проверки целостности данных существует весьма элегантный "банковский" тест. Мы заполняем БД рядом записей о неких "счетах" с деньгами. Затем имитируем серию переводов с одного счета на другой в рамках предоставляемых СУБД транзакций. В результате любого количества транзакций общая сумма денег на счетах не должна измениться.
Чтобы усложнить задачу для СУБД, Stroppy может попытаться сломать кластер БД, ведь в реальном мире отказы случаются гораздо чаще, чем нам хочется. А для горизонтально масштабируемых БД это случается еще чаще, так как большее количество физических узлов дает больше точек отказа.
На данный момент мы реализовали поддержку FoundationDB, MongoDB, CockroachDB и PostgreSQL (нужно же с чем-то сравнивать всё остальное).

# Основные возможности  

- Автоматическое развертывание кластера виртуальных машин в выбранном облаке через terraform. Поддерживается Yandex.Cloud и Oracle.Cloud
- Автоматическое развертывание kubernetes кластера в развернутом кластере виртуальных машин
- Автоматическое развертывание выбранной СУБД в этом кластере
- Автоматический сбор статистики из Grafana метрик k8s кластера и системных метрик виртуальных машин (CPU, RAM, storage и т.д.)
- Гибкое управление параметрами тестов и самого развертывания - от кол-ва VM до подаваемой нагрузки и управляемых неполадок
- Автоматический запуск тестов по команде из консоли
- Логирование хода теста - текущее и итоговое latency и RPS
- Автоматическое удаление кластера виртуальных машин

# Типичный пример использования

Допустим, мы хотим проверить, какую нагрузку выдержит кластер FoundationDB, состоящий из 3 узлов по 1 ядру и 8 ГБ RAM на узел, при этом кластер будет развернут соответствующим [k8s оператором](https://github.com/FoundationDB/fdb-kubernetes-operator)

Предварительные действия:

1) клонируем себе репозиторий на локальную машину:
git clone git@github.com:picodata/stroppy.git
2) из папки docs/examples выбираем директорию с наиболее удобной для нас конфигурацией и копируем её в отдельную папку. Либо используем саму папку с примером, но такой вариант не рекомендуется, т.к. увеличивает вероятность ошибок конфигурования в дальнейшем.
3) Для запуска деплоя кластера в обалке в корне директории с файлами конфигурации (пункт 2 выше) должны быть:

- для Yandex.Cloud:

 1. приватный и публичный ключи. Обязательно именовать их как id_rsa и id_rsa.pub во избежание проблем. Создать ключи можно при помощи утилиты ssh-keygen.
 2. файл с credentials и атрибутами для доступа к cloud, лучше назвать его main.tf, для гарантированной совместимости

- для Oracle.Cloud:

 1. приватный ключ private_key.pem. Данный ключ необходимо получить с помощью веб-интерфейса провайдера.

Чтобы протестировать выбранную конфигурацию с помощью stroppy мы можем пойти двумя разными путями:

1) Гибко, но долго - Развернуть виртуальные машины, кластер k8s и СУБД вручную, поднять рядом под со stroppy из манифеста, подложить ему в директорию /root файл fdb.cluster и запустить загрузку счетов и затем тест переводов, воспользовавшись команды из соответствующего раздела этого документа

2) Тоже гибко, но быстрее

- [ ] Задать необходимые параметры через соответствующие файлы конфигурации:
templates.yaml - здесь задаются шаблоны параметров виртуальных машин будущего кластера виртуальных машин в облаке. Для каждого провайдера свой файл и для удобства конфигурирования в файлах указаны несколько базовых вариантов конфигурации, от small до maximum.

Например :

```yaml
oracle:
  small:
  - description: "Minimal configuration: 2 CPU VM.Standard.E3.Flex, 8 Gb RAM, 50 Gb disk"
  - platform: "standard-v2"
  - cpu: 2
  - ram: 8
  -  disk: 50
```

Она не совсем подойдет для нашей задачи, лучше добавить еще немного ресурсов, чтобы иметь запас для успешной работы k8s и самой операционный системы. Конфигурации medium и далее для нас избыточны, поэтому просто редактируем файл и выбранную конфигурацию, например:

```yaml
oracle:
  small:
  - description: "Minimal configuration: 2 CPU VM.Standard.E3.Flex, 8 Gb RAM, 50 Gb disk"
  - platform: "standard-v2"
  - cpu: 2
  - ram: 10
  -  disk: 1000
```

***Важное замечание***: мы оставили кол-во cpu таким же, т.к. этот тип VM в Oracle.Cloud, как и аналогичные ему, использует процессоры с мультитредингом, а k8s, в таком случае, при оценке заданных limits и requests ориентируется на кол-во виртуальных ядер (тредов), а не на физические. Поэтому указав cpu:2, мы фактически получили 4 виртуальных ядра, 2 из которых отдадим FoundationDB.

- [ ] Затем сконфигурируем параметры для будущих тестов, для этого нам понадобится файл  test_config.json - здесь задаются параметры запуска самих тестов

Пример файла:

```
{
  "log_level": "info", -- здесь мы задаем уровень логирования для наших тестов
  "banRangeMultiplier": 1.1, -- это коэффициент рандомизации, рекомендуемое значение от 1.01 до 1.1, по умолчанию 1.1. Уменьшение приводит к снижению ошибок вида not found при тесте переводов, но к увеличению дубликатов на этапе загрузки счетов
  "database_type": [ 
    "fdb" -- здесь мы указываем краткое название тестируемой СУБД. 
  ],
  "cmd": [
    {
      "pop": { -- здесь задаем параметры теста загрузки счетов
        "count": 5000 -- кол-во загружаемых счетов
      }
    },
    {
      "pay": { -- здесь задаем параметры теста переводов
        "count": 100000, -- кол-во переводов
        "zipfian": false, -- флаг использования распределения данных по закону Ципфа
        "oracle": false, -- флаг внутренней проверки переводов. Пока не используется, указан для совместимости
        "check": true -- флаг проверки пройденного теста. Подробность здесь.
      }
    }
  ]
}
```

- [ ] После того, как мы подготовили файлы конфигурации, необходимо скомпилировать бинарный файл stroppy. Для этого переходим в корневую директорию stroppy внутри репозитория и выполняем пункты 1 и 3 этапа "Compile Stroppy and build container" из раздела "Компиляция и сборка".
Результатом сборки должен стать бинарный файл с именем stroppy в директории stroppy/bin.

- [ ] После успешной компиляции бинарного файла stroppy и заполнения файлов конфигурации мы готовы запустить команду деплоя нашего кластера. Для нашего случая в корневой директории запускаем следующую команду:

для Oracle Cloud:
`./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug`

для Yandex.Cloud:
`./bin/stroppy deploy --cloud yandex --flavor small --nodes 3 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug`

Описание ключей команд можно найти в разделе "Команды" текущего руководства.

Результатом выполнения команды после вывода в консоль некоторого кол-ва отладочной информации и примерно получаса времени должно стать сообщение вида:

Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:3000.
To access to kubernetes cluster in cloud use address localhost:6443.
Enter "quit" or "exit" to exit stroppy and destroy cluster.
Enter "pop" to start populating deployed DB with accounts.
Enter "pay" to start transfers test in deployed DB.
To use kubectl for access kubernetes cluster in another console
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"

***Важное замечание***: Для варианта с тестированием FoundationDB, который мы запланировали после успешного деплоя и вывода сообщения необходимо выполнить немного ручных манипуляций из пункта 2 раздела "Особенности использования".

Под сообщением будет открыта консоль для выбора команд. Для старта теста загрузки счетов необходимо ввести команду pop и дождаться выполнения, после чего мы вновь увидим консоль для ввода команд и можем вввести команду pay для старта теста переводов. Все команды будут запущены с теми параметрами, которые мы задали на этапе конфигурирования в файле test_config.json.

Результатом работы команд будет несколько файлов в корне директории с конфигурацией. Например, для нашего случая:

pop_test_run_2021-10-15T16:09:51+04:00.log - файл с логами теста загрузки счетов
pay_test_run_2021-10-15T16:10:46+04:00.log - файл с логами теста переводов
monitoring/grafana-on-premise/fdb_pop_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz - архив с метриками теста загрузки счетов
monitoring/grafana-on-premise/fdb_pay_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz - архив с метриками теста переводов

Если вместо этого сообщения в консоль была выведена ошибка, имеет смысл повторить выполнение команды deploy, опыт использования показывает, что имеется некоторый пул ошибок, связанных с инфраструктурными или иными проблемами, не имеющими отношения к stroppy, по крайней мере напрямую.
Переповтор идемпотентен для кластера VM и кластера K8S, поэтому при переповторе не будут созданы новые виртуальные машины и кластер Kubernetes.

***Важное замечание***: stroppy пока не гарантирует идемпотентность в отношении развертывания выбранной СУБД. Такое поведение оставлено без изменений в том числе, чтобы дать возможность исправить ошибку конфигурирования БД без редеплоя всего кластера.
 Если же ошибка не устраняется даже при нескольких переповторах, то оптимальным вариантом будем завести issue с описанием ошибки в <https://github.com/picodata/stroppy/issues>.

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
git clone git@gitlab.com:picodata/openway/stroppy.git
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

5.Запустить тест Postgres из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

6.В случае преждевременного прерывания работы программы или разрыве соединения нужно удалить кластер вручную.

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
git clone git@gitlab.com:picodata/openway/stroppy.git
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
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' benchmark/deploy/stroppy-manifest.yaml
```

(!) Если планируется запуск примеров из папки doc/examples, то замену следует проводить в соответствующей папке, например:

```sh
sed -i 's/registry.gitlab.com\/picodata\/openway\/stroppy:latest/docker.io\/USERNAME\/stroppy:latest/g' docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage/stroppy-manifest.yaml
```

6.Скопировать ключ private_key.pem в требуемую рабочую папку

```sh
cp /path/to/private_key.pem docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage
```

7.Запустить тест Postgres из папки `docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage`

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

8.В случае преждевременного прерывания работы программы или разрыве соединения нужно удалить кластер вручную.

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
git clone git@gitlab.com:picodata/openway/stroppy.git
```

3.Откомилировать Stroppy.

```sh
make all
```

4.Далее можно продолжить с шага 3 [Сборки контейнера без компиляции](#сборка-контейнера-без-компиляции)

# Команды

Запу

# Управляемые неисправности

# Особенности использования


1. Запуск в Oracle.Cloud и Yandex.Cloud имеет различия:

- для деплоя трех машин-воркеров и одного мастера в yandex.cloud указываем nodes=3,
в Oracle.Cloud = 4, т.е. для деплоя в Oracle Cloud мастер учитывается в кол-ве создаваемых нод, в случае с Yandex.Cloud создается по умолчанию.
- в деплое Oracle.Cloud есть дополнительный шаг - монтирование отдельных network storages по протоколу ISCSI

**Oracle.Cloud имеет особенность, причины которой пока не установлены: при ручном удалении кластера через GUI нужно явно удалить block volumes в соответствующем разделе. Вместе с кластером они НЕ УДАЛЯЮТСЯ!!!**

2. Для запуска тестов FoundationDB предварительно необходимо скопировать содержимое файла или сам файл fdb.cluster, расположенный в директории /var/dynamic-conf внутри пода sample-cluster-client (имя пода может иметь дополнительный цифро-буквенный постфикс), и  вставить его в директорию /root/ внутри пода stroppy-client. Это необходимо для доступа к кластеру и, на текущий момент, пока не автоматизировано.

3. Архив с графиками мониторинга создается на локальной машине, в директории monitoring/grafana-on-premise каталога с конфигурационными файлами. Среднее время создания архива - 30 минут (для Yandex больше, для Oracle меньше). Создание архива происходит после окончания любого из тестов.

4. Статистика status json для FoundationDB собирается в файл, который лежит внутри пода stroppy в кластере k8s, в директории /root/, имя файла генерится по маске status_json_время_старта_cбора_статистики.json. Сбор статистики запускается перед тестом и завершается вместе с его окончанием. Пока сбор статистики реализован только для FoundationDB, в дальнейшем может быть реализована поддержка сбора специфической статистики для других СУБД.
