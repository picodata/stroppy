minikube addons enable metallb
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbclusters.yaml
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbbackups.yaml
kubectl apply -f https://raw.githubusercontent.com/FoundationDB/fdb-kubernetes-operator/master/config/crd/bases/apps.foundationdb.org_foundationdbrestores.yaml
kubectl apply -f https://raw.githubusercontent.com/foundationdb/fdb-kubernetes-operator/master/config/samples/deployment.yaml
kubectl apply -f cluster_with_client.yaml
