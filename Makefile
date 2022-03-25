-include config.mak

GO ?= go
GO_FLAGS ?=

STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=

GO_PACKAGES ?= ./...
GO_TEST_FLAGS ?=

GODOC ?= godoc
GODOC_FLAGS ?= -http=:6060

LIST_TOOLS = grep -P '^\t_' tools.go | cut -d '"' -f 2

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
test: test-cover test-race

.PHONY: test-cover
test-cover: build
	$(GO) test $(GO_FLAGS) $(GO_TEST_FLAGS) -cover $(GO_PACKAGES)

.PHONY: test-race
test-race: build
	$(GO) test $(GO_FLAGS) $(GO_TEST_FLAGS) -race $(GO_PACKAGES)

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
		run_command() { echo "$$@" && "$$@"; } && \
		$(LIST_TOOLS) | \
		while read -r line; do run_command go install "$$line" || exit 1; done

# this won't work on all systems
.PHONY: update-tools
update-tools:
	run_command() { echo "$$@" && "$$@"; } && \
		$(LIST_TOOLS) | \
		while read -r line; do run_command go get -u "$$line" || exit 1; done

# this won't work on all systems
.PHONY: generate
generate:
	hack/generate.sh

.PHONY: fmt
fmt:
	$(GO) fmt $(GO_PACKAGES)

.PHONY: godoc
godoc:
	@echo 'Running godoc, the default URL is http://localhost:6060/pkg/github.com/joeycumines/sesame/'
	$(GODOC) $(GODOC_FLAGS)
