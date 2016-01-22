#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli c31a7975863e7810c92e2e288a9ab074f9a88f29
clone git github.com/Azure/go-ansiterm 70b2c90b260171e829f1ebd7c17f600c11858dbe
clone git github.com/Sirupsen/logrus v0.8.7 # logrus is a common dependency among multiple deps
clone git github.com/docker/docker v1.10.0-rc1
clone git github.com/docker/libtrust 9cbd2a1374f46905c68a4eb3694a130610adc62a
clone git github.com/gorilla/context 14f550f51a
clone git github.com/gorilla/mux e444e69cbd
clone git golang.org/x/net 47990a1ba55743e6ef1affd3a14e5bac8553615d https://github.com/golang/net.git
clone git github.com/docker/go-units 651fc226e7441360384da338d0fd37f2440ffbe3
clone git github.com/docker/go-connections v0.1.2
clone git github.com/docker/engine-api v0.2.2

# get graph and distribution packages
clone git github.com/docker/distribution 47a064d4195a9b56133891bbb13620c3ac83a827
clone git github.com/vbatts/tar-split v0.9.11

clone git github.com/opencontainers/runc 47e3f834d73e76bc2a6a585b48d2a93325b34979 # libcontainer

clean

mv vendor/src/* vendor/
