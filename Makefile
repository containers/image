.PHONY: all deps test validate lint

all: deps .gitvalidation test validate

deps:
	go get -t ./...
	go get -u github.com/golang/lint/golint
	go get github.com/vbatts/git-validation

test:
	@go test -cover ./...

validate: lint
	@go vet ./...
	@test -z "$(gofmt -s -l . | tee /dev/stderr)"

lint:
	@out="$$(golint ./...)"; \
	if [ -n "$$(golint ./...)" ]; then \
		echo "$$out"; \
		exit 1; \
	fi

.PHONY: .gitvalidation

EPOCH_TEST_COMMIT ?= e68e0e1110e64f906f9b482e548f17d73e02e6b1

# When this is running in travis, it will only check the travis commit range
.gitvalidation:
	@which git-validation > /dev/null 2>/dev/null || (echo "ERROR: git-validation not found. Consider 'make deps' target" && false)
ifeq ($(TRAVIS),true)
	@git-validation -q -run DCO,short-subject,dangling-whitespace
else
	@git-validation -v -run DCO,short-subject,dangling-whitespace -range $(EPOCH_TEST_COMMIT)..HEAD
endif
