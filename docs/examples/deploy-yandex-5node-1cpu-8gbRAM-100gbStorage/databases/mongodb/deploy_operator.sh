
function run () {
    echo "now run '$1'"
    if eval "$1"; then
        echo "success"
    else
        exit $?
    fi

}

run "kubectl apply -f bundle.yaml"
run "kubectl apply -f cr.yaml"


