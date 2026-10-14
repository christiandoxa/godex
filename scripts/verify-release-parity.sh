#!/usr/bin/env bash
set -euo pipefail

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='83ae4cb18ca8000de7cb563291539392c71444e0'
prodex_binary_sha256='634f67882045afc4cc74b3046b784b33618ff8e14a1009fd9d1bc44d699746fa'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.437.11 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.437.11" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.11/prodex-x86_64-unknown-linux-gnu'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/SHA256SUMS" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.11/SHA256SUMS'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/release-manifest.json" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.11/release-manifest.json'
grep -Fqx "$prodex_binary_sha256  prodex-x86_64-unknown-linux-gnu" "$tmp/SHA256SUMS" || {
  echo "Prodex executable is absent or mismatched in published SHA256SUMS" >&2
  exit 1
}
grep -Fq '"version": "0.437.11"' "$tmp/release-manifest.json" &&
grep -Fq '"commit": "83ae4cb18ca8000de7cb563291539392c71444e0"' "$tmp/release-manifest.json" &&
grep -Fq '"asset": "prodex-x86_64-unknown-linux-gnu"' "$tmp/release-manifest.json" || {
  echo "Prodex release manifest does not describe the pinned oracle" >&2
  exit 1
}
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.437.11"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
go build -trimpath -o "$tmp/profileparity" ./tools/profileparity
go build -trimpath -o "$tmp/clierrorparity" ./tools/clierrorparity
go build -trimpath -o "$tmp/contextparity" ./tools/contextparity
go build -trimpath -o "$tmp/continuationparity" ./tools/continuationparity

# Preserve the focused 0.437.10 goal-attachment recovery regressions while
# migrating the executable oracles to 0.437.11. The 0.437.11 session-release
# and full-history rotation regressions are qualified separately.
go test ./internal/gateway/codex -run 'Test(PersistSessionGoal|UpdateSessionGoal|MaintainManagedSessions.*Goal|PersistSessionGoalObjective|RunRuntimeDirectRepairsSelectedGoalBeforeChildStarts|RepairResumeSessionBeforeLaunch|CodexResumeSessionID)' -count=1 -timeout=30s

# Tagged 0.437.11 native-session app-server shutdown must print saved-session
# resume guidance only on a real interactive terminal after socket teardown.
# The PTY regression fails closed when the release environment lacks a PTY.
go test ./internal/gateway/codex \
  -run '^TestProdex043711(SuccessfulSessionCompanionShowsResumeGuidanceAfterSocketClosed|SessionCompanionSuppressesGuidanceOnNonTTYAndFailure)$' \
  -count=1 -timeout=60s

# The tagged 0.437.11 release-fence delta must remain fail-closed under stale
# snapshots, same-second rotation, seven-day retired-fence compaction, backup
# recovery, and invalid inputs. Only self-contained full-history Responses
# requests can repair a conflicting soft session. Native session searching
# must preserve Rust's Unicode lowercase semantics at the Mojo boundary.
# Runtime doctor must observe half-open route circuits without consuming a
# routing probe or mutating the persisted health snapshot.
go test ./internal/repository/routing ./internal/usecase/routing ./internal/usecase/session ./internal/usecase/runtime \
  -run '^TestProdex043711' -count=1 -timeout=60s

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

# Native Codex context forwarding must match the tagged executable. This
# isolated fake-Codex fixture rejects cached maximum-window promotion and
# checks the child argv rather than only the parent CLI's accepted options.
"$tmp/contextparity" \
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

# Exact-tagged Responses WebSocket executable differential. A synthetic
# heartbeat followed by capacity error must remain precommit; actual output
# must never be replayed; trusted previous_response_id outranks soft session.
# The fixture verifies that Godex was built from this clean commit.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket
