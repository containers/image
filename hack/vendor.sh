#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli v1.2.0
clone git github.com/Sirupsen/logrus v0.8.7 # logrus is a common dependency among multiple deps
clone git github.com/docker/docker master
clone git golang.org/x/net master https://github.com/golang/net.git
clone git github.com/docker/engine-api master
clone git github.com/docker/distribution v2.3.0-rc.1
clone git github.com/go-check/check v1
clone git github.com/docker/go-connections master
clone git github.com/docker/go-units master
clone git github.com/docker/libtrust master
clone git github.com/opencontainers/runc master
clone git github.com/vbatts/tar-split master
clone git github.com/gorilla/mux master
clone git github.com/gorilla/context master

clean

mv vendor/src/* vendor/
