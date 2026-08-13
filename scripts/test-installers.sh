#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'installer test: %s\n' "$*" >&2
  exit 1
}

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
project_dir="$(CDPATH= cd -- "$script_dir/.." && pwd)"
fixture_root="$(mktemp -d "${TMPDIR:-/tmp}/godex-installer-test.XXXXXX")"
trap 'rm -rf "$fixture_root"' EXIT HUP INT TERM

case "$(uname -s)" in
Linux) os_name="linux" ;;
Darwin) os_name="darwin" ;;
*) fail "unsupported test operating system" ;;
esac
case "$(uname -m)" in
x86_64|amd64) arch="amd64" ;;
arm64|aarch64) arch="arm64" ;;
*) fail "unsupported test architecture" ;;
esac

release_dir="$fixture_root/releases/v0.0.1"
source_dir="$fixture_root/source"
success_dir="$fixture_root/success"
failure_dir="$fixture_root/failure"
mkdir -p "$release_dir" "$source_dir"

printf '%s\n' '#!/bin/sh' 'if [ "${1:-}" = "--version" ]; then printf "godex synthetic\n"; fi' > "$source_dir/godex"
chmod 0755 "$source_dir/godex"
archive="godex_0.0.1_${os_name}_${arch}.tar.gz"
tar -czf "$release_dir/$archive" -C "$source_dir" godex
(
  cd "$release_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$archive" > checksums.txt
  else
    shasum -a 256 "$archive" > checksums.txt
  fi
)

GODEX_VERSION=0.0.1 \
GODEX_RELEASE_BASE_URL="file://$fixture_root/releases" \
GODEX_INSTALL_DIR="$success_dir" \
"$project_dir/install.sh" >/dev/null
[ -x "$success_dir/godex" ] || fail "verified archive was not installed"

printf 'tampered' >> "$release_dir/$archive"
if GODEX_VERSION=0.0.1 \
  GODEX_RELEASE_BASE_URL="file://$fixture_root/releases" \
  GODEX_INSTALL_DIR="$failure_dir" \
  "$project_dir/install.sh" >"$fixture_root/mismatch.log" 2>&1; then
  fail "checksum mismatch was accepted"
fi
grep -q 'SHA-256 mismatch' "$fixture_root/mismatch.log" || fail "checksum failure was not reported"
[ ! -e "$failure_dir/godex" ] || fail "binary was installed after checksum failure"

printf 'Installer smoke and checksum rejection: passed\n'
