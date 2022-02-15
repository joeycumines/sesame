-include config.mak

GO ?= go
GO_FLAGS ?=

STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=

GO_PACKAGES ?= ./...
GO_TEST_FLAGS ?= -cover -race

.PHONY: all
all: lint build test

.PHONY: clean
clean:

.PHONY: lint
lint: vet staticcheck

.PHONY: build
build:
	$(GO) build $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: test
test: build
	$(GO) test $(GO_FLAGS) $(GO_TEST_FLAGS) $(GO_PACKAGES)

.PHONY: vet
vet:
	$(GO) vet $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: staticcheck
staticcheck:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(GO_PACKAGES)

# this won't work on all systems
.PHONY: tools
tools:
	export CGO_ENABLED=0 && \
		grep -P '^\t_' tools.go | \
		cut -d '"' -f 2 | \
		while read -r line; do go install "$$line" || exit 1; done

.PHONY: fmt
fmt:
	$(GO) fmt $(GO_PACKAGES)
