# Prodex 0.437.9 behavioral parity checkpoint

Canonical reference: tag `0.437.9`, commit
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`.
The official Linux x86_64 release executable has SHA-256
`58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`,
matching its published `SHA256SUMS` entry. Both the exact source and the
binary are required by the active independent differential tools and the
release-only parity script. This replaces the prior 0.437.8 oracle target;
historical checkpoint evidence remains in `PARITY-04378.md`.

## Behavioral changes addressed

1. **Responses keepalive before output.** The exact `keepalive` event is
   transport liveness, not committed model content. Godex now keeps it in
   precommit lookahead for HTTP SSE and upstream Responses WebSocket. A later
   retryable capacity failure can rotate safely, dropping the failed prelude.
   Actual output, unknown event kinds, and a previously committed stream still
   prohibit replay. Regression tests failed on the preceding Go implementation
   and pass with the new classification. The HTTP loopback test exercises both
   declared SSE and missing upstream Content-Type with unchanged request body,
   account rotation, and post-output negative controls. Byte-split SSE tests
   verify every split of the synthetic heartbeat/overload sequence; the
   WebSocket fixture exercises its real framed upstream protocol boundary.

2. **Root native session marker.** Codex SessionStart's `session_id` now
   takes precedence over legacy notify's concrete `thread-id` when both
   appear. A legacy callback bootstraps an absent marker but cannot redirect
   an established root; a repeated root is a no-op, and an explicit new root
   replaces it. The trusted ephemeral marker is serialized under an owned
   OS lock and atomically published as a private file. The Go monitor's
   in-memory unread scan offset is not reset by repeated callbacks. Synthetic
   regressions cover conflicting identifiers, malformed canonical identity,
   explicit root changes, and simultaneous descendant callbacks. A later
   checkpoint also stores a private logical rollout offset when the trusted
   SessionStart provides a verified transcript path; see the follow-up below.
   Full cross-process monitor restart equivalence is not yet established.

No user-visible feature, Codex model context window, compaction setting,
credential handling, or postcommit retry policy was changed.

## Official executable qualification

All runs below used isolated local homes, synthetic credentials, no live
model inference, and official Prodex 0.437.9. Godex candidate
`0b3f1b80da5a2bd06ec4d243332e5d183a7c744e` was built from a clean
committed checkout with Go VCS provenance.

- Managed profile lifecycle oracle: **PASS**, 31 subprocess stages.
- CLI rejection oracle: **PASS**, eight probes and negative controls.
- Native model-context oracle: **PASS**, three explicit cases and cached-model
  negative control.
- Responses continuation loopback: **PASS**, observed account attempts,
  overload, quota, cancellation, and commitment protections.
- Strict DeepSeek mock differential: **PASS** for each of 18 scenarios
  invoked individually (success, tools, user-id, streamed output/limits,
  retry, 401/403/429/500/503, credential rotation/restart, embedded
  stream failure, cancellation, restart, and recovery after 429/503).
  Each invocation retained its strict negative controls and state auditor.
  The combined `--scenario all` invocation was not executed here; do not
  infer that the combined release script passed.
- `go test ./internal/usecase/routing ./internal/gateway/openai -run TestProdex04379 -count=1 -timeout=20s`: **PASS** after failing-before regressions.
- `go test ./internal/delivery/cli/runtime -count=1 -timeout=75s`: **PASS**.
- `go test ./internal/delivery/http/proxy -run TestProdex04379PublicResponsesHeartbeatRecoveryAndCommitFence -count=1 -v -timeout=30s`: **PASS**.
- Focused shuffled `-race` package tests for routing/OpenAI gateway,
  CLI runtime, and HTTP proxy: **PASS**.
- `make verify`: **PASS** (format, source size, Bubble Tea guard, six
  cross-build targets, installers, vet, shuffled race tests and Go build).

## Remaining release blockers

This checkpoint is **not full 1:1 feature parity**. Cross-binary live-goal
recovery with native SessionStart and inherited descendant hooks, independent
rollout-offset durability, cross-process state recovery, WebSocket message
deadline/ownership qualification, broader provider protocols, live OAuth and
quota interactions, and cross-platform native execution still require
independent behavioral evidence. Mock differential agreement for a subset of
contracts is not equivalent to all material behavior.

The `v0.2.1` release gate remains fail-closed. No release or tag is authorized
by these tests, and CI success is not by itself a parity certificate.

## SessionStart pre-poll rollout cursor follow-up

A separate deterministic regression reproduced an unobserved native overload:
Codex emitted `SessionStart`, then accepted user work and a terminal capacity
error before Godex's live monitor first resolved the rollout. Previously the
monitor captured the *current* file end and skipped that terminal error.
The new callback snapshots the logical file position before atomically
publishing a new root marker when a verified native `transcript_path` is
available. Plain JSONL uses the regular file length; bounded `.jsonl.zst`
decoding uses the *decoded* length. The monitor consumes a private offset
sidecar under the marker writer lock and verifies the original prefix before
scanning subsequently appended records.

The failing-before regression now passes for both plain and recompressed
rollouts, with the error arriving before the first monitor poll. Negative
controls reject foreign session metadata, a different rollout UUID, symlinks,
negative/malformed/oversized offsets, offsets in the middle of a JSONL line,
and offsets beyond the current logical file end. Historical failures preceding
SessionStart remain ignored; repeated root/descendant notifications preserve
the sidecar, and an explicit root switch with no verifiable rollout clears it.
No event, original prompt, or completed tool is replayed by the callback.

This closes the **verified-transcript startup cursor** gap only. If the
canonical native hook omits its transcript path, or the path is unavailable
during the callback, the monitor conservatively captures the current end
rather than scanning unknown historical state. The root marker remains
launch-private and is cleaned up on exit. Independent cross-binary evidence
for full state-index resolution, arbitrary monitor/process restarts, and
every compressed-rollout transition remains open and release-blocking.

## Native Codex 0.162.1 SessionStart qualification

An opt-in, credential-free test now drives the actual installed Codex 0.162.1
Linux native executable with a synthetic, loopback-only Responses provider.
It disables Codex plugins, uses isolated `HOME` and `CODEX_HOME`, builds the
Godex hook receiver from source, and relays the native `SessionStart` JSON
through Godex's hidden callback. The test verifies the exact native event
name, session ID, matching rollout path, and snapshot cursor. No external
model or account request is made.

The first real-native run exposed a regression the earlier fabricated rollout
fixture missed: Codex creates legitimate group-writable rollout files
(mode `0664`), so rejecting every group-writable file prevented the cursor
from being recorded. Godex now accepts verified regular Codex rollouts that
are group-writable, while continuing to reject world-writable files and
symlinks. Deterministic positive and negative tests cover modes `0664`
and `0666` for both JSONL and compressed rollouts. The native opt-in
fixture failed before this permission adjustment and passed afterward.

Opt-in validation:

```sh
GODEX_TEST_CODEX_01621_BIN=<native-codex-binary> \
  go test ./internal/delivery/cli/runtime \
    -run '^TestProdex04379OfficialCodex01621NativeSessionStartCursor$' -count=1 -v
