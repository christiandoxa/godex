#!/usr/bin/env bash
set -euo pipefail

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
project_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
output_dir="$(mktemp -d "${TMPDIR:-/tmp}/godex-cross-build.XXXXXX")"
trap 'rm -rf "$output_dir"' EXIT HUP INT TERM

cd "$project_dir"
for target in \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64 \
  windows/arm64; do
  IFS=/ read -r goos goarch <<<"$target"
  output="$output_dir/godex_${goos}_${goarch}"
  if [[ "$goos" == windows ]]; then
    output+='.exe'
  fi
  echo "Building $goos/$goarch"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -o "$output" ./cmd/godex
  test -s "$output"
done

# Production cross-builds do not compile _test.go files. Guard test-only
# build tags and platform-specific helpers before the remote CI matrix.
for target in darwin/arm64 windows/amd64; do
  IFS=/ read -r goos goarch <<<"$target"
  for package in \
    internal/delivery/cli/superexpose \
    internal/gateway/codex \
    internal/usecase/runtime; do
    binary="${package//\//_}"
    output="$output_dir/${binary}_${goos}_${goarch}.test"
    if [[ "$goos" == windows ]]; then
      output+='.exe'
    fi
    echo "Compiling tests $goos/$goarch $package"
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
      go test -c -o "$output" "./$package"
    test -s "$output"
  done
done

echo 'Cross-build and cross-test compile matrix: passed'
