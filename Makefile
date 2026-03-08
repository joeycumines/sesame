-include config.mak

GO ?= go
GO_FLAGS ?=

STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=

GO_PACKAGES ?= ./...
GO_TEST_FLAGS ?=

# provides default timeout for tests if not set by the user
resolve_go_test_flags = $(if $(filter -timeout -timeout=%,$(GO_TEST_FLAGS)),,-timeout=15m) $(GO_TEST_FLAGS)

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
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -cover $(GO_PACKAGES)

.PHONY: test-race
test-race: build
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -race $(GO_PACKAGES)

.PHONY: vet
vet:
	$(GO) vet $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: staticcheck
staticcheck:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(GO_PACKAGES)

.PHONY: fmt
fmt:
	$(GO) fmt $(GO_PACKAGES)

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
