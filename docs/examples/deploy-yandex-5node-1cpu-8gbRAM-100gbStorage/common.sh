#!/bin/bash

function run () {
    if [ "$#" -lt 2 ]; then
      echo "run: acquired incompatible parameters count, aborting"
      exit 1
    fi

    rc=
    echo "now starting '$1'"
    if eval "${@:2}"; then
        echo "'$1' step success"
    else
        rc=$?
        echo -e "'$1' step failed with exit code $rc\n\n\n"
        exit $rc
    fi
}
