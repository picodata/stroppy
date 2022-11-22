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
  - [Base options](#base-options)
  - [Deploy options](#deploy-options)
  - [Basic pop and pay keys](#basic-pop-and-pay-keys)
  - [Basic chaos test keys](#basic-chaos-test-keys)
- [Test scenario](#test-scenario)
- [The data model](#the-data-model)
- [Managed Faults](#managed-faults)
- [Specific of use](#specific-of-use)

---
## Introduction

[Stroppy](http://github.com/picodata/stroppy) is a framework for testing 
various databases. It allows you to deploy a cluster in the cloud, run load 
tests and simulate, for example, network unavailability of one of the nodes 
in the cluster.

How does all this allow you to test reliability? The fact is that there is a 
very elegant "banking" test to verify the integrity of the data. We fill the 
database with a number of records about certain "accounts" with money. Then 
we simulate a series of transfers from one account to another within the 
framework of transactions provided by the DBMS. As a result of any number of 
transactions, the total amount of money in the accounts should not change.

To complicate the task for the DBMS, Stroppy may try to break the DB cluster, 
because in the real world failures happen much more often than we want. And 
for horizontally scalable databases, this happens even more often, since a 
larger number of physical nodes gives more points of failure.

At the moment, we have implemented support for FoundationDB, MongoDB, 
CockroachDB, YandexDB and PostgreSQL (for comparison with other DBMS from the 
list).
In addition, in order to make it easier to analyze test results, stroppy is 
integrated with Grafana and after each run automatically collects an archive 
with monitoring data scaled by run time. Also, for FoundationDB and MongoDB, 
we have implemented support  internal statistic collecting with a specified 
frequency - for FoundationDB, data from the status json console command is 
collected, for MongoDB, data from the db.serverStatus() command is collected.

---
> **Note:** This instruction is relevant for use on Ubuntu OS >=18.04 and 
> has not yet been tested on other operating systems.
---

## Main features

- Deployment of a cluster of virtual machines in the selected cloud via 
terraform. Yandex.Cloud and Oracle are supported Clouds.
- Deploying kubernetes cluster in a deployed cluster of virtual machines.
- Deployment of the selected DBMS in this cluster.
- Collecting statistics from Grafana k8s cluster metrics and system metrics 
of virtual machines (CPU, RAM, storage, etc.).
- Managing the parameters of tests and the deployment itself - from the number 
of VMs to the load supplied and managed problems.
- Running tests on command from the console.
- Logging of the test progress - current and final latency and RPS.
- Deleting a cluster of virtual machines.
- Deployment of multiple clusters from a single local machine with isolated 
monitoring and a startup console.

---
## Start the Stroppy

For example, we want to check how much load a FoundationDB cluster consisting 
of 3 nodes with 1 core and 8 GB of RAM per node will withstand, while the 
cluster will be deployed by the corresponding [k8s operator](https://github.com/FoundationDB/fdb-kubernetes-operator).

---

### Startup options

To test the selected configuration with Stroppy we can
go two different ways:
- Run tests manually.
    - Deploy virtual machines.
    - Deploy k8s cluster and DBMS manually.
    - Raise next to Stroppy from the manifest.
    - Mount in the database connection file (if required by the database).
    - Mount to the pod file with the test configuration.
    - Start downloading invoices and then test translations using
      commands from [Commands](#commands).
- Run tests and deployment automatically.
    - Configure the infrastructure if necessary (or skip this step)
    - Configure test parameters, or pass them on the command line at startup.

Next, regardless of the option chosen, you need to set the necessary
launch options via appropriate command line flags. As well as
files prepare the following configuration files.

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

Regardless of whether we run `Stroppy` in docker or by locally building from
sources, the arguments that we can pass at startup are the same. For example:

```shell
stroppy deploy --cloud yandex --dbtype fdb
```

```shell
stroppy deploy --cloud oracle --dbtype fdb
```

---
> **Note:** A description of the command keys can be found in the section -
> [Commands](#commands)
---

In order to deploy a `Kubernetes` cluster in the cloud infrastructure in the 
root directory with the project should be:
**Yandex.Cloud:**
- Private and public keys. Be sure to name them like `id_rsa` and `id_rsa.pub` 
to avoid problems. You can create keys with command 
`ssh-keygen -b 4096 -f id_rsa`.
- A file with credentials and attributes for accessing the cloud, it's better 
to name it `vars.tf`, for guaranteed compatibility.

**Oracle Cloud:**
- Private key `private_key.pem`. This key must be obtained from
  using the provider's web interface.

---

After outputting a certain amount of debugging information to the console and passing
about 10-20 minutes, the result of the command execution should be a message like:
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

The `>` prompt string means that the infrastructure deployment,
deployment of monitoring and database was successful. Our cluster is ready for
testing. Below the message, a console will open to select commands.
To start the invoice loading test, enter the command ```pop``` and
wait for execution. The result of a successful test run will be approximately
following output:

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

After executing `pop` we will again see the console for entering commands, and 
we can enter the `pay` command to start the translation test. `Pay` test will 
be run with the parameters that we set at the configuration stage in the file
`test_config.json`, or in the arguments when running `deploy`. An example of 
a successful execution would be the output of something like this:

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
> **Note:** Specified ports in interactive mode (Stroppy mode in which it's 
> waiting for the next command) these are the default ports to access 
> monitoring (port 3000) and access to the k8s cluster API (6443). Because 
> Stroppy supports deployment of multiple clusters on one local machine,
> then the ports for clusters launched after the first one will be incremented.

> **Note:** For the FoundationDB test case we planned
> after a successful deployment and displaying a message, you need to do a little 
> manual manipulations from paragraph 2 of the "Peculiarities of use" section.
---

### Test results

The result of the commands will be several files in the root of the directory 
with configuration. For example, for our case:

`pop_test_run_2021-10-15T16:09:51+04:00.log` - file with `pop` test logs.
`pay_test_run_2021-10-15T16:10:46+04:00.log` - file with `pay` test logs.  
`fdb_pop_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz` - archive with 
`pop` test metrics.
`fdb_pay_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz` - archive with 
`pay` test metrics.

If an error occurs instead of a console message that cannot be resolved
restart (no more than 3 repetitions), then we start an issue with a description 
of the error in <https://github.com/picodata/stroppy/issues>. The test can be 
repeated by running `pop` first and then `pay`.

Retry is idempotent for VM cluster and K8S cluster, so when retry will not 
create new VMs and Kubernetes cluster.

> **Note** Stroppy is not yet guaranteed to be idempotent with respect to 
> deployment selected DBMS. This behavior is left unchanged including to give
> the ability to fix a database configuration error without redeploying the 
> entire cluster.

---

## Deploy Stroppy in minikube

For local testing of any new features, (or just for something try it) `Stroppy`
supports running in `Minikube`.

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

If everything is fine with the cluster, and it works, you should see something 
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

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.
```

4. Connect to `Stroppy` pod.
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

To use `Stroppy` effectively, you need to study the set of available
commands and options.

---

### Base options

`run-type` - `Stroppy` startup type. If you do not need deployment
infrastructure, then the option can be omitted. To run Stroppy in client, 
you need to specify `client`. To run integration tests `local`.
`log-level` - logging level. Supports trace, debug, info, warn, error, fatal, 
panic.
`dbtype` - the name of the tested DBMS. Supported by postgres (PostgreSQL), 
fdb(FoundationDB), mongodb(MongoDB), cockroach(cockroach), ydb(YandexDB).
`url` - connection string to the tested database.

---

### Deploy options

`cloud` - cloud provider name. Now supported `yandex` and `oracle`.  
`dir` - configuration files directory. By default, is a current directory.

**Example of running a cluster deployment in the cloud**:

```sh
stroppy deploy --cloud oracle --dir . --log-level debug
```

---

### Basic pop and pay keys

`count, n` - number of loaded accounts, default 100000;
`workers, w` - number of load workers (goroutine threads), by default
`4 * runtime.NumCPU()`;
`banRangeMultiplier, r` - coefficient defining the BIC/BAN ratio
in the process of generation, details below;
`stat-interval, s` - statistics collection interval, 10 seconds by default;
`pool-size` - the size of the database connection pool. Relevant for 
PostgreSQL, MongoDB and CockroachDB. If the key is not set, then the pool size 
is equal to the number of workers. For PostgreSQL and CockroachDB, the pool 
size can also be set via the parameter
`max_pool_size` in the connection string. In this case, the `pool-size` 
parameter ignored.

***Important note***:

`banRangeMultiplier` (hereinafter brm) is a number that determines the BAN 
ratio (Bank Identification Number) to BIC (Bank Identification Code).
The number of bits generated is approximately equal to the square root of
number of accounts (parameter `count`).
The number of BANs is determined by the following formula:
`Nban = (Nbic *brm)/square(count)`.
If Nban* Nbic > count, we generate more combinations (BIC, BAN) than we
saved during the database seeding process (this is achieved if brm > 1).
The recommended range for brm is 1.01 to 1.1. Increase reduces quantity
not found on the translation test, but increases the number of duplicates 
at the stage downloading invoices.
The default value for the banRangeMultiplier parameter is 1.1.

**An example command to run an invoice download test**:

```shell
stroppy pop --run-type client --url fdb.cluster --count 5000 --w 512 --dbtype=fdb
```

Additional options for the `pop` command:
`sharded` - flag for using sharding when creating a data schema.
Relevant only for MongoDB, false by default;

**An example command to run a translation test**:

```sh
stroppy pay --run-type client --url fdb.cluster --check --count=100000
```

Additional options for the `pay` command:
`zipfian` - flag for using data distribution according to Zipf's law, the 
default is `false`.
`oracle` - flag for internal checking of translations. While not in use
specified for compatibility with `oracle`.
`check` - flag for checking test results. The essence of the check is counting
total account balance after the test and comparing this value with the saved
total balance after the account loading test. The default is `true`.

---

### Basic chaos test keys

`kube-master-addr` - internal ip-address of the deployed master node
kubernetes cluster.
`chaos-parameter` - filenames of chaos-mesh scripts located in
folder `deploy/databases/name of DBMS under test/chaos`. Specified without
.yaml extensions

---

## Test scenario

In order to be able to check how the correctness of the manager transactions 
and its performance, the load test simulates a series of bank money transfers 
between accounts. The key idea that makes this test is useful for checking data 
integrity without resorting to oracle (that is, without comparison with the 
canonical result), is that no money transfers can change the total balance of 
all accounts. Thus, the test consists of three main stages:

1) Loading invoices. The total balance is calculated and saved separately as
canonical/expected result.

To create records, a self-written generator is used, which over time can 
produce duplicates within the test. But inserting bills implemented in such a 
way that only unique records are stored in the database and the number of 
successfully loaded records matched the specified number.

2) A series of money transfers between accounts. Transfers run in parallel
and can use the same source or destination account.

3) Calculation of the total balance of accounts and its comparison with the total 
balance, received at the stage of loading invoices.

An example of a log of successful completion of the invoice loading test:

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

An example of a successful translation test completion log:

```shell
[Oct 15 16:11:12.872] Total time: 26.486s, 377 t/sec             
[Oct 15 16:11:12.872] Latency min/max/avg: 0.001s/6.442s/0.314s    
[Oct 15 16:11:12.872] Latency 95/99/99.9%: 0.575s/3.268s/6.407s    
[Oct 15 16:11:12.872] dummy chaos successfully stopped             
[Oct 15 16:11:12.872] Errors: 0, Retries: 0, Recoveries: 0, Not found: 1756, Overdraft: 49 
[Oct 15 16:11:12.872] Calculating the total balance...             
[Oct 15 16:11:12.922] Final balance: 4930494048 
```

An example of the end of the log in case of a discrepancy in the final balance:
```shell
Calculating the total balance...             
Check balance mismatch:
before: 748385757108.0000000000
after:  4999928088923.9300000000
```

While running tests, Stroppy workers may get various errors due to 
infrastructure problems or the state of the DBMS. For sustainability test 
worker that received an error from some pool of errors detected at the stage 
debugging and testing, stops for a certain period (up to 10 milliseconds), 
increments the ```Retries``` counter - the number of repetitions, and performs 
the operation with new generated invoice. The pool consists of both general 
errors and specific to the tested DBMS. To study the list, it is recommended
to refer to [payload package](https://github.com/picodata/stroppy/tree/main/internal/payload).
If the worker receives an error that is not in the pool, it stops its work
with output to the log of a fatal error and increment of the `Errors` counter.

Also, inside Stroppy there are several counters for "logical" errors, which is 
regular behavior in the general sense, but are fixed separately from the total 
number of operations:

`duplicates` - number of operations that received a data duplication error.
Relevant for the invoice loading test.
`Not found` - the number of operations that ended with an error due to that 
the record with the transferred accounts was not found in the database. 
Relevant for the translation test.
`Overdraft` - the number of operations that ended with an error due to
that the balance of the source account is insufficient for the transfer with 
the transferred amount. Those. Stroppy doesn't perform transfers that can 
steal balance source account in the negative.

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

The primary key of the accounts' table is a pair of BIC and BAN values, tables
transfer - transfer_id whose value is generated by the package 
[github.com/google/uuid](github.com/google/uuid). For other DBMS, use a similar 
data model, taking into account the nuances of the implementation of the DBMS 
itself. Also, worth note that for PostgreSQL and MongoDB, in the method that 
does the translation, implemented lock order management to exclude deadlocks.
Management is carried out by lexicographic comparison of BIC and BAN pairs
source and recipient accounts.

---

## Managed Faults

The use of managed faults in Stroppy is implemented with
[chaos-mesh](https://chaos-mesh.org/) - Chaos test management solutions, which 
introduces bugs at every layer of the Kubernetes system.

**An example of running a test using the chaos-mesh script**:

```shell
stroppy pay --run-type client --url fdb.cluster --check --count=100000 --kube-master-addr=10.1.20.109 --chaos-parameter=fdb-cont-kill-first
```

---

## Specific of use

1. Launching in Oracle.Cloud and Yandex.Cloud has differences:

- to deploy three worker machines and one master in yandex.cloud, specify
nodes=3, in Oracle.Cloud = 4, i.e. for deployment to Oracle Cloud, the master 
is taken into account in the number of nodes created, in the case of 
Yandex.Cloud, it is created by default.

- there is an additional step in the Oracle.Cloud deployment - mounting 
individual network storages using the iSCSI protocol. Yandex.Cloud uses local
virtual machine disks.

> **Note:** Oracle.Cloud has a peculiarity, the reasons for which are not yet known.
> installed: when manually deleting a cluster via the GUI, you must explicitly remove
> block volumes in the relevant section. Together with the cluster
> may NOT REMOVE!!!

2. To run FoundationDB tests, you must first copy the contents of the file or 
the fdb.cluster file itself, located in the directory `/var/dynamic-conf` 
inside the sample-cluster-client pod (the pod name may have an additional 
alphanumeric postfix) and paste it into the directory `/root/` inside the 
Stroppy-client pod. This is required to access the cluster and, at the moment, 
not yet automated.

3. An archive with monitoring graphs is created on the local machine, in the 
directory `monitoring/grafana-on-premise` directory with configuration files. 
Average archive creation time - 30 minutes (more for Yandex, less for Oracle).
The archive is created after the end of the tests.

4. Statistics status json for FoundationDB is collected in a file that
lies inside the Stroppy pod in the k8s cluster, in the `/root/` directory, 
file name generated by the mask 
`status_json_statistics_collection_start_time.json`. Collection statistics 
starts before the test and ends when it ends. So far, statistics collection is 
implemented only for FoundationDB, in the future support for collecting 
specific statistics for other DBMS. The statistics files are stored inside 
the `Stroppy` pod, they're copying to a working machine is not yet automated.

5. To deploy multiple clusters in the cloud from one local machine, it is 
recommended that you make multiple copies of the Stroppy repository with your 
configuration file directories. This will avoid overlaps and flexibly manage 
each of the clusters.
