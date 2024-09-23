.PHONY: all tools test validate lint .gitvalidation fmt

export GOPROXY=https://proxy.golang.org


GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# when cross compiling _for_ a Darwin or windows host, then we must use openpgp
BUILD_TAGS_WINDOWS_CROSS = containers_image_openpgp
BUILD_TAGS_DARWIN_CROSS = containers_image_openpgp

BUILDTAGS = btrfs_noversion libdm_no_deferred_remove
BUILDFLAGS := -tags "$(BUILDTAGS)"

PACKAGES := $(shell GO111MODULE=on go list $(BUILDFLAGS) ./...)
SOURCE_DIRS = $(shell echo $(PACKAGES) | awk 'BEGIN{FS="/"; RS=" "}{print $$4}' | uniq)

PREFIX ?= ${DESTDIR}/usr
MANINSTALLDIR=${PREFIX}/share/man
GOMD2MAN ?= $(shell command -v go-md2man || echo '$(GOBIN)/go-md2man')
MANPAGES_MD = $(wildcard docs/*.5.md)
MANPAGES ?= $(MANPAGES_MD:%.md=%)

export PATH := $(PATH):${GOBIN}

all: tools test validate .gitvalidation

build:
	GO111MODULE="on" go build $(BUILDFLAGS) ./...

$(MANPAGES): %: %.md
	$(GOMD2MAN) -in $< -out $@

docs: $(MANPAGES)

install-docs: docs
	install -d -m 755 ${MANINSTALLDIR}/man5
	install -m 644 docs/*.5 ${MANINSTALLDIR}/man5/

install: install-docs

cross:
	GOOS=windows $(MAKE) build BUILDTAGS="$(BUILDTAGS) $(BUILD_TAGS_WINDOWS_CROSS)"
	GOOS=darwin $(MAKE) build BUILDTAGS="$(BUILDTAGS) $(BUILD_TAGS_DARWIN_CROSS)"

tools: .install.gitvalidation .install.golangci-lint

.install.gitvalidation:
	if [ ! -x "$(GOBIN)/git-validation" ]; then \
		GO111MODULE="off" go get $(BUILDFLAGS) github.com/vbatts/git-validation; \
	fi

.install.golangci-lint:
	if [ ! -x "$(GOBIN)/golangci-lint" ]; then \
		curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(GOBIN) v1.51.0; \
	fi

clean:
	rm -rf $(MANPAGES)

test:
	@GO111MODULE="on" go test $(BUILDFLAGS) -cover ./...

fmt:
	@gofmt -l -s -w $(SOURCE_DIRS)

validate: lint
	@BUILDTAGS="$(BUILDTAGS)" hack/validate.sh

lint:
	$(GOBIN)/golangci-lint run --build-tags "$(BUILDTAGS)"

# When this is running in CI, it will only check the CI commit range
.gitvalidation:
	@which $(GOBIN)/git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make clean && make tools'" && false)
	git fetch -q "https://github.com/containers/image.git" "refs/heads/main"
	upstream="$$(git rev-parse --verify FETCH_HEAD)" ; \
		$(GOBIN)/git-validation -q -run DCO,short-subject,dangling-whitespace -range $$upstream..HEAD

vendor-in-container:
	podman run --privileged --rm --env HOME=/root -v `pwd`:/src -w /src golang go mod tidy

codespell:
	codespell -S Makefile,build,buildah,buildah.spec,imgtype,copy,AUTHORS,bin,vendor,.git,go.sum,CHANGELOG.md,changelog.txt,seccomp.json,.cirrus.yml,"*.xz,*.gz,*.tar,*.tgz,*ico,*.png,*.1,*.5,*.orig,*.rej" -L keypair,flate,uint,iff,od,ERRO -w
