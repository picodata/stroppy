minikube addons enable metallb
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/v0.31.1/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml
kubectl apply -f https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/v0.31.1/config/samples/deployment.yaml

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
kubectl apply -f "$SCRIPT_DIR/cluster_with_client.yaml"

echo "Waiting foundationdb deploy for 5 seconds..."
sleep 5
