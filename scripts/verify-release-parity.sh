#!/usr/bin/env bash
set -euo pipefail

# A release without explicit phase flags still executes the *entire* gate.
# Phases make the same clean-commit, source-verified binary comparisons runnable
# under synchronous tool deadlines. One phase is never a release certificate.
case "${1:-}" in
  '') phase='all' ;;
  --phase=preflight) phase='preflight' ;;
  --phase=deepseek) phase='deepseek' ;;
  --phase=gemini) phase='gemini' ;;
  --phase=websocket) phase='websocket' ;;
  *) echo 'usage: verify-release-parity.sh [--phase=preflight|deepseek|gemini|websocket]' >&2; exit 2 ;;
esac
if [ "$#" -gt 1 ]; then
  echo 'release parity accepts at most one phase option' >&2
  exit 2
fi

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='ef7a965ed8a4996eb8288b1918533d395f65a132'
prodex_binary_sha256='94acc49aadd1be65c1578f2d2a7ca6288b87758106f245d7684a65dfef1f3d12'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.437.14 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.437.14" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.14/prodex-x86_64-unknown-linux-gnu'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/SHA256SUMS" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.14/SHA256SUMS'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/release-manifest.json" \
  'https://github.com/christiandoxa/prodex/releases/download/0.437.14/release-manifest.json'
grep -Fqx "$prodex_binary_sha256  prodex-x86_64-unknown-linux-gnu" "$tmp/SHA256SUMS" || {
  echo "Prodex executable is absent or mismatched in published SHA256SUMS" >&2
  exit 1
}
grep -Fq '"version": "0.437.14"' "$tmp/release-manifest.json" &&
grep -Fq '"commit": "ef7a965ed8a4996eb8288b1918533d395f65a132"' "$tmp/release-manifest.json" &&
grep -Fq '"asset": "prodex-x86_64-unknown-linux-gnu"' "$tmp/release-manifest.json" || {
  echo "Prodex release manifest does not describe the pinned oracle" >&2
  exit 1
}
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.437.14"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
go build -trimpath -o "$tmp/profileparity" ./tools/profileparity
go build -trimpath -o "$tmp/clierrorparity" ./tools/clierrorparity
go build -trimpath -o "$tmp/contextparity" ./tools/contextparity
go build -trimpath -o "$tmp/continuationparity" ./tools/continuationparity
go build -trimpath -o "$tmp/sessionfooterparity" ./tools/sessionfooterparity

if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ]; then
# Preserve the focused 0.437.10 goal-attachment recovery regressions while
# migrating the executable oracles to 0.437.11. The 0.437.11 session-release
# and full-history rotation regressions are qualified separately.
go test ./internal/gateway/codex -run 'Test(PersistSessionGoal|UpdateSessionGoal|MaintainManagedSessions.*Goal|PersistSessionGoalObjective|RunRuntimeDirectRepairsSelectedGoalBeforeChildStarts|RepairResumeSessionBeforeLaunch|CodexResumeSessionID)' -count=1 -timeout=30s

# Tagged 0.437.11 native-session shutdown must print saved-session resume
# guidance only after the owned socket closes, and restore inherited termios,
# alt-screen, keyboard, mouse, focus and bracketed-paste modes on every exit.
# These real PTY regressions fail closed when no PTY is available and verify
# queued input is never flushed during restoration.
go test ./internal/gateway/codex \
  -run '^TestProdex043711(SuccessfulSessionCompanionShowsResumeGuidanceAfterSocketClosed|SessionCompanionSuppressesGuidanceOnNonTTYAndFailure|ChildTerminalHandoffPreservesQueuedInputAndProtocols|NativeTUIRestoresTerminalAfterSuccessAndFailure|NonInteractiveChildNeverEmitsTerminalProtocols|OwnedCompanionRestoresTTYBeforeResumeGuidance)$' \
  -count=1 -timeout=75s

# Prodex 0.437.14 saved-session output must carry the launch-owned identity,
# preserve CRLF append-only boundaries, offer a picker when unknown, and only
# allocate identity hooks for interactive companion launches.
go test ./internal/gateway/codex \
  -run '^TestProdex043712' \
  -count=1 -timeout=30s

# Prodex 0.437.14 resets DEC 1047 (no DECRC) after native TUI exit,
# and the live runtime only accepts the exact terminal WebSocket idle timeout.
# Both fail-before regressions protect native output and recovery boundaries.
go test ./internal/gateway/codex \
  -run '^TestProdex043713TerminalHandoff' -count=1 -timeout=45s
go test ./internal/delivery/cli/runtime \
  -run '^TestProdex043713WebSocketIdle' -count=1 -timeout=45s

# Prodex 0.437.14 Retry-After text parser uses UInt128 bounds, rejects
# zero/signed values, rounds fractional millis, and caps at five minutes.
go test ./internal/usecase/routing -run '^TestProdex043714RetryAfter' -count=1 -timeout=30s