```

This qualifies the installed binary by reported version, not by an official
SHA-256-pinned asset or a full Prodex-vs-Godex running-model differential.
The release gate remains fail-closed.

The same opt-in native fixture also exercises the complete local recovery path:
after the real Codex 0.162.1 child exits, it appends synthetic accepted-user
and terminal `server_overloaded` records to **that Codex-created rollout**
before the Godex monitor's first read. The monitor then reports the exact
native root as `usage_limited`. This establishes hook-to-marker-to-cursor-to-
rollout observation with a real Codex-generated session layout; the terminal
error is intentionally synthetic, not evidence of a real upstream outage or
of Prodex/Godex equivalence for every live model turn.

## WebSocket request-scoped precommit watchdog follow-up

The tagged Prodex 0.437.9 WebSocket message boundary maps an expired
request-scoped precommit deadline to an error frame with status `504`,
code `request_timeout`, and message `request timed out`, while preserving
the already-upgraded *local* WebSocket connection for later messages. Before
this change, Godex's upstream WebSocket frame reader could remain blocked
until its independent eight-second idle timer fired despite the logical
request being cancelled. Separately, Godex exposed its internal 504 as
`service_unavailable`, with a longer internal diagnostic, instead of the
canonical WebSocket error code and message.

The upstream OpenAI gateway now owns a cancellation hook for each
precommit WebSocket message. Cancelling that context promptly closes the
pending upstream socket and unblocks both frame reads and writes. The hook
is disarmed as the message returns; a real committed response keeps its
upstream connection readable after the request's precommit context ends.
Cancellation does not trigger account rotation, quota penalty, or replay
of committed output. Only the exact typed local watchdog failure maps to
the native `request_timeout` frame. An unrelated upstream 504 keeps its
own diagnostic instead of impersonating a local watchdog.

Regressions first failed against the previous implementation and then
passed after the two small patches:

- `TestProdex04379WebSocketPrecommitCancelledReadReleasesUpstream`
  uses a real duplex socket stalled before any output and proves cancellation
  releases it promptly instead of waiting for the idle timer.
- `TestProdex04379RealWebSocketMessagePrecommitWatchdogReturns504`
  exercises the real gateway through the routing watchdog with a short
  fixture deadline; it returns a typed 504 and closes the pending upstream.
- `TestProdex04379CommittedWebSocketSurvivesRequestContextCancellation`
  confirms real visible output and a later terminal event still arrive after
  the precommit context is cancelled.
- `TestProdex04379PublicWebSocketTimeoutKeepsSessionAlive` exercises two
  logical requests over one locally upgraded WebSocket: two canonical 504
  frames, or one canonical 504 followed by a committed successful response.
  `TestProdex04379WebSocketLocalWatchdogErrorHasNativeCode` covers both
  delivery mappings and rejects the unrelated-upstream-504 false positive.

`go test -race -shuffle=on -count=1 ./internal/delivery/http/proxy
./internal/gateway/openai ./internal/usecase/routing` and the full
`make verify` gate passed after both patches (format/source-size, six
cross-builds, installer smoke, vet, and shuffled race tests).

These are synthetic local upstream and local client fixtures matched to
the tagged Prodex source/test contract; they are not a full binary-to-binary
WebSocket differential. Full hard-affinity owner recovery, native remote
WebSocket handoff and live provider quota/capacity behavior remain
release-blocking.
