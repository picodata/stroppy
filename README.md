# Stroppy

- [Introduction](#introduction)
- [Main features](#main-features)
- [Start the Stroppy](#start-the-Stroppy)
  - [Startup options](#startup-options)
  - [Terraform parameters](#terraform-parameters)
  - [Kubespray files](#kubespray-files)
  - [Database and infrastructure manifests](#database-and-infrastructure-manifests)
  - [Test configuration](#test-configuration)
  - [Run tests](#run-tests)
    - [Stroppy in docker](#stroppy-in-docker)
    - [Build from source](#build-from-source)
    - [Stroppy deploy](#stroppy-deploy)
  - [Test results](#test-results)
- [Deploy stroppy in minikube](#deploy-stroppy-in-minikube)
  - [Environment preparation](#environment-preparation)
  - [Building Stroppy](#building Stroppy)
- [Commands](#commands)
  - [Basic options](#base-options)
  - [Deploy options](#deploy-options)
  - [Basic pop and pay keys](#basic-pop-and-pay-keys)
  - [Basic chaos test keys](#basic-chaos-test-keys)
- [Test scenario](#test-scenario)
- [The data model](#the-data-model)
- [Managed Faults](#managed-faults)
- [Special notice](#special-notice)

---
## Introduction

[Stroppy](http://github.com/picodata/stroppy) is a framework for
testing various types of databases. It allows you to deploy a cluster in
the cloud, run load tests and simulate different failures, such as
network unavailability of one of the nodes in the cluster.

To complicate the task for the DBMS, Stroppy may try to deliberately
break the DB cluster, because in the real world failures happen much
more often than we want. And for horizontally scalable databases this
happens even more often, since a larger number of physical nodes gives
more points of failure.

At the moment, we have implemented support for FoundationDB, MongoDB,
CockroachDB and PostgreSQL (which we use a system-wide measure to
compare everything else with). In addition, Stroppy makes it easier to
analyze test results since it is integrated with Grafana. After each run
it automatically collects an archive with the database metrics, which
are scaled by the time of running. Also, you can collect even more
statistics with the desired frequency. In particular, Stroppy collects
the following data for FoundationDB and MongoDB: for FoundationDB it is
the ‘status json’ console command output, and for MongoDB it is the
‘db.serverStatus()’ command output.

---
> **Note:** This instruction is relevant for use on Ubuntu OS >=18.04 and 
> has not yet been tested on other operating systems.
---

## Main features

- Deployment of a cluster of virtual machines in the selected cloud via
  Terraform. Supported options are Yandex.Cloud and Oracle.Cloud.
- Deployment of a Kubernetes cluster inside a running cluster of virtual
  machines.
- Deployment of the selected DBMS in a running cluster.
- Collecting statistics from Grafana k8s cluster metrics and system
  metrics of virtual machines (CPU, RAM, storage, etc.).
- Managing test parameters and the deployment in general - from the
  number of VMs to the supplied load and managed problems.
- Running tests on demand from CLI.
- Logging of the test progress - current and final latency, and RPS.
- Deleting a cluster of virtual machines.
- Deployment of multiple clusters from a single local machine with
  isolated monitoring and a startup console.

---
## Start the Stroppy

For example, we want to check how much load a FoundationDB cluster
consisting of 3 nodes with 1 core and 8 GB of RAM per node will
withstand, while the cluster will be deployed by the corresponding [k8s
operator](https://github.com/FoundationDB/fdb-kubernetes-operator).

---

### Startup options

To test the selected configuration with Stroppy we can
go two different ways:
- Run tests manually.
    - Deploy virtual machines.
    - Deploy k8s cluster and DBMS manually.
    - Build and run a Stroppy pod using the manifest.
    - Attach the database connection file to the pod (if required by the database).
    - Attach the test configuration file to the pod.
    - Start downloading accounts and then test transactions using the
      commands from [Commands](#commands).
- Run tests and deployment automatically.
    - Configure the infrastructure if necessary (or skip this step)
    - Configure test parameters, or pass them as CLI flags at startup.

Next, regardless of the option chosen you need to set the necessary
startup options via appropriate command line flags. Also make sure you
have prepared the following configuration files.

---

### Terraform parameters

The file `third_party/terraform/vars.tf` is used by `terraform` to read
script variables. Changing the variables in this file will increase
or reduce the number of virtual machines, resources allocated to each
virtual machine, number of `control plane` cluster nodes, settings
service account, etc.

For example, this configuration will result in a cluster with three kubelet
nodes each of which will have 2 CPUs and 8 GB of RAM.
```terraform
variable "workers_count" {
    type = number
    description = "Yandex Cloud count of workers"
    default = 3
}

variable "workers_cpu" {
    type = number
    description = "Yandex Cloud cpu in cores per worker"
    default = 2
}

variable "workers_memory" {
    type = number
    description = "Yandex Cloud memory in GB per worker"
    default = 8
}
```

Also, it is not necessary to set parameters in `vars.tf`, you can
just set environment variables to be read by `terraform`.
```shell
export TF_VAR_workers_count=3
export TF_VAR_workers_cpu=2
export TF_VAR_workers_memory=8
```

---
> **Note:** A list of variables can be found in [tfvars](third_party/terraform/tfvars)
---

The file `third_party/terraform/main.tf` is a script for creating 
infrastructure for `terraform`. Not required. `Stroppy` can create `hcl` 
scripts by itself.
But! If during the deployment process, stroppy detects a script, then it will 
apply exactly it without changing anything in the structure of the script.

---
> **Note:**: Please note that VMs in Oracle.Cloud, like similar ones,
> uses processors with multithreading, and k8s, in this case, when evaluating
> given limits and requests are guided by the number of virtual cores
> (threads), not physical ones. Therefore, by specifying cpu: 2, we actually got 4
> virtual cores, 2 of which we will give to FoundationDB.
---

### Kubespray files

If you wish, you can manage k8s deployment by modifying files for `ansible` 
located in the `third_party/kubespray` directory

---

### Database and infrastructure manifests

The `third_party/extra/manifests` directory contains manifests for k8s which
will be used in the deployment process. You can change them if you wish.

The manifests for deploying the databases under test are located in the 
directory
`third_party/extra/manifests/databases`.

---

### Test configuration

The file `third_party/tests/test_config.json` contains parameters for database tests.
An example file is below. The name and purpose of the parameters is the same as the parameters
to run tests from the [Commands](#commands) section.

```json
{
  "log_level": "info", 
  "banRangeMultiplier": 1.1,
  "database_type": [ 
    "fdb" 
  ],
  "cmd": [
    {
      "pop": { // block with pop test configuration
        "count": 5000 
      }
    },
    {
      "pay": { // block with pay test configuration
        "count": 100000, 
        "zipfian": false, 
        "oracle": false, 
        "check": true 
      }
    }
  ]
}
```

---
> **Note:** The file is optional, and will only be used by `Stroppy` if 
> some parameters differ from the default settings.
---

### Run tests

After we (if necessary) set the configuration infrastructure, modified database 
manifests, edited test parameters, we need to choose how exactly we want to run 
`Stroppy` itself.

---

#### Stroppy in docker

1) Run the ready-made `docker` image with the client `Stroppy`
```shell
docker run -it docker.binary.picodata.io/stroppy:latest
```
2) We set the necessary `terraform` for access to the cloud of our choice for
creation of infrastructure.
- yandex
```shell
TF_VAR_token=
TF_VAR_cloud_id=
TF_VAR_folder_id=
TF_VAR_zone=
TF_VAR_network_id=
```

- oracle
```shell
TF_VAR_tenancy_ocid=
TF_VAR_user_ocid=
TF_VAR_region=
```

3) Run `Stroppy` with parameters for the database under test
```shell
stroppy deploy --cloud yandex --dbtype fdb
```

---

#### Build from source

1) Clone our repository to the local machine:

```shell
git clone https://github.com/picodata/stroppy.git
```

2) Install FoundationDB client
```shell
curl -fLO https://github.com/apple/foundationdb/releases/download/7.1.25/foundationdb-clients_7.1.25-1_amd64.deb
sudo dpkg -i foundationdb-clients_7.1.25-1_amd64.deb
```

3) Install `Ansible`
```shell
sudo apt update
sudo apt install -y python ansible
```

4) Install `Terraform`
- Add Hashicorp GPG key
```shell
wget -O- https://apt.releases.hashicorp.com/gpg | \
    gpg --dearmor | \
    sudo tee /usr/share/keyrings/hashicorp-archive-keyring.gpg
```
- Add Hashicorp repo
```shell
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] \
    https://apt.releases.hashicorp.com $(lsb_release -cs) main" | \
    sudo tee /etc/apt/sources.list.d/hashicorp.list
```
- Update `apt` cache and install `Terraform`
```shell
sudo apt update
sudo apt install -y terraform
```

5) Go to the directory with `Stroppy` and install the dependencies for the 
correct work of `Kubespray`.
```shell
cd stroppy
python3 -m venv stroppy
source stroppy/bin/activate
python3 -m pip install -f third_party/kubespray/requirements.txt
```

6) Install `kubectl`.
```shell
curl -fsLO https://storage.googleapis.com/kubernetes-release/release/v1.25.0/bin/linux/amd64/kubectl
sudo chmod +x kubectl
sudo mv kubectl /usr/bin/kubectl
```

7) Run `Stroppy` build.
```shell
sudo go build -o /usr/bin/stroppy ./cmd/stroppy
```

8) Run `Stroppy` with testing database parameters.
```shell
stroppy deploy --cloud yandex --dbtype fdb
```

---

#### Stroppy deploy

Regardless of whether we run `Stroppy` in Docker or build locally from
sources, the arguments that we can pass at startup are the same. For example:

```shell
stroppy deploy --cloud yandex --dbtype fdb
```

```shell
stroppy deploy --cloud oracle --dbtype fdb
```

---
> **Note:** A description of the command keys can be found in the 
> [Commands](#commands) section.
---

In order to deploy a `Kubernetes` cluster in the cloud infrastructure,
we need the following files in the project root directory:

**Yandex.Cloud:**
- Private and public keys. Be sure to name them like `id_rsa` and `id_rsa.pub` 
to avoid problems. You can create keys with the 
`ssh-keygen -b 4096 -f id_rsa` command.
- A file with credentials and attributes for accessing the cloud. It's better 
to name it `vars.tf`, for guaranteed compatibility.

**Oracle Cloud:**
- The `private_key.pem` provate key. This key must be obtained using the
  provider's web interface.

---

After filling the console with a certain amount of debugging information
for about 10-20 minutes, the result of the command execution should
looke like this:

```sh
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:3000.
To access to kubernetes cluster in cloud use address localhost:6443.
Enter "quit" or "exit" to exit Stroppy and destroy cluster.
Enter "pop" to start populating deployed DB with accounts.
Enter "pay" to start transfers test in deployed DB.
To use kubectl for access kubernetes cluster in another console
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"
>                               
```

The `>` prompt string means that the deployment of infrastructure,
monitoring and database was successful. Our cluster is ready for
testing. Below the message you'll find a command prompt where you can
 inpout some commands. To start the accounts loading test, enter the
 ```pop``` command and wait for its execution to complete. The result of
a successful run looks roughly like this:

```shell
[Nov 17 15:23:07.334] Done 10000 accounts, 0 errors, 16171 duplicates 
[Nov 17 15:23:07.342] dummy chaos successfully stopped             
[Nov 17 15:23:07.342] Total time: 21.378s, 467 t/sec               
[Nov 17 15:23:07.342] Latency min/max/avg: 0.009s/0.612s/0.099s    
[Nov 17 15:23:07.342] Latency 95/99/99.9%: 0.187s/0.257s/0.258s    
[Nov 17 15:23:07.344] Calculating the total balance...             
[Nov 17 15:23:07.384] Persisting the total balance...              
[Nov 17 15:23:07.494] Total balance: 4990437743 
```

After `pop` is complete, we will return to the command prompt, which
means that it's now time to enter the `pay` command and start the
transaction test. The `pay` test will run using the parameters we set at
the configuration stage in the `test_config.json` file, or with the
arguments that were provided for the `deploy` command. The result of
a successful run looks roughly like this:

```shell
[Nov 17 15:23:07.334] Done 10000 accounts, 0 errors, 16171 duplicates 
[Nov 17 15:23:07.342] dummy chaos successfully stopped             
[Nov 17 15:23:07.342] Total time: 21.378s, 467 t/sec               
[Nov 17 15:23:07.342] Latency min/max/avg: 0.009s/0.612s/0.099s    
[Nov 17 15:23:07.342] Latency 95/99/99.9%: 0.187s/0.257s/0.258s    
[Nov 17 15:23:07.344] Calculating the total balance...             
[Nov 17 15:23:07.384] Persisting the total balance...              
[Nov 17 15:23:07.494] Total balance: 4990437743 
```

---
> **Note:** The ports used in the interactive mode (the one where
> Stroppy is waiting for the next command) are the default ports to
> access monitoring (3000) and the k8s cluster API (6443). Because
> Stroppy supports deployment of multiple clusters on one local machine,
> then the ports for clusters launched after the first one will be
> incremented.

> **Note:** For the FoundationDB test case that we planned to perform
> after successful deployment, you'll need to do a few manual
> manipulations as described in paragraph 2 of the "Special notice"
> section.
---

### Test results

The commands that we mentioned above will generate several new files in
the current directory, which is supposedly the root directory of our
configuration set. Here is an example:

`pop_test_run_2021-10-15T16:09:51+04:00.log` - file with `pop` test logs.
`pay_test_run_2021-10-15T16:10:46+04:00.log` - file with `pay` test logs.  
`fdb_pop_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz` - archive with 
`pop` test metrics.
`fdb_pay_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz` - archive with 
`pay` test metrics.

If you don't get dropped into the command promt but get an error, which
 does not go away after multiple (but no more than 3) restart attempts,
 then we encourage you to file an issue in
 <https://github.com/picodata/stroppy/issues> and provide a detailed
 description of the error. You can always start the test once again by
 consequently executing `pop` and then `pay`.

A retry attempt is idempotent for VM cluster and K8S cluster, so that it
will not create any new VMs or another Kubernetes cluster.

> **Note** Stroppy is not yet guaranteed to be idempotent to a deployed
> DBMS. This behavior is explicitly preserved for fixing database
> configuration errors without redeploying the entire cluster.

---

## Deploy Stroppy in Minikube

 `Stroppy` supports running in `Minikube`, which is useful for local
testing and trying out new features.

---

### Environment preparation

1. Install `Minikube`.
```sh
curl -Lo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64 && chmod +x minikube
sudo mkdir -p /usr/local/bin/
sudo install minikube /usr/local/bin/
minikube version
```

2. Install `kubectl`
```sh
curl -fsLO https://storage.googleapis.com/kubernetes-release/release/v1.25.0/bin/linux/amd64/kubectl
sudo chmod +x kubectl
sudo mv kubectl /usr/bin/kubectl
```

3. Configure `Minikube`.
```sh
minikube config set memory 6144
minikube config set cpus 4
```

4. Run `Minikube`.
```sh
minikube start
```

---

### Building Stroppy

1. Clone `Stroppy` repository and build.
```shell
git clone https://github.com/picodata/stroppy.git && cd stroppy
make all
```

2. Deploy `Stroppy` and `Postgres`.
```sh
kubectl apply -f third_party/extra/manifests/stroppy/deployment.yml
kubectl apply -r -f third_party/extra/manifests/databases/postgres
```

3. Check how the cluster has running, whether all the pods have 
switched to the Running state.
```shell
minikube status && kubectl get pods && kubectl cluster-info
```

If everything is fine and the cluster works as it should, you will see something 
like this:
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

Use the 'kubectl cluster-info dump' command for further debug and diagnostics.
```

4. Connect to the `Stroppy` pod.
```shell
kubectl exec --stdin --tty stroppy-client -- /bin/bash
```

5. Run tests.
```shell
stroppy pop --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --count 5000 --run-type client --kube-master-addr=8.8.8.8  --dir .
stroppy pay --url postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable --check --count=100000 --run-type client --kube-master-addr=8.8.8.8  --dir .
```

---

## Commands

We recommend exploring the available options and arguments in order to
use `Stroppy` more efficiently.

---

### Basic options

`run-type` - defines the `Stroppy` startup type. If you do not need to
deploy an infrastructure, feel free to omit this option. Otherwise
provide the `client` value to run `Stroppy` as a client, Provide `local`
to run integration tests. 

`log-level` - defines the logging level. Supported values are `trace`,
`debug`, `info`, `warn`, `error`, `fatal`, `panic`.

`dbtype` - the type of the DB being tested. Supported values are `postgres` (PostgreSQL), `fdb` (FoundationDB),
`mongodb` (MongoDB), `cockroach` (cockroach), `ydb` (YandexDB). 

`url` - provide a connection string for the DB.

---

### Deploy options

`cloud` - cloud provider name. So far the supported values are `yandex` and `oracle`.  
`dir` - configuration files directory. By default it is the current directory.

**Example of running a cluster deployment in the cloud**:

```sh
stroppy deploy --cloud oracle --dir . --log-level debug
```

---

### Basic pop and pay keys

`count, n` - number of loaded accounts, default is 100000;

`workers, w` - number of load workers (goroutine threads), by default is
`4 * runtime.NumCPU()`;

`banRangeMultiplier, r` - the BIC/BAN ratio used during the generation
proces (more below);

`stat-interval, s` - statistics collection interval, default is 10 seconds;

`pool-size` - the size of the database connection pool. Applies to
PostgreSQL, MongoDB and CockroachDB. If the argument is not provided,
then the pool size is equal to the number of workers. For PostgreSQL and
CockroachDB, the pool size can also be set via the `max_pool_size`
argument in the connection string. In such case, the `pool-size`
argument is ignored.

***Important note***:

`banRangeMultiplier` (also reffered to as `brm`) is a number that determines the ratio of BAN 
 (Bank Identification Number) to BIC (Bank Identification Code).
The number of bits generated is approximately equal to the square root of the
number of accounts (the `count` parameter).
The number of BANs is calculated by the following formula:
`Nban = (Nbic *brm)/square(count)`.
If Nban* Nbic > count, then we generate more combinations (BIC, BAN) than we
saved during the database seeding process (this is achieved if `brm` > 1).
The recommended range for `brm` is 1.01 to 1.1. Larger values reduce the number of `not found` 
occurrences during the transaction test, but also produce more `duplicates` 
at the accounts downloading stage.
The default value for the banRangeMultiplier parameter is 1.1.

**An example command to run an accounts download test**:

```shell
stroppy pop --run-type client --url fdb.cluster --count 5000 --w 512 --dbtype=fdb
```

Additional options for the `pop` command:
`sharded` - enables sharding when creating a data schema.
Relevant only for MongoDB, the default is `false`.

**An example command to run a transaction test**:

```sh
stroppy pay --run-type client --url fdb.cluster --check --count=100000
```

Additional options for the `pay` command:
`zipfian` - enables data distribution according to the Zipf law, the 
default is `false`.
`oracle` - enables internal checking of transactions. Not used so far, but reserved for compatibility with `oracle`.
`check` - enables checking test results. The check implies comparing the total account balance after the test with the saved
total balance after the account loading test. The default is `true`.

---

### Basic chaos test keys

`kube-master-addr` - internal IP address of the deployed master node in the
Kubernetes cluster.
`chaos-parameter` - filenames of chaos-mesh scripts located in
folder `deploy/databases/name of DBMS under test/chaos`. Specified without the
.yaml extension.

---

## Test scenario

In order to be able to check how the correctness of the manager transactions 
and its performance, the load test simulates a series of bank money transfers 
between accounts. The key idea that makes this test is useful for checking data 
integrity without resorting to oracle (that is, without comparison with the 
canonical result), is that no money transfers can change the total balance of 
all accounts. Thus, the test consists of three main stages:

1) Loading accounts. The total balance is calculated and saved separately as
canonical/expected result.

To create records, a self-written generator is used, which over time can 
produce duplicates within the test. But inserting bills implemented in such a 
way that only unique records are stored in the database and the number of 
successfully loaded records matched the specified number.

2) A series of money transfers between accounts. Transfers run in parallel
and can use the same source or destination account.

3) Calculation of the total balance of accounts and its comparison with the total 
balance, received at the stage of loading accounts.

Below are few examples of logs that indicate the successful completion of various tests.

Accounts loading test:

```shell
[Nov 17 15:23:07.334] Done 10000 accounts, 0 errors, 16171 duplicates 
[Nov 17 15:23:07.342] dummy chaos successfully stopped             
[Nov 17 15:23:07.342] Total time: 21.378s, 467 t/sec               
[Nov 17 15:23:07.342] Latency min/max/avg: 0.009s/0.612s/0.099s    
[Nov 17 15:23:07.342] Latency 95/99/99.9%: 0.187s/0.257s/0.258s    
[Nov 17 15:23:07.344] Calculating the total balance...             
[Nov 17 15:23:07.384] Persisting the total balance...              
[Nov 17 15:23:07.494] Total balance: 4990437743 
```

Transaction test:

```shell
[Oct 15 16:11:12.872] Total time: 26.486s, 377 t/sec             
[Oct 15 16:11:12.872] Latency min/max/avg: 0.001s/6.442s/0.314s    
[Oct 15 16:11:12.872] Latency 95/99/99.9%: 0.575s/3.268s/6.407s    
[Oct 15 16:11:12.872] dummy chaos successfully stopped             
[Oct 15 16:11:12.872] Errors: 0, Retries: 0, Recoveries: 0, Not found: 1756, Overdraft: 49 
[Oct 15 16:11:12.872] Calculating the total balance...             
[Oct 15 16:11:12.922] Final balance: 4930494048 
```

Example of the final balance disrepancy:

```shell
Calculating the total balance...             
Check balance mismatch:
before: 748385757108.0000000000
after:  4999928088923.9300000000
```

While running tests, Stroppy workers may encounter various errors due to
infrastructure problems or issues with the DBMS state. To retain the
manageability of the test or debug run, Stroppy can handle certain
errors provided they are listed and described in a special pool of
errors. If so, Stroppy temporarily stops the faulty worker for a small
period of time (up to 10 ms), increments the ```Retries``` (the number
of repetitions) counter and repeats the operation with newly generated
account. The pool of known errors consists of both general errors and
errors specific to the DBMS being tested. Learn more about the error pool
by referring to the [payload
package](https://github.com/picodata/stroppy/tree/main/internal/payload).
If the worker receives an error that is not in the pool, it stops
itself, outputs the `fatal error` to the log, and increments the
`Errors` counter.

Also Stroppy has got some extra internal counters for 'logical' errors
that are considered acceptable and treated as warnings. These errors are
counted and stored apart from the general list of operations. Namely,
they are:

`duplicates` - number of operations that received a data duplication
error. Relevant for the accounts loading test. 

`Not found` - number of operations finished with an error as the record
with the transferred accounts was not found in the database. Relevant
for the transaction test. 

`Overdraft` - number of operations finished with an error as the
balance of the source account is insufficient for the transfer with the
transferred amount. That said, Stroppy will not perform a transfer that
can render the source account balance negative.

---

## The Data model

Using the example of PostgreSQL:
<table>
<thead>
<tr class="header">
<th>Table</th>
<th>Column</th>
<th>Value and data type</th>
</tr>
</thead>
<tbody>
<tr class="odd">
<td>account</td>
<td>bic</td>
<td>account BIC, TEXT</td>
</tr>
<tr class="even">
<td></td>
<td>ban</td>
<td>account BAN, TEXT</td>
</tr>
<tr class="odd">
<td></td>
<td>balance</td>
<td>account balance, DECIMAL</td>
</tr>
<tr class="even">
<td>transfers</td>
<td>transfer_id</td>
<td>transfer id, UUID</td>
</tr>
<tr class="odd">
<td></td>
<td>src_bic</td>
<td>source account BIC, TEXT</td>
</tr>
<tr class="even">
<td></td>
<td>src_ban</td>
<td>source account BAN, TEXT</td>
</tr>
<tr class="odd">
<td></td>
<td>dst_bic</td>
<td>destination account BIC, TEXT</td>
</tr>
<tr class="even">
<td></td>
<td>dst_ban</td>
<td>destination account BAN, TEXT</td>
</tr>
<tr class="odd">
<td></td>
<td>amount</td>
<td>transfer amount, DECIMAL</td>
</tr>
<tr class="even">
<td>checksum</td>
<td>name</td>
<td>name for saving of total balance, TEXT</td>
</tr>
<tr class="odd">
<td></td>
<td>amount</td>
<td>value of total balance, DECIMAL</td>
</tr>
<tr class="even">
<td>settings</td>
<td>key</td>
<td>setting parameter name, TEXT</td>
</tr>
<tr class="odd">
<td></td>
<td>value</td>
<td>setting parameter value, TEXT</td>
</tr>
</tbody>
</table>

The primary key of the `accounts` table is a pair of BIC and BAN values.
The `transfers` table has `transfer_id` as its primary key (the value is
generated by the [github.com/google/uuid](github.com/google/uuid)
package). A similar data model is used for other DBMS with applicable
adjustments. Also it's worth noting that the transaction method used for
PostgreSQL and MongoDB supports lock order management in order to exclude
deadlocks. This mechanism implies lexicographic comparison of BIC
and BAN pairs for source and recipient accounts.

---

## Managed Faults

Managed faults in Stroppy are implemented with
[chaos-mesh](https://chaos-mesh.org/), a Chaos test management solution, which 
emulates bugs at every layer of the Kubernetes system.

**An example of running a test using the chaos-mesh script**:

```shell
stroppy pay --run-type client --url fdb.cluster --check --count=100000 --kube-master-addr=10.1.20.109 --chaos-parameter=fdb-cont-kill-first
```

---

## Special notice

1. Below are some differences in launching between Oracle.Cloud and Yandex.Cloud:

- to deploy three worker machines and one master to Yandex.Cloud,
specify `nodes=3`, while with Oracle.Cloud it should be `nodes=4`. I.e.
deploying to Oracle.Cloud means that the master is counted as a regular
node, while is taken into account in the number of nodes created, in the
case of Yandex.Cloud takes care of creating the master by default.

- there is an additional step in the Oracle.Cloud deployment - mounting 
individual network storages using the iSCSI protocol. Yandex.Cloud uses local
virtual machine disks.

> **Note:** Oracle.Cloud has a quirk, ther origins of which hasn't been
> discovered yet: when manually deleting a cluster via the GUI, you must
> explicitly remove block volumes in the relevant section. They will NOT
> get removed automatically when deleting a cluster!
 

2. To run FoundationDB tests, you must first copy the `fdb.cluster` file
located in `/var/dynamic-conf` of the `sample-cluster-client` pod (the
pod name may have an additional alphanumeric postfix) into the `/root/`
directory inside the `Stroppy-client` pod. This is required to access
the cluster and, at the moment, not yet automated.

3. An archive with monitoring graphs is created on the local machine in 
 `monitoring/grafana-on-premise` inside the directory with configuration files. 
Average archive creation time is 30 minutes (more for Yandex, less for Oracle).
The archive is created after the tests are completed.

4. The `status json` statistics for FoundationDB are stored in a file
inside the Stroppy pod in the k8s cluster, specifically in the `/root/`
directory. The name of the file is generated using the
`status_json_<statistics_collection_start_time>.json`. Statistics
collection starts before the test and ends when it is completed. So far
the statistics collection is implemented only for FoundationDB, but we
plan to support other DBMS in the future as well. The statistics files
are stored inside the `Stroppy` pod, therefore you'll likely need to
manually copy it to the host machine as that is not automated yet.

5. To deploy multiple clusters in the cloud from one local machine, it
is recommended that you make multiple copies of the Stroppy repository,
each with its own directory for configuration files. This will avoid
overlaps and maintain flexible cluster management.
