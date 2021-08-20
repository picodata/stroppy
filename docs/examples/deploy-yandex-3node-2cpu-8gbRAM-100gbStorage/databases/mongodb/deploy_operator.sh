
function run () {
    echo "now run '$1'"
    if eval "$1"; then
        echo "success"
    else
        exit $?
    fi

}

run "kubectl apply -f deploy/clusterwide"
run "kubectl apply -f config/crd/bases/mongodbcommunity.mongodb.com_mongodbcommunity.yaml"
run "kubectl apply -k config/rbac --namespace mongodbcommunity"
run "kubectl create -f config/manager/manager.yaml --namespace mongodbcommunity"

