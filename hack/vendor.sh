#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli c31a7975863e7810c92e2e288a9ab074f9a88f29
#clone git github.com/Azure/go-ansiterm 70b2c90b260171e829f1ebd7c17f600c11858dbe
clone git github.com/Sirupsen/logrus v0.8.7 # logrus is a common dependency among multiple deps
clone git github.com/docker/docker 7be8f7264435db8359ec9fd18362391bad1ca4d7
clone git golang.org/x/net 47990a1ba55743e6ef1affd3a14e5bac8553615d https://github.com/golang/net.git
clone git github.com/docker/engine-api v0.2.2
clone git github.com/docker/distribution 47a064d4195a9b56133891bbb13620c3ac83a827

clean

mv vendor/src/* vendor/
