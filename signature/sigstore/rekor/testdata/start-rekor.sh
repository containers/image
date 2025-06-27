#!/usr/bin/env bash

# Script to run a local rekor server, setup based on
# https://github.com/sigstore/rekor/blob/main/docker-compose.yml
# Required for Test_rekorUploadKeyOrCert unit test so it can run
# against a real server.
# set REKOR_SERVER_URL="http://127.0.0.1:3000"

set -e

SUFFIX=${1:-default}

REKOR_IMAGE=ghcr.io/sigstore/rekor/rekor-server:v1.3.10
TRILLIAN_SIGNER_IMAGE=ghcr.io/sigstore/rekor/trillian_log_signer:v1.3.4
TRILLIAN_SERVER_IMAGE=ghcr.io/sigstore/rekor/trillian_log_server:v1.3.4
MYSQL_IMAGE=gcr.io/trillian-opensource-ci/db_server:v1.7.2

POD_NAME=rekor-pod-$SUFFIX

if [[ "$2" == "remove" ]]; then
    podman pod rm -f -t0 $POD_NAME
    exit 0
fi

# On errors make sure to remove the pod again.
trap "podman pod rm -f -t0 $POD_NAME" ERR

podman pod create --name $POD_NAME -p 3000:3000

podman run -d --pod $POD_NAME --name rekor-db-$SUFFIX \
    -e MYSQL_ROOT_PASSWORD=zaphod \
    -e MYSQL_DATABASE=test \
    -e MYSQL_USER=test \
    -e MYSQL_PASSWORD=zaphod \
    $MYSQL_IMAGE

# The db takes a bit to start up, wait until it is ready otherwise the trillian
# containers fail to start due the missing db connection.
max_retries=20
retries=0
while [[ $retries -le $max_retries ]]; do
    out=$(podman logs rekor-db-$SUFFIX 2>&1)
    if [[ "$out" =~ "port: 3306" ]]; then
        break
    fi

    retries=$((retries + 1))
    if [[ $retries -ge $max_retries ]]; then
        echo "Failed to wait for the database to become ready"
        podman pod rm -f -t0 $POD_NAME
        exit 1
    fi
    sleep 1
done

podman run -d --pod $POD_NAME --name rekor-trillian-server-$SUFFIX \
    $TRILLIAN_SERVER_IMAGE \
    --quota_system=noop \
    --storage_system=mysql \
    --mysql_uri="test:zaphod@tcp(127.0.0.1:3306)/test" \
    --rpc_endpoint=0.0.0.0:8090 \
    --http_endpoint=0.0.0.0:8091 \
    --alsologtostderr

podman run -d --pod $POD_NAME --name rekor-trillian-signer-$SUFFIX \
    $TRILLIAN_SIGNER_IMAGE \
    --quota_system=noop \
    --storage_system=mysql \
    --mysql_uri="test:zaphod@tcp(127.0.0.1:3306)/test" \
    --rpc_endpoint=0.0.0.0:8190 \
    --http_endpoint=0.0.0.0:8191 \
    --force_master \
    --alsologtostderr

podman run -d --pod $POD_NAME --name rekor-server-$SUFFIX \
    -e TMPDIR=/var/run/attestations \
    -v "/var/run/attestations" \
    $REKOR_IMAGE \
    serve \
    --trillian_log_server.address=127.0.0.1 \
    --trillian_log_server.port=8090 \
    --rekor_server.address=0.0.0.0 \
    --rekor_server.signer=memory \
    --enable_retrieve_api=false \
    --enable_attestation_storage \
    --attestation_storage_bucket="file:///var/run/attestations" \
    --enable_stable_checkpoint \
    --search_index.storage_provider=mysql \
    --search_index.mysql.dsn="test:zaphod@tcp(127.0.0.1:3306)/test"
