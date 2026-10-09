#!/usr/bin/env bash
set -euo pipefail

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='b70f7429fb163760e3cd35a6564a0e79f9281a49'
prodex_binary_sha256='0082ed1348183cc53b0dc4ad44d9f6a5e009d3d3dcc8bbf372bc2eec60447d76'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.437.0 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.437.0" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.0/prodex-x86_64-unknown-linux-gnu'
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.437.0"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --scenario all
