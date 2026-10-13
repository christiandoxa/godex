#!/usr/bin/env bash
set -euo pipefail

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='8c5c6f40fb4f864d1542f9c8fa48d15863316652'
prodex_binary_sha256='49de897a147c1ab7ec7aebdcd6d8fd2075cb0b6de58487062eedc9be3da4eb1e'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.437.8 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.437.8" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.8/prodex-x86_64-unknown-linux-gnu'
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.437.8"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
go build -trimpath -o "$tmp/profileparity" ./tools/profileparity
go build -trimpath -o "$tmp/clierrorparity" ./tools/clierrorparity

# Independent managed-profile persistence oracle. All 29 CLI actions run in
# separate subprocesses with isolated credential-free homes; state projections
# must match across creation, active switching, rejected writes and deletion.
"$tmp/profileparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

# CLI argument rejection oracle. This catches delivery paths that accidentally
# discard user arguments before reaching their owning command parser.
"$tmp/clierrorparity" \
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
