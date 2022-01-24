- [Introduction](#introduction)
- [Main features](#main-features)
- [Example of usage](#example-of-usage)
- [Compilation and build](#compilation-and-build)
- [Commands](#commands)
- [Test scenario](#test-scenario)
- [The data model](#the-data-model)
- [Chaos testing](#chaos-testing)
- [Usage features](#usage-features)

## Introduction

[Stroppy](http://github.com/picodata/stroppy) is a framework for testing various databases. It allows you to deploy a cluster in the cloud, run load tests and simulate, for example, network unavailability of one of the nodes in the cluster.

How does all this allow you to test reliability? The fact is that there is a very elegant "banking" test to verify the integrity of the data. We fill the database with a number of records about certain "accounts" with money. Then we simulate a series of transfers from one account to another within the framework of transactions provided by the DBMS. As a result of any number of transactions, the total amount of money in the accounts should not change.

To complicate the task for the DBMS, Stroppy may try to break the DB cluster, because in the real world failures happen much more often than we want. And for horizontally scalable databases, this happens even more often, since a larger number of physical nodes gives more points of failure.

At the moment, we have implemented support for FoundationDB, MongoDB, CockroachDB and PostgreSQL (for comparison with other DBMS from the list).
In addition, in order to make it easier to analyze test results, stroppy is integrated with Grafana and after each run automatically collects an archive with monitoring data scaled by run time. Also, for FoundationDB and MongoDB, we have implemented support  internal statistic collecting with a specified frequency - for FoundationDB, data from the status json console command is collected, for MongoDB, data from the db.serverStatus() command is collected.

***Important***:
This instruction is relevant for use on Ubuntu OS >=18.04 and has not yet been tested on other operating systems.

## Main features

- Deployment of a cluster of virtual machines in the selected cloud via terraform. Yandex.Cloud and Oracle are supported.Cloud
- Deploying kubernetes cluster in a deployed cluster of virtual machines
- Deployment of the selected DBMS in this cluster
- Collecting statistics from Grafana k8s cluster metrics and system metrics of virtual machines (CPU, RAM, storage, etc.)
- Managing the parameters of tests and the deployment itself - from the number of VMs to the load supplied and managed problems
- Running tests on command from the console
- Logging of the test progress - current and final latency and RPS
- Deleting a cluster of virtual machines
- Deployment of multiple clusters from a single local machine with isolated monitoring and a startup console

## Example of usage

For example, we want to check how much load a FoundationDB cluster consisting of 3 nodes with 1 core and 8 GB of RAM per node will withstand, while the cluster will be deployed by the corresponding [k8s operator](https://github.com/FoundationDB/fdb-kubernetes-operator).

Preliminary actions:

1) Clone a repository to a local machine:

```sh
git clone git@github.com:picodata/stroppy.git
```

2) Select the directory with the most convenient configuration for us from the ```docs/examples``` folder,and copy it to a separate folder. Or use the folder with the example itself, but this option is not recommended, because it increases the likelihood of configuration errors in the future.

3) To start the cluster deployment in the cloud, the root directory with the configuration files (point 2 above) must have:

- for Yandex.Cloud:

- private and public keys. Be sure to name them ```id_rsa``` and ```id_rsa.pub``` to avoid problems. You can create keys using the ssh-keygen utility.
- a file with credentials and attributes for accessing the cloud, it is better to name it ```main.tf```, for guaranteed compatibility

- for the Oracle.Cloud:

- private key with name ```private_key.pem```. This key must be obtained using the provider's web interface.
  
To test the selected configuration using stroppy, you can go two different ways:
  
1) Manual test run - Deploy virtual machines, k8s cluster and DBMS manually, raise the stroppy pod from the manifest next to it, put the fdb.cluster file in the stroppy pod root directory and start loading accounts and then the money transfers test using the commands from [Commands](#commands).

2) Automatic deployment and launch of tests
  
- Set the necessary parameters through the appropriate configuration files:

templates.yaml - here you can set templates for virtual machine parameters of a future cluster of virtual machines in the cloud. Each provider has its own file and for the convenience of configuration, several basic configuration various are specified in the files, from ```small``` to ```maximum```.

For example :

```yaml
oracle:
  small:
  - description: "Minimal configuration: 2 CPU VM.Standard.E3.Flex, 10 Gb RAM, 50 Gb disk"
  - platform: "standard-v2" #Virtual machine type. Relevant for Yandex.Cloud
  - cpu: 2 #number of cpu allocated to a virtual machine, in cores
  - ram: 10 #the amount of RAM allocated to the virtual machine, in GB
  -  disk: 50 #объем диска, выделенного виртуальной машине, в ГБ
```

For our task, we will choose the ```small``` configuration as the most suitable.

***Important***: we left the CPU count the same, because this type of VM is in Oracle.Cloud, like similar ones, uses processors with multi-threading, and k8s, in this case, when evaluating the specified limits and requests, focuses on the number of virtual cores (threads), and not on physical ones. Therefore, by specifying cpu:2, we actually got 4 virtual cores, 2 of which we will give to FoundationDB.

- Then we configure the parameters for future tests, for this we will need the test_config file.json - here the parameters for running the tests themselves are set:

Example of the ```test_config file.json``` below the text. The name and purpose of the parameters are the same as the parameters for running tests from the section [Commands](#commands).

```json
{
  "log_level": "info", 
  "banRangeMultiplier": 1.1,
  "database_type": [ 
    "fdb" 
  ],
  "cmd": [
    {
      "pop": { // parameters for accounts loading test
        "count": 5000 
      }
    },
    {
      "pay": { // parameters for money transfers test
        "count": 100000, 
        "zipfian": false, 
        "oracle": false, 
        "check": true 
      }
    }
  ]
}
```

- After we have prepared the configuration files, it is necessary to compile the stroppy binary file. To do this, go to the stroppy root directory inside the repository and perform steps 1 and 3 of the ["Compile Stroppy and build container" from the section Compilation and Build](#compilation-and-build#compile-stroppy-and-build-container). The build result should be a binary file named stroppy in the stroppy/bin directory.

- After successfully compiling the stroppy binary file and filling in the configuration files, we are ready to run the deployment command of our cluster. For our case, run the following command in the root directory:

Oracle Cloud:  
```./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug```

Yandex.Cloud:  
```./bin/stroppy deploy --cloud yandex --flavor small --nodes 3 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug```

A description of the command keys can be found in the section [Commands](#commands) of the current manual.

The result of executing the command after output to the console of a certain amount of debugging information and about half an hour of time should be a message like:

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

***Important***: the specified ports are the default ports for monitoring access (port 3000) and access to the k8s cluster API (6443). Since stroppy supports deploying multiple clusters on one local machine, the ports for clusters launched after the first one will be incremented.

***Important***: for the FoundationDB testing, which we planned after the successful deployment and output of the message, it is necessary to perform some manual manipulations from paragraph 2 of the section [Usage Features](#usage-features).

A console opens under the message to select commands. To run the account download test, you need to enter the command ```pop``` and wait for execution, after which we will see the console for entering commands again and will be able to enter the command ```pay``` to start the money transfers test. All commands will be executed with the parameters that we set at the configuration stage in the ```test_config.json``` file.

The result of the commands will be several files in the root directory with the configuration. For example:  

```pop_test_run_2021-10-15T16:09:51+04:00.log``` - accounts loading test logs  
```pay_test_run_2021-10-15T16:10:46+04:00.log``` - tranfers test logs
```monitoring/grafana-on-premise/fdb_pop_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz``` - archive with accounts loading test's grafana metrics (export to png files)  
```monitoring/grafana-on-premise/fdb_pay_5000_1.1_zipfian_false_2021-10-15T16_10_46.tar.gz``` - archive with transfers test's grafana metrics (export to png files)

If, instead of a message in the console, an error occurs that cannot be fixed by restarting (no more than 3 repetitions), then we run the problem with the error description in [GitHub Issues](https://github.com/picodata/stroppy/issues).  

The retry is idempotent for the VM cluster and the K8S cluster, so the replay will not create new virtual machines and the Kubernetes cluster.

***Important***: stroppy does not yet guarantee idempotency with respect to the deployment of the selected DBMS. This behavior has been left unchanged, among other things, in order to make it possible to correct the database configuration error without redeploying the entire cluster.

### Compilation and build

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

5. Run cluster deploy from folder ```docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage```

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

7. Run cluster deploy from folder ```docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage```

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

## Commands

### Common base keys for all commands

```log-level``` - logging level. Trace, debug, info, warn, error, fatal, panic are supported;  
```dbtype``` - the name of the DBMS under test. Supported by `postgres` (PostgreSQL), `fdb`(FoundationDB), `mongodb`(MongoDB), `cockroach`(CockroachDB);  
```url```- the connection string to the database under test.  

### Cluster deployment command keys (```deploy```)

```cloud``` - the name of the selected cloud provider. Supported by yandex and oracle;  
```flavor```- configuration from the templates.yaml file. Supports small, standard, large, xlarge, xxlarge, maximum;  
```nodes``` - the number of nodes of the cluster of virtual machines. Only numeric value input is supported. When specifying, pay attention to paragraph 1 from the [Usage features](#usage-features);  
```dir``` - directory with configuration files.  

**Example of command for cluster deployment in the cloud**:

```sh
./bin/stroppy deploy --cloud oracle --flavor small --nodes 4 --dir docs/examples/deploy-oracle-3node-2cpu-8gbRAM-100gbStorage --log-level debug
```

### Basic keys for test run commands

```count, n``` - number of uploaded accounts, by default 100000;  
```workers, w``` - number of load workers (threads-goroutin), by default  ```4 * runtime.NumCPU()```;  
```banRangeMultiplier, r``` -  the coefficient that determines the BIC/BAN ratio in the generation process, details below;  
```stat-interval, s``` - statistics collection interval, by default 10 seconds;  
```pool-size``` - the size of the database connection pool. Relevant for PostgreSQL, MongoDB and CocroachDB. If the key is not specified, then the pool size is equal to the number of workers.
For PostgreSQL and CocroachDB, the pool size can also be set via the `max_pool_size" parameter in the connection string. In this case, the ``pool-size"' parameter is ignored.
  
***Important***: ```ban range multiplier``` (next ```brm```) is a number that defines the ratio of BAN (Bank Identifier Number) per BIC (Bank Identifier Code). The number of generated BICs is approximately equal to the square root of 'count'.  
The count of BANs is defined by the following formula: ```Nban = (Nbic *brm)/square(count)```. If Nban* Nbic > count we generate more (BIC, BAN) combinations
than we saved during DB population process (that is achieved if brm > 1).  
The recommended range of brm is from 1.01 to 1.1. The default value of banRangeMultipluer is 1.1.  
  
**Example of command for accounts loading test**:

```sh
./bin/stroppy pop --url fdb.cluster --count 5000 --w 512 --dbtype=fdb
```

Additional keys for the ```pop``` command:  
```sharded``` - flag for using sharding strategy when creating a data schema. Relevant only for MongoDB, false by default;  

**Example of command for tranfers test**:  

```sh
./bin/stroppy pay --url fdb.cluster --check --count=100000
```
  
Additional keys for the ```pay``` command:  
```zipfian``` - flag for using data distribution according to Zipf's law, false by default;  
```check``` - flag for checking the test results. The essence of the check is to calculate the total balance of accounts after the test and compare this value with the saved total balance after the test of loading accounts. By default, true.  
  
### Basic keys for commands to run chaos tests

```kube-master-addr``` - the internal ip address of the master node of the deployed kybernetes cluster.
```chaos-parameter``` - the names of the chaos-mesh script files located in the deploy/databases/ folder```name of the DBMS under test``/chaos. Specified without extension .yaml

## Testing scenario

In order to be able to check both the correctness of the transaction manager and its performance, the load test simulates a series of bank money transfers between accounts.

The key idea that makes this test useful for checking the integrity of data without referring to the oracle (that is, without comparing with the canonical result) is that no money transfers can change the total balance of all accounts.

Thus, the test consists of three main stages:

1) Accounts loading. The total balance is calculated and stored separately as a canonical/expected result.

To create records, a self-written generator is used, which over time can produce duplicates within the test. But the insertion of accounts is implemented in such a way that only unique records are stored in the database and the number of successfully uploaded records coincides with the specified one.

2) A series of money transfers between accounts. Transfers are made in parallel and can use the same source or target account.

3) Calculation of the total balance of accounts and its comparison with the total balance obtained at the stage of loading accounts.

Example of the log of successful completion of the accounts loading test:

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

Example of the log of successful completion of the money transfers test:

```sh
[Oct 15 16:11:12.872] Total time: 26.486s, 377 t/sec             
[Oct 15 16:11:12.872] Latency min/max/avg: 0.001s/6.442s/0.314s    
[Oct 15 16:11:12.872] Latency 95/99/99.9%: 0.575s/3.268s/6.407s    
[Oct 15 16:11:12.872] dummy chaos successfully stopped             
[Oct 15 16:11:12.872] Errors: 0, Retries: 0, Recoveries: 0, Not found: 1756, Overdraft: 49 
[Oct 15 16:11:12.872] Calculating the total balance...             
[Oct 15 16:11:12.922] Final balance: 4930494048 
```

Example of the end of the log in case mismatch final and total balance:

```sh
Calculating the total balance...             
Check balance mismatch:
before: 748385757108.0000000000
after:  4999928088923.9300000000
```

During the execution of tests, stroppy workers may receive various errors due to infrastructure problems or the state of the DBMS.To ensure the stability of the test, a worker who receives an error from a certain pool of errors identified at the debugging and testing stage stops for a certain period (up to 10 milliseconds), increases the counter ```Retries``` - the number of repetitions, and performs an operation with a newly generated account. To study the list of retryable errors, it is recommended to watch [payload package](https://github.com/picodata/stroppy/tree/main/internal/payload).
If the worker receives an error that is not in the pool, he stops his work with the output of a fatal error to the log and an increase in the counter ```Errors```.

Also, several counters are defined inside stroppy for "logical" errors, which are standard behavior in the general sense, but are register separately from the total number of operations:  
```dublicates``` - the number of operations that received a data duplication error. Relevant for the accounts loading test.  
```Not found``` -  the number of operations that ended with an error due to the fact that the record with the this accounts was not found in the database. Relevant for the money transfers test.  
```Overdraft``` - the number of transactions that ended with an error due to the fact that the source account balance is insufficient for the transfer with the this amount. I.e. stroppy does not perform a transfer that can take the source account balance into the negative.  

## The data model

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

The primary key of the accounts table is a pair of BIC and BAN values, primary key for the transfer table is transfer_id, the value of which is generated by the package [github.com/google/uuid](https://github.com/google/uuid). For other DBMS, a similar data model is used, taking into account the nuances of the implementation of the DBMS itself. It is also worth noting that for PostgreSQL and MongoDB, the method that performs the transfer implements lock order control to exclude deadlocks. Management is carried out by lexicographic comparison of the BIC and BAN pairs of the source account and the destination account.

## Chaos testing

The use of controlled faults in stroppy is implemented using [chaos-mesh](http://chaos-mist.org) is a chaos test management solution that introduces errors at every level of the Kubernetes system.

**Example of running a test using the chaos-mesh script**:

```sh
./bin/stroppy pay --url fdb.cluster --check --count=100000 --kube-master-addr=10.1.20.109 --chaos-parameter=fdb-cont-kill-first
```

## Usage Features

1. Launch in Oracle.Cloud and Yandex.Cloud have differences:

- for the deployment of three worker machines and one master in yandex.cloud, specify nodes=3,
in Oracle.Cloud is 4, i.e. for deployment in Oracle Cloud, the master is taken into account in the number of nodes created, in the case of Yandex.Cloud, it is created by default.
- in the Oracle deployment.Cloud there is an additional step - mounting individual network stores over the ISCSI protocol. Yandex.Cloud uses local disks of virtual machines.

**Oracle.Cloud has a feature, the reasons for which have not yet been established: when manually deleting a cluster via the GUI, you need to explicitly delete block volumes in the corresponding section. Together with the cluster, they may NOT BE DELETED!!!**

2. To run the FoundationDB tests, you first need to copy the contents of the file or the fdb.cluster file itself, located in the /var/dynamic-conf directory inside the sample-cluster-client pod (the pod name may have an additional alphanumeric postfix), and paste it into the /root/ directory inside the stroppy-client pod. This is necessary to access the cluster and, at the moment, is not automated yet.

3. An archive with monitoring metrics is created on the local machine, in the monitoring/grafana-on-premise directory of the directory with configuration files. The average archive creation time is 30 minutes (more for Yandex, less for Oracle). The archive is created after the end of any of the tests.

4. The ```status json``` statistics for FoundationDB are collected in a file that lies inside the stroppy pod in the k8s cluster, in the /root/ directory, the file name is generated by the ```status_json_mask_start_core_statistics.json```. Statistics collection starts before the test and ends with its completion. While statistics collection is implemented only for FoundationDB, support for collecting specific statistics for other DBMS may be implemented in the future. Statistics files are stored inside the stroppy pod, their copying to the working machine is not automated yet.

5. To deploy multiple clusters in the cloud from one local machine, it is recommended to make several copies of the stroppy repository with its own configuration file directories. This will avoid overlaps and flexibly manage each of the clusters.
