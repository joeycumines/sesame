#!/bin/sh

if ! { [ "$#" -eq 1 ] && ! [ -z "$1" ]; }; then
  echo "Usage: $0 <check>"
  exit 1
fi

staticcheck -checks "$1" ./... | tee /dev/stderr | grep -F -- "$1" | cut -d : -f 1-2 | uniq | tac
