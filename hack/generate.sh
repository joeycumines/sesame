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

main() {
    set -euo pipefail || exit 1

    common=(-I . -I ./api-common-protos)

    schema_prefix=sesame/v1alpha1/sesame.
    schema_openapi="$schema_prefix"openapi.yaml

    client_name=genrest
    client_dir=../internal/"$client_name"

    set -x

    protoc \
        "${common[@]}" \
        --go_out=. --go_opt=paths=source_relative \
        --go-grpc_out=. --go-grpc_opt=paths=source_relative \
        --go-copy_out=. --go-copy_opt=paths=source_relative \
        --grpc-gateway_out=. --grpc-gateway_opt=paths=source_relative --grpc-gateway_opt=logtostderr=true \
        "$@"

    protoc \
        "${common[@]}" \
        --sesame_out=. --sesame_opt=paths=source_relative \
        "$@"

    protoc \
        "${common[@]}" \
        --openapiv2_out=. --openapiv2_opt=logtostderr=true \
        "$schema_prefix"proto

    curl 'https://converter.swagger.io/api/convert' \
        -sf \
        -H 'accept: application/yaml' \
        -H 'content-type: application/json' \
        --data-binary '@'"$schema_prefix"swagger.json \
        --compressed \
        -o "$schema_openapi"

    mkdir -p "$client_dir"
    oapi-codegen -generate types,client -package "$client_name" -o "$client_dir"/"$client_name".gen.go "$schema_openapi"
    cd "$client_dir"
    go vet
    go build
    go test
}

cmd=(bash -c "$(declare -f main && echo 'main "$@"')" -)

cd "$script_path/../schema"
echo "Schema path: $(pwd)"

find ./sesame -type f -name '*.proto' -exec "${cmd[@]}" {} +
