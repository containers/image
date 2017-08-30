.PHONY: all tools test validate lint

# Which github repostiory and branch to use for testing with skopeo
SKOPEO_REPO = projectatomic/skopeo
SKOPEO_BRANCH = master
# Set SUDO=sudo to run container integration tests using sudo.
SUDO =
BUILDTAGS   = btrfs_noversion libdm_no_deferred_remove
BUILDFLAGS := -tags "$(BUILDTAGS)"

PACKAGES := $(shell go list ./... | grep -v github.com/containers/image/vendor)

all: tools .gitvalidation test validate

tools: tools.timestamp

tools.timestamp: Makefile
	@go get -u $(BUILDFLAGS) github.com/golang/lint/golint
	@go get $(BUILDFLAGS) github.com/vbatts/git-validation
	@go get -u github.com/rancher/trash
	@touch tools.timestamp

vendor: tools.timestamp vendor.conf
	@trash
	@touch vendor

clean:
	rm -rf vendor tools.timestamp

test: vendor
	@go test $(BUILDFLAGS) -cover $(PACKAGES)

# This is not run as part of (make all), but Travis CI does run this.
# Demonstarting a working version of skopeo (possibly with modified SKOPEO_REPO/SKOPEO_BRANCH, e.g.
#    make test-skopeo SKOPEO_REPO=runcom/skopeo-1 SKOPEO_BRANCH=oci-3 SUDO=sudo
# ) is a requirement before merging; note that Travis will only test
# the master branch of the upstream repo.
test-skopeo:
	@echo === Testing skopeo build
	@export GOPATH=$$(mktemp -d) && \
		skopeo_path=$${GOPATH}/src/github.com/projectatomic/skopeo && \
		vendor_path=$${skopeo_path}/vendor/github.com/containers/image && \
		git clone -b $(SKOPEO_BRANCH) https://github.com/$(SKOPEO_REPO) $${skopeo_path} && \
		rm -rf $${vendor_path} && cp -r . $${vendor_path} && rm -rf $${vendor_path}/vendor && \
		cd $${skopeo_path} && \
		make BUILDTAGS="$(BUILDTAGS)" binary-local test-all-local && \
		$(SUDO) make BUILDTAGS="$(BUILDTAGS)" check && \
		rm -rf $${skopeo_path}

validate: lint
	@go vet $(PACKAGES)
	@test -z "$$(gofmt -s -l . | grep -ve '^vendor' | tee /dev/stderr)"

lint:
	@for package in $(PACKAGES); do \
		out="$$(golint $$package)"; \
		if [ -n "$$out" ]; then \
			echo "$$out"; \
			exit 1; \
		fi \
	done

.PHONY: .gitvalidation

EPOCH_TEST_COMMIT ?= e68e0e1110e64f906f9b482e548f17d73e02e6b1

# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make clean && make tools'" && false)
ifeq ($(TRAVIS),true)
	git-validation -q -run DCO,short-subject,dangling-whitespace
else
	git-validation -q -run DCO,short-subject,dangling-whitespace -range $(EPOCH_TEST_COMMIT)..HEAD
endif
