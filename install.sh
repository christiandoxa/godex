#!/bin/sh
set -eu

repository="${GODEX_REPOSITORY:-christiandoxa/godex}"
install_dir="${GODEX_INSTALL_DIR:-$HOME/.local/bin}"
release_base="${GODEX_RELEASE_BASE_URL:-https://github.com/$repository/releases/download}"
version_input="${GODEX_VERSION:-}"

fail() {
  printf 'godex installer: %s\n' "$*" >&2
  exit 1
}

require() {
  command -v "$1" >/dev/null 2>&1 || fail "required command '$1' was not found"
}

require curl
require uname
require tar

if [ -z "$version_input" ]; then
  latest_url="https://github.com/$repository/releases/latest"
  effective_url="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$latest_url")" || fail "cannot resolve latest release"
  tag="${effective_url##*/}"
  [ -n "$tag" ] || fail "latest release did not contain a tag"
else
  case "$version_input" in
    v*) tag="$version_input" ;;
    *) tag="v$version_input" ;;
  esac
fi
case "$tag" in
  v[0-9]*)
    case "$tag" in
      *[!v0-9A-Za-z.+-]*) fail "release tag is invalid" ;;
    esac
    ;;
  *) fail "release tag is invalid" ;;
esac
version="${tag#v}"

case "$(uname -s)" in
  Linux) os_name="linux" ;;
  Darwin) os_name="darwin" ;;
  *) fail "unsupported operating system: $(uname -s)" ;;
esac

case "$(uname -m)" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) fail "unsupported architecture: $(uname -m)" ;;
esac

archive="godex_${version}_${os_name}_${arch}.tar.gz"
temporary="$(mktemp -d 2>/dev/null || mktemp -d -t godex)"
staged_destination=""
cleanup() {
  [ -z "$staged_destination" ] || rm -f "$staged_destination"
  rm -rf "$temporary"
}
trap cleanup EXIT HUP INT TERM

curl -fsSL "$release_base/$tag/$archive" -o "$temporary/$archive" || fail "cannot download $archive"
curl -fsSL "$release_base/$tag/checksums.txt" -o "$temporary/checksums.txt" || fail "cannot download checksums.txt"

expected="$(awk -v file="$archive" '$2 == file { print $1; exit }' "$temporary/checksums.txt")"
[ -n "$expected" ] || fail "checksum for $archive was not found"

if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/$archive" | awk '{print $1}')"
elif command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$temporary/$archive" | awk '{print $1}')"
else
  fail "sha256sum or shasum is required"
fi
[ "$actual" = "$expected" ] || fail "SHA-256 mismatch for $archive"

tar -xzf "$temporary/$archive" -C "$temporary"
[ -f "$temporary/godex" ] || fail "archive does not contain godex"
chmod 0755 "$temporary/godex"
version_output="$("$temporary/godex" --version 2>/dev/null)" || fail "downloaded binary failed its version check"
case "$version_output" in
  "godex $version" | "godex $version "*) ;;
  *) fail "downloaded binary reported an unexpected version" ;;
esac

mkdir -p "$install_dir"
destination="$install_dir/godex"
staged_destination="$destination.tmp.$$"
cp "$temporary/godex" "$staged_destination"
chmod 0755 "$staged_destination"
mv -f "$staged_destination" "$destination"
staged_destination=""

printf 'Installed %s to %s\n' "$("$destination" --version)" "$destination"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    printf 'Add %s to PATH, for example:\n  export PATH="%s:$PATH"\n' "$install_dir" "$install_dir"
    ;;
esac
