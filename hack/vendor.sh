#!/usr/bin/env bash
set -e

cd "$(dirname "$BASH_SOURCE")/.."
rm -rf vendor/
source 'hack/.vendor-helpers.sh'

clone git github.com/codegangsta/cli v1.2.0
clone git github.com/Sirupsen/logrus v0.10.0
clone git github.com/go-check/check v1
clone git github.com/docker/docker master

clean

mv vendor/src/* vendor/
