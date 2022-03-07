#!/usr/bin/env bash

# (re)generates all auto-generated source and specifications
#
# this script is intended to be used like `make generate`

# TODO consider gapic support https://github.com/googleapis/gapic-showcase
#      e.g. https://github.com/googleapis/gapic-showcase/blob/main/util/compile_protos.go#L30

set -euo pipefail || exit 1
trap 'echo [FAILURE] line_number=${LINENO} exit_code=${?} bash_version=${BASH_VERSION}' ERR
script_path="$( (DIR="$( (
    SOURCE="${BASH_SOURCE[0]}"
    while [ -h "$SOURCE" ]; do
        DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
        SOURCE="$(readlink "$SOURCE")"
        [[ ${SOURCE} != /* ]] &&
            SOURCE="$DIR/$SOURCE"
    done
    echo "$(cd -P "$(dirname "$SOURCE")" && pwd)"
))" &&
    [ ! -z "$DIR" ] &&
    cd "$DIR" &&
    pwd))"

command -v protoc >/dev/null 2>&1

# install these commands like `make tools`
command -v protoc-gen-go >/dev/null 2>&1
command -v protoc-gen-go-grpc >/dev/null 2>&1
command -v protoc-gen-go-copy >/dev/null 2>&1
command -v protoc-gen-sesame >/dev/null 2>&1

cmd=(
    bash -c '
set -x &&
protoc \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    --go-copy_out=. --go-copy_opt=paths=source_relative \
    "$@" \
&&
protoc \
    --sesame_out=. --sesame_opt=paths=source_relative \
    "$@"'
    -
)

cd "$script_path/../schema"
echo "Schema path: $(pwd)"

find . -type f -name '*.proto' -exec "${cmd[@]}" {} +
