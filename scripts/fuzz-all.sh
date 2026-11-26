#!/usr/bin/env bash
# Runs every native Fuzz target of the module for the given duration each.
# Part of `make deep`, the asynchronous verification stage.
set -euo pipefail

duration="${1:-30s}"

for pkg in $(go list ./...); do
  for target in $(go test -list '^Fuzz' "$pkg" | grep '^Fuzz' || true); do
    echo "fuzz $pkg $target for $duration"
    go test -run '^$' -fuzz "^${target}\$" -fuzztime "$duration" "$pkg"
  done
done
