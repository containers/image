FROM golang

ENV SRCPATH /go/src/github.com/containers/image
ENV DOCKER_VERSION 1.13.0
ENV DOCKER_PATH docker-${DOCKER_VERSION}.tgz

# needed because if the tests run as root the tests that test permissions will fail.
RUN useradd -d /home/gouser -m -s /bin/bash gouser

# NOTE: iptables and psmisc are required by docker.
RUN apt-get update -qq && apt-get install -y sudo iptables psmisc btrfs-tools libdevmapper-dev libgpgme11-dev

RUN wget -q https://get.docker.com/builds/Linux/x86_64/${DOCKER_PATH}
RUN tar -xpf ${DOCKER_PATH} --strip-components=1 -C /usr/bin
RUN gpasswd -a gouser root

# dind is required for the skopeo tests
COPY dind /dind
RUN chmod +x /dind

COPY . ${SRCPATH}
WORKDIR ${SRCPATH}
RUN make deps
RUN chown -R gouser:gouser ${SRCPATH}

CMD sh -c "cd ${SRCPATH} && bash docker-test.sh"
ENTRYPOINT ["/dind"]
