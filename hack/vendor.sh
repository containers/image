#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli master
clone git github.com/Sirupsen/logrus v0.8.7
clone git github.com/vbatts/tar-split v0.9.11
clone git github.com/gorilla/mux master
clone git github.com/gorilla/context master
clone git golang.org/x/net master https://github.com/golang/net.git
clone git github.com/go-check/check v1

clone git github.com/docker/docker 29bade2cd0a09191279f04ebc6aeedaa70c772a0
clone git github.com/docker/engine-api 7f6071353fc48f69d2328c4ebe8f3bd0f7c75da4
clone git github.com/docker/distribution 7b66c50bb7e0e4b3b83f8fd134a9f6ea4be08b57

clone git github.com/docker/go-connections master
clone git github.com/docker/go-units master
clone git github.com/docker/libtrust master
clone git github.com/opencontainers/runc master

clean

mv vendor/src/* vendor/
