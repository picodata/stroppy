/* Copyright 2021 The Stroppy Authors. All rights reserved         *
 * Use of this source code is governed by the 2-Clause BSD License *
 * that can be found in the LICENSE file.                          */

package deployment

const (
	configFileName = "test_config.json"

	interactiveUsageHelpTemplate = `
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:%d.
To access to kubernetes cluster in cloud use address localhost:%d.
Enter "quit" or "exit" to exit stroppy and destroy cluster.
Enter "pop" to start populating deployed DB with accounts.
Enter "pay" to start transfers test in deployed DB.
To use kubectl for access kubernetes cluster in another console 
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"`
)
