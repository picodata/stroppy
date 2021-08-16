#!/usr/bin/env bash

set -e

VERSION=`git rev-list --tags --max-count=1`
if [ "$VERSION" == "" ]; then
  VERSION=`git branch --show-current`
fi

echo $VERSION
