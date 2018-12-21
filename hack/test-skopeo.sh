#!/usr/bin/env bash
set -e

export GOPATH=$(mktemp -d)
skopeo_path=${GOPATH}/src/github.com/containers/skopeo

trap "rm -rf ${GOPATH}" EXIT

if [ -z ${TRAVIS_PULL_REQUEST_SLUG} ] ; then
    git clone -b master https://github.com/containers/skopeo ${skopeo_path}
else
    if ! git clone -b "${TRAVIS_BRANCH}" \
            https://:@github.com/"${TRAVIS_PULL_REQUEST_SLUG/image/skopeo}" \
            ${skopeo_path} ; then
        git clone -b master https://github.com/containers/skopeo ${skopeo_path}
    fi
fi

vendor_path=${skopeo_path}/vendor/github.com/containers/image
rm -rf ${vendor_path} && cp -r . ${vendor_path} && rm -rf ${vendor_path}/vendor
cd ${skopeo_path}
make BUILDTAGS="${BUILDTAGS}" binary-local test-all-local
${SUDO} make BUILDTAGS="${BUILDTAGS}" check
