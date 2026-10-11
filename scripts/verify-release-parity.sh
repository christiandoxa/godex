#!/usr/bin/env bash
set -euo pipefail

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='03815a6157e7a67e83e38fc2a79bd679cc988fa0'
prodex_binary_sha256='834275c168121ce181eb7a7ff0b730b392c23e853803b6dea4a27e6166a6d074'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.437.3 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.437.3" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.3/prodex-x86_64-unknown-linux-gnu'
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.437.3"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
go build -trimpath -o "$tmp/profileparity" ./tools/profileparity

# Independent managed-profile persistence oracle. All 29 CLI actions run in
# separate subprocesses with isolated credential-free homes; state projections
# must match across creation, active switching, rejected writes and deletion.
"$tmp/profileparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

# Separate real-provider DeepSeek mock/oracle, still fail-closed on all material
# mismatches including the private durable-file layout.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --scenario all
