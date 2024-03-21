#!/bin/bash

set -eo pipefail

eval $(go env)
PATH="$GOPATH/bin:$PATH"

die() { echo "Error: ${1:-No message provided}" > /dev/stderr; exit 1; }

# Always run from the repository root
cd $(dirname "${BASH_SOURCE[0]}")/../

if [[ -z $(type -P gofmt) ]]; then
    die "Unable to find 'gofmt' binary in \$PATH: $PATH"
fi

echo "Executing go vet"
go vet -tags="$BUILDTAGS" ./...

echo "Executing gofmt"
OUTPUT=$(gofmt -s -l . | sed -e '/^vendor/d')
if [[ ! -z "$OUTPUT" ]]; then
    die "Please fix the formatting of the following files:
$OUTPUT"
fi
