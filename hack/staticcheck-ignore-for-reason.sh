#!/bin/sh

if ! { [ "$#" -eq 2 ] && ! [ -z "$1" ] && ! [ -z "$2" ]; }; then
  echo "Usage: $0 <code> <reason>"
  exit 1
fi

code="$1" &&
  shift &&
  reason="$1" &&
  shift &&
  lines="$(hack/staticcheck-lines-for-code.sh "$code")" &&
  hack/insert-line-files.sh "//lint:ignore $code $reason" ${lines}
