SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
kubectl delete -f "$SCRIPT_DIR/cluster_with_client.yaml"

kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml
kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml
kubectl delete -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml
kubectl delete -f https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/master/config/samples/deployment.yaml
