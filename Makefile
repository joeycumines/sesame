-include config.mak

GO ?= go
GO_FLAGS ?=

STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=

GO_PACKAGES ?= ./...
GO_TEST_FLAGS ?=

GODOC ?= godoc
GODOC_FLAGS ?= -http=:6060

ifeq ($(OS),Windows_NT)
LIST_TOOLS ?= if exist tools.go (for /f tokens^=2^ delims^=^" %%a in ('findstr /r "^[\t ]*_" tools.go') do echo %%a)
else
LIST_TOOLS ?= [ ! -e tools.go ] || grep -E '^[	 ]*_' tools.go | cut -d '"' -f 2
endif

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

.PHONY: fmt
fmt:
	$(GO) fmt $(GO_PACKAGES)

.PHONY: godoc
godoc:
	@echo 'Running godoc, the default URL is http://localhost:6060/pkg/github.com/joeycumines/sesame/'
	$(GODOC) $(GODOC_FLAGS)

.PHONY: update
update: GO_TOOLS := $(shell $(LIST_TOOLS))
update:
	$(GO) get -u -t ./...
	$(foreach tool,$(GO_TOOLS),$(update__TEMPLATE))
	$(GO) mod tidy
define update__TEMPLATE =
$(GO) get -u $(tool)

endef

.PHONY: tools
tools: GO_TOOLS := $(shell $(LIST_TOOLS))
tools:
	$(foreach tool,$(GO_TOOLS),$(tools__TEMPLATE))
define tools__TEMPLATE =
$(GO) install $(tool)

endef

# this won't work on all systems
.PHONY: generate
generate:
	hack/generate.sh
