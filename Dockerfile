FROM fedora

RUN dnf -y update && dnf install -y make git golang golang-github-cpuguy83-go-md2man golang-github-Sirupsen-logrus-devel golang-github-codegangsta-cli-devel golang-golangorg-net-devel

# Install two versions of the registry. The first is an older version that
# only supports schema1 manifests. The second is a newer version that supports
# both. This allows integration-cli tests to cover push/pull with both schema1
# and schema2 manifests.
ENV REGISTRY_COMMIT_SCHEMA1 ec87e9b6971d831f0eff752ddb54fb64693e51cd
ENV REGISTRY_COMMIT 47a064d4195a9b56133891bbb13620c3ac83a827
#ENV REGISTRY_COMMIT_v1 TODO(runcom)
RUN set -x \
	&& export GOPATH="$(mktemp -d)" \
	&& git clone https://github.com/docker/distribution.git "$GOPATH/src/github.com/docker/distribution" \
	&& (cd "$GOPATH/src/github.com/docker/distribution" && git checkout -q "$REGISTRY_COMMIT") \
	&& GOPATH="$GOPATH/src/github.com/docker/distribution/Godeps/_workspace:$GOPATH" \
		go build -o /usr/local/bin/registry-v2 github.com/docker/distribution/cmd/registry \
	&& (cd "$GOPATH/src/github.com/docker/distribution" && git checkout -q "$REGISTRY_COMMIT_SCHEMA1") \
	&& GOPATH="$GOPATH/src/github.com/docker/distribution/Godeps/_workspace:$GOPATH" \
		go build -o /usr/local/bin/registry-v2-schema1 github.com/docker/distribution/cmd/registry \
	&& rm -rf "$GOPATH"

ENV GOPATH /usr/share/gocode:/go
WORKDIR /go/src/github.com/runcom/skopeo

#ENTRYPOINT ["hack/dind"]

COPY . /go/src/github.com/runcom/skopeo

# remove distro-supplied dependencies, so we build against them and the rest from vendor/
RUN rm -rf /go/src/github.com/runcom/skopeo/vendor/golang.org && rm -rf /go/src/github.com/runcom/skopeo/vendor/github.com/Sirupsen && rm -rf /go/src/github.com/runcom/skopeo/vendor/github.com/codegangsta
