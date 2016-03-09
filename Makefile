.PHONY: all binary build clean install install-binary man shell test-integration

export GO15VENDOREXPERIMENT=1

PREFIX ?= ${DESTDIR}/usr
INSTALLDIR=${PREFIX}/bin
MANINSTALLDIR=${PREFIX}/share/man
# TODO(runcom)
#BASHINSTALLDIR=${PREFIX}/share/bash-completion/completions

GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
DOCKER_IMAGE := skopeo-dev$(if $(GIT_BRANCH),:$(GIT_BRANCH))
# set env like gobuildtag?
DOCKER_FLAGS := docker run --rm -i #$(DOCKER_ENVS)
# if this session isn't interactive, then we don't want to allocate a
# TTY, which would fail, but if it is interactive, we do want to attach
# so that the user can send e.g. ^C through.
INTERACTIVE := $(shell [ -t 0 ] && echo 1 || echo 0)
ifeq ($(INTERACTIVE), 1)
	DOCKER_FLAGS += -t
endif
DOCKER_RUN_DOCKER := $(DOCKER_FLAGS) "$(DOCKER_IMAGE)"

all: man binary

binary:
	go build -o ${DEST}skopeo .

build-container:
	docker build ${DOCKER_BUILD_ARGS} -t "$(DOCKER_IMAGE)" .

clean:
	rm -f skopeo
	rm -f skopeo.1

install: install-binary
	install -m 644 skopeo.1 ${MANINSTALLDIR}/man1/
	# TODO(runcom)
	#install -m 644 completion/bash/skopeo ${BASHINSTALLDIR}/

install-binary:
	install -d -m 0755 ${INSTALLDIR}
	install -m 755 skopeo ${INSTALLDIR}

man:
	go-md2man -in man/skopeo.1.md -out skopeo.1

shell: build-container
	$(DOCKER_RUN_DOCKER) bash

test-integration: build-container
	$(DOCKER_RUN_DOCKER) hack/make.sh test-integration

validate: build-container
	$(DOCKER_RUN_DOCKER) hack/make.sh validate-git-marks validate-gofmt validate-lint validate-vet
