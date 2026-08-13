#!/usr/bin/env bash
set -euo pipefail

unformatted="$(gofmt -l cmd internal)"
if [[ -n "$unformatted" ]]; then
  echo "Go files need gofmt:" >&2
  echo "$unformatted" >&2
  exit 1
fi
