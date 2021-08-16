package deployment

const (
	configFileName = "test_config.json"

	interactiveUsageHelpTemplate = `
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:%d.
To access to kubernetes cluster in cloud use address localhost:%d.
Enter "quit" to exit stroppy and destroy cluster.
Enter "pop" to start populating PostgreSQL with accounts.
Enter "pay" to start transfers test in PostgreSQL.
Enter "pop" to start populating FoundationDB with accounts.
Enter "pay" to start transfers test in FoundationDB.
To use kubectl for access kubernetes cluster in another console 
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"`
)