# The live observer must distinguish an absent goal from an unavailable DB,
# preserve unread failure evidence under EMFILE, and recover after resources
# return. The Linux low-RLIMIT case runs inside a dedicated child process.
go test ./internal/repository/runtime ./internal/delivery/cli/runtime \
  -run '^TestProdex043713(GoalStoreDistinguishes|UnavailableGoalDatabase|LiveObserverDescriptor|ObserverMarker)' \
  -count=1 -timeout=65s

# Prodex 0.437.14 does not allow delayed older successful quota probes
# to overwrite later confirmed observations and durable quota snapshots.
go test -race ./internal/usecase/quota \
  -run '^TestProdex043714LateSuccessfulProbe' -count=1 -timeout=60s

# The tagged 0.437.11 release-fence delta must remain fail-closed under stale
# snapshots, same-second rotation, seven-day retired-fence compaction, backup
# recovery, and invalid inputs. Only self-contained full-history Responses
# requests can repair a conflicting soft session. Native session searching
# must preserve Rust's Unicode lowercase semantics at the Mojo boundary.
# Runtime doctor must observe half-open route circuits without consuming a
# routing probe or mutating the persisted health snapshot.
go test ./internal/repository/routing ./internal/usecase/routing ./internal/usecase/session ./internal/usecase/runtime \
  -run '^TestProdex043711' -count=1 -timeout=60s

# The 0.437.11 Gemini bridge Mojo request kernel gives nested
# request.contents priority over root contents, and only a signature on a
# model part (not its nested functionCall) can suppress first-call hardening.
# Its signature presence check also includes U+001C–U+001F as whitespace.
go test ./internal/gateway/gemini \
  -run '^TestProdex043711GeminiHardening' -count=1 -timeout=40s

# Prodex 0.437.14 DeepSeek streamed tool arguments use tagged serde_json
# surrogate, depth and finite-number boundaries, independent of Go's more
# permissive escape and recursion handling. Verify real SSE terminal frames.
go test ./internal/gateway/deepseek \
  -run '^TestProdex043713DeepSeek' -count=1 -timeout=45s

# The 0.437.14 streamed-argument differential compares four exact-tagged
# real SSE tool-call envelopes. Reject extra empty arguments in item.added,
# unstable invalid-JSON diagnostics, bad Unicode/numeric/depth validation,
# failed completion ordering, and any extra/missing stream event.
go test ./tools/differential \
  -run '^TestProdex043713StreamToolDifferentialFixtureRejectsMutations$' \
  -count=1 -timeout=30s

# Independent Linux PTY footer oracle. It uses one synthetic Codex helper for
# both exact binaries and checks launch-owned identity, picker fallback, and
# all-stream TTY suppression without credentials or provider calls.
"$tmp/sessionfooterparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

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

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'deepseek' ]; then
# Separate real-provider DeepSeek mock/oracle, still fail-closed on all material
# mismatches including the private durable-file layout.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --scenario all --scenario-workers 3

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'gemini' ]; then
# Gemini API-key Super uses OpenAI-compatible Chat Completions transport,
# but native GenerateContent response semantics. Qualify both the outbound
# request and accepted/invalid native response, and require tagged native
# SSE chunks on that Chat-compatible route not to become false text deltas.
# Genuine Chat Completions SSE chunks must preserve created, item-added,
# text delta, item-done and completed events with exact usage/finish metadata.
# Multi-turn Gemini chat requests must reorder paired tool results ahead of
# later user content and drop unanswered calls, never duplicating an output.
# Gemini plaintext 429 must pass through without rotation (child exit 2),
# while structured quota JSON must exhaust the exact five-model first-key
# chain before the second key's successful initial-model response (exit 0).
# The response UUIDv7 namespace and absent created_at are mandatory.
# Independent route, auth, model, usage, framing, tool-call adjacency,
# key-rotation, exit-code, item ID and response mutation sentinels fail closed.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --gemini

# The 0.437.14 Gemini native envelope must unwrap `response` with or
# without traceId. The API-key buffered bridge must generate its own UUIDv7
# and must not expose stale native/trace IDs. These cases are mandatory parts
# of the full --gemini differential above.
go test ./internal/gateway/gemini ./tools/differential \
  -run '^TestProdex043713Gemini' -count=1 -timeout=50s

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'websocket' ]; then
# Exact-tagged Responses WebSocket executable differential. A synthetic
# heartbeat followed by capacity error must remain precommit; actual output
# must never be replayed; trusted previous_response_id outranks soft session.
# The fixture verifies that Godex was built from this clean commit.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket

fi

# Printed only after all stages *selected for this invocation* succeeded.
# --phase=... must never be mistaken for the default complete release gate.
printf 'PASS: Prodex 0.437.14 parity phase %s (Godex %s)\n' "$phase" "$(git rev-parse HEAD)"
