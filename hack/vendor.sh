#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli v1.2.0
clone git github.com/Sirupsen/logrus v0.10.0
clone git github.com/go-check/check v1
clone git github.com/stretchr/testify v1.1.3
clone git github.com/davecgh/go-spew master
clone git github.com/pmezard/go-difflib master
clone git github.com/docker/docker master
clone git github.com/docker/distribution master
clone git github.com/opencontainers/runc master
clone git github.com/mtrmac/gpgme master

clean

mv vendor/src/* vendor/
