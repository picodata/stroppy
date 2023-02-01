/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

const (
	testConfDir    = "third_party/tests"
	configFileName = "test_config.json"

	interactiveUsageHelpTemplate = `
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.

Grafana:        http://%s:%s@%s:%d
Prometheus      http://%s:%d.

Enter "quit" or "exit" to exit stroppy and destroy cluster.
Enter "pop" to start populating deployed DB with accounts.
Enter "pay" to start transfers test in deployed DB.
To use kubectl for access kubernetes cluster in another console 
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"`

	stroppyBinaryPath = "/usr/local/bin/stroppy"
	stroppyHomePath   = "/home/stroppy"

	//nolint
	addToHosts = `
%s      prometheus.cluster.picodata.io
%s	    status.cluster.picodata.io
    `
)

const (
	pgDefaultURI    = "postgres://stroppy:stroppy@acid-postgres-cluster/stroppy?sslmode=disable"
	fdbDefultURI    = "fdb.cluster"
	mongoDefaultURI = "mongodb://stroppy:stroppy@sample-cluster-name-mongos" +
		".default.svc.cluster.local/admin?ssl=false"

	crDefaultURI   = "postgres://stroppy:stroppy@/stroppy?sslmode=disable"
	cartDefaultURI = "http://routers:8081"
	ydbDefaultURI  = "grpc://stroppy-ydb-database-grpc:2135/root/stroppy-ydb-database"
)
