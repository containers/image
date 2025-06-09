export GOPROXY=https://proxy.golang.org


GOBIN := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

# when cross compiling _for_ a Darwin or windows host, then we must use openpgp
BUILD_TAGS_WINDOWS_CROSS = containers_image_openpgp
BUILD_TAGS_DARWIN_CROSS = containers_image_openpgp
SEQUOIA_SONAME_DIR =

BUILDTAGS =
BUILDFLAGS := -tags "$(BUILDTAGS)" -ldflags '-X github.com/containers/image/v5/signature/internal/sequoia.sequoiaLibraryDir='"$(SEQUOIA_SONAME_DIR)"

# Extra flags passed to go test
TESTFLAGS :=
TEST_PACKAGES := ./...

PACKAGES := $(shell go list $(BUILDFLAGS) ./...)
SOURCE_DIRS = $(shell echo $(PACKAGES) | awk 'BEGIN{FS="/"; RS=" "}{print $$4}' | uniq)

PREFIX ?= ${DESTDIR}/usr
MANINSTALLDIR=${PREFIX}/share/man
GOMD2MAN ?= $(shell command -v go-md2man || echo '$(GOBIN)/go-md2man')
MANPAGES_MD = $(wildcard docs/*.5.md)
MANPAGES ?= $(MANPAGES_MD:%.md=%)

ifeq ($(shell uname -s),FreeBSD)
CONTAINERSCONFDIR ?= /usr/local/etc/containers
else
CONTAINERSCONFDIR ?= /etc/containers
endif
REGISTRIESDDIR ?= ${CONTAINERSCONFDIR}/registries.d

# N/B: This value is managed by Renovate, manual changes are
# possible, as long as they don't disturb the formatting
# (i.e. DO NOT ADD A 'v' prefix!)
GOLANGCI_LINT_VERSION := 2.3.1

export PATH := $(PATH):${GOBIN}

.PHONY: all
all: tools test validate .gitvalidation

.PHONY: build
build:
	go build $(BUILDFLAGS) ./...

$(MANPAGES): %: %.md
	$(GOMD2MAN) -in $< -out $@

docs: $(MANPAGES)

.PHONY: install-docs
install-docs: docs
	install -d -m 755 ${MANINSTALLDIR}/man5
	install -m 644 docs/*.5 ${MANINSTALLDIR}/man5/

.PHONY: install
install: install-docs
	install -d -m 755 ${DESTDIR}${CONTAINERSCONFDIR}
	install -m 644 default-policy.json ${DESTDIR}${CONTAINERSCONFDIR}/policy.json
	install -d -m 755 ${DESTDIR}${REGISTRIESDDIR}
	install -m 644 default.yaml ${DESTDIR}${REGISTRIESDDIR}/default.yaml

.PHONY: cross
cross:
	GOOS=windows $(MAKE) build BUILDTAGS="$(BUILDTAGS) $(BUILD_TAGS_WINDOWS_CROSS)"
	GOOS=darwin $(MAKE) build BUILDTAGS="$(BUILDTAGS) $(BUILD_TAGS_DARWIN_CROSS)"

.PHONY: tools
tools: .install.gitvalidation .install.golangci-lint

.PHONY: .install.gitvalidation
.install.gitvalidation:
	if [ ! -x "$(GOBIN)/git-validation" ]; then \
		go install github.com/vbatts/git-validation@latest; \
	fi

.PHONY: .install.golangci-lint
.install.golangci-lint:
	if [ ! -x "$(GOBIN)/golangci-lint" ]; then \
		curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(GOBIN) v$(GOLANGCI_LINT_VERSION) ; \
	fi

.PHONY: clean
clean:
	rm -rf $(MANPAGES)

.PHONY: test
test:
	@go test $(BUILDFLAGS) $(TESTFLAGS) -cover $(TEST_PACKAGES)

.PHONY: fmt
fmt:
	@gofmt -l -s -w $(SOURCE_DIRS)

.PHONY: validate
validate: lint
	@BUILDTAGS="$(BUILDTAGS)" hack/validate.sh

.PHONY: lint
lint:
	$(GOBIN)/golangci-lint run --build-tags "$(BUILDTAGS)"

# When this is running in CI, it will only check the CI commit range
.PHONY: .gitvalidation
.gitvalidation:
	@which $(GOBIN)/git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make clean && make tools'" && false)
	git fetch -q "https://github.com/containers/image.git" "refs/heads/main"
	upstream="$$(git rev-parse --verify FETCH_HEAD)" ; \
		$(GOBIN)/git-validation -q -run DCO,short-subject,dangling-whitespace -range $$upstream..HEAD

.PHONY: vendor-in-container
vendor-in-container:
	podman run --privileged --rm --env HOME=/root -v `pwd`:/src -w /src golang go mod tidy

.PHONY: codespell
codespell:
	codespell -w
