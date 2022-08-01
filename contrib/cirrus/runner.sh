#!/bin/bash

# This script is intended to be executed by automation or humans
# under a hack/get_ci_vm.sh context.  Use under any other circumstances
# is unlikely to function.

set -e

if [[ -r "/etc/automation_environment" ]]; then
    source /etc/automation_environment
    source $AUTOMATION_LIB_PATH/common_lib.sh
else
    (
    echo "WARNING: It does not appear that containers/automation was installed."
    echo "         Functionality of most of ${BASH_SOURCE[0]} will be negatively"
    echo "         impacted."
    ) > /dev/stderr
fi

export "PATH=$PATH:$GOPATH/bin"

_run_setup() {
    req_env_vars SKOPEO_PATH SKOPEO_CI_TAG GOSRC

    project_module=$(go list .)

    make tools

    rm -rf "${SKOPEO_PATH}"
    git clone -b ${SKOPEO_CI_TAG} \
        https://github.com/containers/skopeo.git ${SKOPEO_PATH}

    cd "${SKOPEO_PATH}"
    if [[ -n "$SKOPEO_PR" ]] && [[ $SKOPEO_PR -gt 1000 ]]; then
        warn "Fetching and checking out code from skopeo pull-request #$SKOPEO_PR"
        git fetch origin "+refs/pull/$SKOPEO_PR/head"
        git checkout FETCH_HEAD
    fi

    msg "Replacing upstream skopeo $SKOPEO_CI_TAG branch $project_module module"
    go mod edit -replace ${project_module}=$GOSRC

    "${SKOPEO_PATH}/${SCRIPT_BASE}/runner.sh" setup
}

req_env_vars GOSRC

handler="_run_${1}"
if [ "$(type -t $handler)" != "function" ]; then
    die "Unknown/Unsupported command-line argument '$1'"
fi

msg "************************************************************"
msg "Runner executing $1 on $OS_REL_VER"
msg "************************************************************"

cd "$GOSRC"
$handler
