#!/usr/bin/env bash

# This script runs the Google API linter over the sesame protos.
# The googleapis dependency tree is vendored (a git submodule under
# schema/googleapis) and the grpc-gateway openapiv2 options are vendored under
# schema/protoc-gen-openapiv2, so no export step is needed: the linter resolves
# imports directly from those include roots, exactly like protoc does.

set -o pipefail || exit 1

SCRIPT_DIR=$(dirname "$0")
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
LINTER_MODULE_DIR="$REPO_ROOT/hack/google-api-linter"
CONFIG_FILE="$REPO_ROOT/google-api-linter.yaml"

if ! PROTO_FILES=$(find "$REPO_ROOT/schema/sesame" -type f -name '*.proto' | sort); then
  echo "::error::Failed to list sesame proto files."
  exit 1
fi

if [ -z "$PROTO_FILES" ]; then
  echo "::error::No sesame proto files found under schema/sesame."
  exit 1
fi

# go -C runs the tool from the linter module directory, so every path handed to
# the linter must be absolute. Repo-relative paths are restored in the output so
# annotations stay readable (and CI-actionable).
find "$REPO_ROOT/schema/sesame" -type f -name '*.proto' -print0 |
  xargs -0 go -C "$LINTER_MODULE_DIR" tool github.com/googleapis/api-linter/cmd/api-linter \
  --config="$CONFIG_FILE" \
  --output-format=github \
  --set-exit-status \
  --proto-path="$REPO_ROOT/schema" \
  --proto-path="$REPO_ROOT/schema/googleapis" \
  --proto-path="$REPO_ROOT/schema/protoc-gen-openapiv2" |
  sed "s|$REPO_ROOT/||g"

LINTER_EXIT_CODE="$?"

if ! [ "$LINTER_EXIT_CODE" -eq 0 ]; then
  echo "::error::API linter found issues."
  exit "$LINTER_EXIT_CODE"
fi
