#!/bin/bash

set -eo pipefail

# Always run from the repository root
cd $(dirname "${BASH_SOURCE[0]}")/../

echo "Executing go vet"
go vet -tags="$BUILDTAGS" ./...
