
function run () {
    echo "now run '$1'"
    if eval "$1"; then
        echo "success"
    else
        exit $?
    fi

}

run "kubectl apply -f mongodb/bundle.yaml"
run "kubectl apply -f mongodb/cr.yaml"


