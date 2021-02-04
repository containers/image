ARG BASE_IMAGE=ubuntu:latest
FROM $BASE_IMAGE
ARG GOPATH=/var/tmp/go
ARG TEST_USER=testuser

RUN apt-get -qq update && \
    apt-get install -y sudo docker.io libdevmapper-dev libgpgme-dev libostree-dev

RUN adduser --shell=/bin/bash --disabled-password \
        --gecos "$TEST_USER" "$TEST_USER" && \
    mkdir -p "$GOPATH" && \
    chown -R $TEST_USER:$TEST_USER "$GOPATH" && \
    find "$GOPATH" -type d -exec chmod 2770 '{}' +
ENV PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:$GOPATH/bin"
USER $TEST_USER
