#!/bin/bash

function _run () {
    if [ "$#" -lt 2 ]; then
      echo "run: acquired incompatible parameters count, aborting"
      exit 1
    fi

    skip_errexit=$1
    preamble=$2

    rc=
    echo "now starting '$preamble'"
    if eval "${@:3}"; then
        echo "'$preamble' step success"
    else
        rc=$?
        echo -e "'$preamble' step failed with exit code $rc\n\n\n"
        if [ "$skip_errexit" == "no_skip" ]; then
          exit $rc
        fi
    fi
}

function run() {
    _run "no_skip" "$@"
}

function errorless_run () {
  _run "skip" "$@"
}
