#!/bin/bash

function run () {
    echo "now run '$1'"
    if eval "$2"; then
        echo "step success"
    else
        exit $?
    fi
}
