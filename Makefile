# Note: This Makefile is constrained to GNU Make 3 functionality.
# This ensures out-of-the-box support for macOS (which ships with Make 3.81)
# and proper support for Windows environments (e.g. via `choco install make -y`).

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
LOOP_START ?= @for %%t in ($(or $(shell $(LIST_TOOLS)),$(error failed to list tools))) do @echo + $(GO) install %%t && (
LOOP_VAR ?= %%t
LOOP_END ?= ) || ( echo ERROR: exit code %%errorlevel%% & exit 1 )
else
LIST_TOOLS ?= [ ! -e tools.go ] || grep -E '^[	 ]*_' tools.go | cut -d '"' -f 2
LOOP_START ?= for t in $(or $(shell $(LIST_TOOLS)),$(error failed to list tools)); do if ! ( set -x;
LOOP_VAR ?= $$t
LOOP_END ?= ; ); then echo "ERROR: exit code $$?" >&2; exit 1; fi; done
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
update:
	$(GO) get -u -t ./...
	@$(LOOP_START) $(GO) get -u $(LOOP_VAR)$(LOOP_END)
	$(GO) mod tidy

.PHONY: tools
tools:
	@$(LOOP_START) $(GO) install $(LOOP_VAR)$(LOOP_END)

# this won't work on all systems
.PHONY: generate
generate:
	hack/generate.sh

.PHONY: ci
ci:
	$(GO) env
	$(MAKE) tools
	$(MAKE) all
