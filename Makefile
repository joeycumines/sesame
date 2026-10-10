# Simple Go Makefile. This Makefile is restricted to GNU Make 3 functionality.
# This ensures out-of-the-box support for macOS (which ships with Make 3.81)
# and proper support for Windows environments (e.g. via `choco install make -y`).
#
# For a much more sophisticated, modular, multi-module Makefile, see also:
# https://gist.github.com/joeycumines/3352c393c1bf43df72b120ae9134168d
#
# TODO: Consider migrating to the new `go tool` pattern for tools.
# N.B. There is _some_ argument for the existing/legacy `tools.go` pattern.
# Host dependencies aren't great but, using them _does_ have the benefit of
# avoiding the need to mutate the PATH, to shim the protoc plugins.

-include config.mak

GO ?= go
GO_FLAGS ?=

STATICCHECK ?= staticcheck
STATICCHECK_FLAGS ?=

GO_PACKAGES ?= ./...
GO_TEST_FLAGS ?=

GO_INTEGRATION_PACKAGE ?= ./internal/integration/...
GO_INTEGRATION_FLAGS ?= -integration

# provides default timeout for tests if not set by the user
resolve_go_test_flags = $(if $(filter -timeout -timeout=%,$(GO_TEST_FLAGS)),,-timeout=15m) $(GO_TEST_FLAGS)

ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
.SHELLFLAGS := /c
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

.DEFAULT_GOAL := check

SESAME_ENDPOINT_DIR ?= sesame-endpoint
BUN ?= bun

.PHONY: check
check: all test-ts test-integration

.PHONY: all
all: lint build test

.PHONY: clean
clean: sesame-endpoint.clean

.PHONY: lint
lint: vet staticcheck sesame-endpoint.lint

.PHONY: build
build: sesame-endpoint.build
	$(GO) build $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: test
test: test-cover test-race

.PHONY: test-cover
test-cover: build
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -cover $(GO_PACKAGES)

.PHONY: test-race
test-race: build
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -race $(GO_PACKAGES)

.PHONY: test-integration
test-integration: test-integration-cover test-integration-race

# sesame-endpoint (TypeScript) targets, aligned with sesame-endpoint/package.json scripts.
# Canonical targets use the `sesame-endpoint.` prefix; the legacy `*-ts`
# names below are thin backwards-compat aliases.
.PHONY: sesame-endpoint.build
sesame-endpoint.build:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run compile

.PHONY: sesame-endpoint.lint
sesame-endpoint.lint:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run lint

.PHONY: sesame-endpoint.fix
sesame-endpoint.fix:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run fix

.PHONY: sesame-endpoint.clean
sesame-endpoint.clean:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run clean

.PHONY: sesame-endpoint.generate
sesame-endpoint.generate:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run generate

.PHONY: sesame-endpoint.test
sesame-endpoint.test: sesame-endpoint.test-bun sesame-endpoint.test-node

.PHONY: sesame-endpoint.test-bun
sesame-endpoint.test-bun: sesame-endpoint.build
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) test

# The runner loads build/src/index.js, so it must build first even when
# invoked directly rather than through sesame-endpoint.test.
.PHONY: sesame-endpoint.test-node
sesame-endpoint.test-node: sesame-endpoint.build
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) run test:node

.PHONY: sesame-endpoint.install
sesame-endpoint.install:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) install

.PHONY: sesame-endpoint.update
sesame-endpoint.update:
	cd $(SESAME_ENDPOINT_DIR) && $(BUN) update

.PHONY: test-ts
test-ts: sesame-endpoint.test

.PHONY: build-ts
build-ts: sesame-endpoint.build

.PHONY: test-ts-bun
test-ts-bun: sesame-endpoint.test-bun

.PHONY: test-ts-node
test-ts-node: sesame-endpoint.test-node

.PHONY: test-integration-cover
test-integration-cover: build
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -cover $(GO_INTEGRATION_PACKAGE) $(GO_INTEGRATION_FLAGS)

.PHONY: test-integration-race
test-integration-race: build
	$(GO) test $(GO_FLAGS) $(resolve_go_test_flags) -race $(GO_INTEGRATION_PACKAGE) $(GO_INTEGRATION_FLAGS)

.PHONY: vet
vet:
	$(GO) vet $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: staticcheck
staticcheck:
	$(STATICCHECK) $(STATICCHECK_FLAGS) $(GO_PACKAGES)

.PHONY: fmt
fmt: sesame-endpoint.fix
	$(GO) fmt $(GO_PACKAGES)

.PHONY: fix
fix: fmt
	$(GO) fix $(GO_FLAGS) $(GO_PACKAGES)

.PHONY: update
update: sesame-endpoint.update
	$(GO) get -u -t ./...
	@$(LOOP_START) $(GO) get -u $(LOOP_VAR)$(LOOP_END)
	$(GO) mod tidy

.PHONY: tools
tools: sesame-endpoint.install
	@$(LOOP_START) $(GO) install $(LOOP_VAR)$(LOOP_END)

# this won't work on all systems
.PHONY: generate
generate: sesame-endpoint.generate
	hack/generate.sh

# Manual TLS ClientHello verification utility (internal/cmd/quick-test-tls).
# Not part of check: the endpoint track needs a running bun/node subprocess.
.PHONY: quick-test-tls
quick-test-tls: sesame-endpoint.build
	$(GO) run $(GO_FLAGS) ./internal/cmd/quick-test-tls -endpoint \
		-cli $(SESAME_ENDPOINT_DIR)/build/src/cli.js \
		-json scratch/testdata/quick-test-tls/results.json \
		-html scratch/tls-report.html

.PHONY: ci
ci:
	$(GO) env
	$(MAKE) tools
	$(MAKE) all
