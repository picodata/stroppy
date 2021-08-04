package deployment

const (
	configFileName   = "test_config.json"
	statJsonFileName = "status_json.json"

	interactiveUsageHelpTemplate = `
Started ssh tunnel for kubernetes cluster and port-forward for monitoring.
To access Grafana use address localhost:%v.
To access to kubernetes cluster in cloud use address localhost:%v.
Enter "quit" to exit stroppy and destroy cluster.
Enter "postgres pop" to start populating PostgreSQL with accounts.
Enter "postgres pay" to start transfers test in PostgreSQL.
Enter "fdb pop" to start populating FoundationDB with accounts.
Enter "fdb pay" to start transfers test in FoundationDB.
To use kubectl for access kubernetes cluster in another console 
execute command for set environment variables KUBECONFIG before using:
"export KUBECONFIG=$(pwd)/config"`
)
