#!/bin/sh

# TODO(runcom): we need docker somehow in container to push/pull to registries
#command -v docker >/dev/null 2>&1 || { echo >&2 "Docker is required but it's not installed. Aborting."; exit 1;  }

# TODO(runcom): this can be done also with ensure-frozen-images - see docker/docker
#docker pull busybox:latest >/dev/null 2>&1 || { echo >&2 "docker pull busybox:latest failed. Aborting." exit 1; }

make binary
make install-binary
export GO15VENDOREXPERIMENT=1

echo ""
echo ""
echo "Testing..."
echo ""

cd integration/ && go test -test.timeout=10m -check.v
