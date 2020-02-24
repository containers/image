FROM ubuntu:18.04

RUN apt-get -qq update && \
    apt-get install -y sudo docker.io git make btrfs-tools libdevmapper-dev libgpgme-dev curl

ADD https://storage.googleapis.com/golang/go1.11.12.linux-amd64.tar.gz /tmp

RUN tar -C /usr/local -xzf /tmp/go1.11.12.linux-amd64.tar.gz && \
    rm /tmp/go1.11.12.linux-amd64.tar.gz && \
    ln -s /usr/local/go/bin/* /usr/local/bin/
