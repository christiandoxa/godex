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

## Cross-process native marker arbitration

A new Linux subprocess fixture invokes the **actual built Godex callback
executable** from multiple overlapping processes rather than simulating
concurrent notifications with goroutines in one process. It races canonical
root SessionStart payloads (with a matching rollout transcript) against
inherited legacy descendant thread notifications, then repeats both forms
after a native-style terminal capacity error is appended but before the
monitor reads its first tail. The root identity and original logical cursor
remain unchanged, and the monitor observes the unread `server_overloaded`
failure. This exercises OS-level lock ownership and atomic cursor/marker
publication without real accounts or external requests.

The following command passed:

```sh
go test -race -count=3 ./internal/delivery/cli/runtime \
  -run '^TestProdex04379NativeMarkerConcurrentProcessesPreserveUnreadFailure$' -timeout=60s
```

This qualifies competing callback **processes**, not a persisted monitor
restart or a cross-binary full native-session recovery flow; those release
checks remain open.

## Pinned executable WebSocket differential (follow-up)

The first isolated **binary-to-binary** WebSocket oracle now runs both the
official SHA-256-pinned Prodex `0.437.9` executable and a clean committed
Godex build against the same synthetic two-profile loopback backend.
The release-only verifier registers the strict `--websocket` mode of
`tools/continuationparity` after its existing profile, CLI, and DeepSeek
oracles. No real credentials or model requests are used.

This oracle exposed a production-path parity gap that simpler Go tests
had missed: the public Responses WebSocket ingress left `QuotaSelection`
unset. Therefore, a verified hard `previous_response_id` owned by account
A plus a conflicting soft `session_id` owned by account B could return a
local `409 stale_continuation` before dispatch, while canonical Prodex
routed the message to account A. Setting the proper WebSocket route on
ingress fixed that disagreement without changing the HTTP Responses path.
The later 0.437.9 unbound-discovery checkpoint below supersedes the initial
assumption that unknown previous-response IDs must be rejected *before*
upstream. Missing-chain failures still remain fail-closed.

With candidate commit `299ef5509a761a492fbe04c40b3b4dd42c073b79`,
the exact official binaries agreed on these **three** behaviors:

| Synthetic scenario | Official Prodex attempts/events | Godex attempts/events |
| --- | --- | --- |
| `keepalive` then precommit overload | account-a twice; `response.output_text.delta`, `response.completed` | Same |
| `keepalive`, visible output then overload | account-a once; `keepalive`, `response.output_text.delta`, `response.failed` | Same |
| Hard previous-response A conflicting with soft session B | account-a once; `response.completed` | Same |

The official reference retries the *same account* on the first scenario.
Therefore two attempts demonstrate safe precommit retry, **not**
cross-account rotation. The checks reject leaked precommit response IDs,
heartbeat exposure after a failed attempt, event order drift, missing
visible output, a second attempt after commitment, and loss of the
verified hard-affinity owner. Negative-control tests deliberately
introduce those defects and ensure the oracle rejects them.

The oracle validates the Prodex source commit and executable SHA-256,
and now requires a clean Godex source commit matching both the requested
40-character Git SHA and the built binary's Go VCS metadata with
`vcs.modified=false`. An intentionally wrong commit was rejected before
starting either product. The original HTTP continuation mode still
**PASS**es independently after the harness refactor; `make verify`
also **PASS**es (format, source size, Linux/macOS/Windows cross-builds,
installers, vet, shuffled race tests).

This **does not** establish full WebSocket or project parity. In
particular, the official executable comparison does not yet cover
durable WebSocket restart ownership, all quota/circuit phases, live
upstream WebSocket connection handoff, arbitrary hard-affinity failure
paths, or cross-platform execution. The complete networked
`scripts/verify-release-parity.sh` was not executed in this follow-up,
so its multi-oracle release gate remains unclaimed and fail-closed.

## Durable WebSocket hard owner across a gateway restart

An additional release-oracle WebSocket scenario now **terminates the running
gateway process and starts a new one** between two independent local WebSocket
connections. It replays the same synthetic conflict between a verified
`previous_response_id` owned by account-a and an unrelated soft
`session_id` owned by account-b. The upstream attempt must still go to
account-a with exactly one committed `response.completed` event; the soft
session's ownership must not override the harder continuation.

Against the exact official Prodex 0.437.9 binary and clean Godex
`d13fb9e3e4a23a6e302d760c7557e7fc7a5d7e04`, all **four**
WebSocket fixture scenarios passed for each executable. Observed attempts:
Prodex `2 + 1 + 1 + 1`; Godex `2 + 1 + 1 + 1`. In particular,
`ws-bound-restart` routed to account-a for **both**, confirming persistence
across a gateway **process** boundary rather than only WebSocket reuse. The
two products continued to agree on precommit retry and postcommit no-replay.
The scenario is now part of the pinned release-only differential.

This strengthens evidence for durable **previous-response** ownership in
one deterministic two-profile fixture. It does not certify every storage
failure/recovery permutation, turn-state-only recovery, cross-platform
WebSocket state durability, quota collapse, or real-provider execution.
Full parity remains fail-closed.

## Durable turn-state WebSocket owner across gateway restart

The strict pinned executable differential now checks **six** cases, adding
a hard `x-codex-turn-state: turn-a` binding owned by account-a while the
same request carries a conflicting soft `session_id: sess-conflict`
owned by account-b. This is tested on a fresh local WebSocket before
restart and again on a new WebSocket **after** the gateway process is
stopped and restarted. Both the official SHA-256-pinned Prodex 0.437.9
executable and the clean Godex `d13fb9e3e4a23a6e302d760c7557e7fc7a5d7e04`
reference build made exactly one account-a attempt in each case and
returned `response.completed`. The overall observation recorded 15
backend attempts across both binaries (14 WebSocket requests plus one
Prodex quota/usage probe).

The public proxy regression additionally checks a direct turn-state
property, a `client_metadata` turn-state property, an unknown opaque
turn-state (which may be discovered on one selected owner), and a
soft-session-only request (whose owner can depend on the request history).
The hard turn-state owner and the soft session owner both remain intact;
neither can be overwritten by the other. This confirms one deterministic durable routing contract,
**not** all turn-state/protocol recovery paths. No live credentials,
real provider inference, or release tags are involved.

## Unbound WebSocket discovery: official executable requalification

The tagged 0.437.9 binary permits a bounded upstream *discovery* attempt
when a WebSocket request carries an opaque `previous_response_id` or
`x-codex-turn-state` for which the durable store has no verified owner.
Godex previously returned a local `409 stale_continuation` before reaching
the provider. The change in commit `c88c89e` now selects one eligible
candidate, leaving the original request intact. Discovery does **not**
authorize cross-account replay of a missing conversation chain: when
`previous_response_not_found` follows a tool-output continuation,
same-owner retries remain bounded (the existing 75/200/500 ms policy),
and then a stale-continuation error is returned.

Against official Prodex 0.437.9 and clean Godex commit
`c88c89eee3d84ea1f29cdebe878fd3f6c042e803`, an independent
WebSocket differential passed **11 request cases per executable**:
initial soft-session observation, heartbeat/precommit retry, postcommit
no-replay, hard previous-response and hard turn-state precedence,
unknown previous-response and unknown turn-state discovery, subsequent
soft-session observation, and three follow-ups after full gateway restart.
The fixture recorded **25 upstream attempts across the two binaries**
(including Prodex's background usage request). All WebSocket requests
went to account-a on *both* executables. This is the reference's observed
ordering on these synthetic accounts, not a general guarantee about
unbound or soft-session placement.

Negative controls verify that a tool-output discovery attempt preserves
the original body and a missing-chain precommit failure never causes a
retry on a second account. The updated differential checks
both upstream owner identities and client event order, rejecting a false
parity pass when attempt counts agree but owners differ. This is still
a subset of live provider/WebSocket and durability behavior. The full
networked release verifier was **not** executed in this checkpoint; parity
and release remain fail-closed.


## Official unbound missing-chain error-frame parity

The next exact-tagged differential case exposed a client-wire mismatch
after the bounded retry sequence. Prodex `0.437.9` returned a **generic**
WebSocket `error` frame with `status=409` and
`error.code=stale_continuation` when an unbound
`previous_response_id` carried a tool output and the selected upstream
returned `previous_response_not_found`. Godex previously used a nested
`response.failed` envelope despite matching the owner's four attempts.
Commit `1b722ab` now emits the canonical generic error **only when the
previous-response owner is unbound**. The previously verified owner's
`response.failed` envelope remains unchanged; its public regression
passed unchanged.

Against official SHA-256-pinned Prodex 0.437.9 and clean Godex commit
`1b722ab05e892bececba47dcd12842d02c169ff6`, the WebSocket
differential passed **12 scenarios per executable**: the earlier eleven,
plus the unbound missing-chain tool-output scenario. Both sides made
**four same-owner attempts** for that case, returned exactly one
`409 / stale_continuation` client-visible `error` frame, and left the
original `previous_response_id`, session ID, tool-call ID, tool type,
and output intact on each upstream request. In all, the synthetic
fixture observed **33 upstream requests across the two executables**,
including Prodex's background usage query.

The oracle additionally rejects the wrong HTTP status, error code, event
type, account identity, or altered opaque tool fields. Local race tests
covering OpenAI gateway, HTTP proxy and routing passed after the runtime
fix. This qualifies the tested path only: live-provider discovery,
connection-limit recovery, alternate quota phases and cross-platform
ownership remain open; no release tag has been issued.


## Verified-bound tool-output missing chain: native error envelope parity

A thirteenth executable differential case revealed a second envelope
difference. With a **verified** previous-response owner, a completed tool
output, and an upstream `previous_response_not_found` response, tagged
Prodex 0.437.9 performs the original owner attempt plus three short retries
on that *same owner*. It then emits a **generic** WebSocket
`error / 409 / stale_continuation` client frame. Before the fix, Godex
retained the original nested `response.failed` envelope for this locked
tool-output continuation, although attempt count and owner already matched.

Commit `4c30360` now uses the generic stale frame when the request
semantically **requires locked previous-response affinity**, regardless
of whether a trusted binding exists. In contrast, ordinary verified
previous-response continuations **without** that tool-output dependency
still retain their original nested `response.failed` envelope; the
existing known-owner regression continues to pass. A new routing
regression verifies four same-owner attempts with unmodified tool-output
payload, delays of 75/200/500 ms, status 409 and code
`stale_continuation`.

The extended pinned executable oracle **PASS**ed on **13/13 scenarios
for both** official Prodex 0.437.9 and clean Godex
`4c30360a0e9e515e25714c5d5ff2bf26d9f6c7ef`. All four upstream
attempts in the verified-bound missing-chain case preserved
`previous_response_id`, `session_id`, `call_id`, the
`function_call_output` event type and its `output` content, with
no cross-profile replay. The combined fixture recorded **41 upstream
requests** (40 WebSocket message attempts across both executables plus
Prodex's usage preflight query). The oracle now rejects mismatched client
event type, status, error code, owner, attempt count, and mutated tool fields.

These checks qualify bounded hard-affinity failure handling only.
Live provider inference, all quota/circuit states, app-server handoff and
cross-platform operation remain outside the demonstrated parity surface.
No release tag is authorized by the new evidence.

## Native SessionStart cursor at partial JSONL record boundaries

A further deterministic regression exposed a gap in the pre-poll recovery
cursor when native Codex invokes `SessionStart` **during an unfinished JSONL
write**. The callback previously captured the physical / decoded file length
even if the final record ended halfway through a line. The monitor rejects
such a cursor as an invalid record boundary and falls back to the *current*
file end, which can silently skip a terminal capacity failure completed
before the first monitor poll.

Commit `ca4d437` now snapshots the offset immediately after the **last
fully written JSONL line**. Plain rollout files use a fast final-byte check
and, only for incomplete tails, a bounded 1 MiB backward read; compressed
`.jsonl.zst` rollouts use the last newline in their already-bounded decoded
content. The recovered checkpoint still hashes and verifies the original
prefix. The callback refuses to publish a cursor when no complete record
exists within its bounded inspection, instead of inventing one.

The regression failed against the previous implementation with cursor
`124` instead of the last complete offset `80` on **both** regular and
Zstandard rollouts. After the patch it passes: the next completed user
record and a terminal `server_overloaded` event are observed at the first
monitor poll. Negative controls check empty/incomplete metadata, bounded
large partial tails, no replay of historical failures already preceding the
cursor, and preservation across an inherited same-root compact callback.

**Qualification:** focused regressions; shuffled race tests for CLI
runtime, runtime use case and session repository; full `make verify`
including cross-platform compilation, vet, installers, and repository-wide
shuffled race tests; an opt-in native Codex 0.162.1 callback smoke; and
the already-pinned **13-scenario Prodex 0.437.9 vs clean Godex
`ca4d4379bc9bcfed24edbd414199b4efde78b2f7`** WebSocket executable
differential all passed. The real-Codex smoke confirms the ordinary native
hook, **not** an actual concurrent mid-record native write.

The last-complete-line safeguard is bounded: a partially written line
longer than 1 MiB without any complete record in the inspected suffix
does not establish a cursor. Full native cross-binary recovery at arbitrary
record boundaries and every restart/compaction transition remain open.
An experimental HTTP SSE executable fixture was not admitted to the release
gate because the official reference did not finish within the fixture's
bounded observation window; no new HTTP executable-parity claim is made.
No release tag was created.

## Tagged 0.437.9 quota viewport and scroll pagination

The 0.437.9 quota renderer delegates page planning to
`prodex_quota_report_page_plan_v1` in compiled Mojo. Its policy clamps
`requested_start` to the **last profile index**, not to the last index of
a guessed number of visible profile sections. Godex's Bubble Tea quota TUI
previously used `len(reports) - visibleReportCount()`, so the user could
not select the last profile as the *start* of the viewport on a taller screen.

Commit `75b5213` introduces a small quota-local page planner matching
the tagged Mojo boundary cases: empty list and extreme scroll input,
hidden-above / hidden-below counts, reservation of one line for a
hidden-profile notice, insufficient height, and unlimited display height.
The interactive quota view now budgets **actual rendered report lines**
instead of treating each report as a fixed one-line profile section.
Scrolling through seven synthetic profiles at height 19 previously stopped
at offset 2; a failing-before regression now reaches the canonical
last-profile offset 6. The Bubble Tea event-loop ownership and
noninteractive table format are unchanged.

Commit `ba50758` additionally clamps the finished Bubble Tea view
to the terminal's available row count even when a tiny window cannot
accommodate the fixed pool summary. The quit/footer controls stay visible
when a row exists; zero-height output remains empty. Deterministic cases
cover heights 0/1/6/8/12/19, visible-notice capacity, a multiline
profile label, a blank list, and an extreme requested offset. These tests
qualify the **pagination decision and height safety**, not pixel- or
byte-identical TUI output across languages and terminal environments.

`make verify` passed after the two commits, including the full Go
shuffled race suite, vet, installation checks, and the six-target
Linux/macOS/Windows cross-build matrix. Using a separately built **clean**
Godex `ba50758d68009ccba94e8fa08c6822da5f6846a5` executable,
the SHA-256-pinned official Prodex 0.437.9 **13-scenario WebSocket**
differential still passed. The existing HTTP continuation loopback
also passed its bounded checks, including hard-affinity, quota,
cancellation and postcommit no-replay; its observed upstream attempt
counts still differ between products and are not represented as full
strict HTTP parity.

A full interactive quota terminal binary-to-binary differential, all
provider quota categories, upstream login, and the complete release
gate remain unqualified. These checks do not authorize a tag or release.

## Quota report comparator parity: capacity rank and ASCII-only text (follow-up)

Against **tagged Prodex 0.437.9** source commit
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`, the quota renderer's
`quota_status_rank` uses the admission and window-capacity candidate
decision, rather than the separately rendered status label. Before this
patch, Godex used the string `State == "ready"`; an OpenAI report with
no observed windows, a known 100%-used window, or explicit admission
denial could incorrectly outrank a genuinely healthy quota report.
The pinned comparator also assigns generic external-provider reports
an **unset reset sort key**; only typed provider cases such as Copilot
and Gemini use their own reset timestamps.

Commit `efb9bca` introduces the matching capacity-based OpenAI display
rank and ensures a generic external reset timestamp no longer reorders
reports. Provider-specific reset ordering remains intact. The fixture
`TestProdex04379QuotaSortUsesWindowCapacityInsteadOfStatusText`
asserts ready/unknown/exhausted/admission-denied/error ranks, while
`TestProdex04379QuotaSortOnlyUsesRelevantProviderResetEpoch` confirms
the difference between generic external reports and Copilot. Both tests
failed against the prior code, then passed after the fix.

The same tagged Mojo comparator's `quota_report_compare_text`
trims Unicode whitespace, then lowercases **ASCII A–Z bytes only**;
it does not apply general Unicode lowercasing. Godex's previous
`strings.ToLower` made non-ASCII labels like `É` and `é` appear
equivalent, sometimes changing which auth/profile sorted first.
Commit `0cc9216` preserves byte-wise non-ASCII identity while keeping
Unicode whitespace trimming and ASCII-insensitive comparison.
`TestProdex04379QuotaTextSortKeepsNonASCIIByteIdentity` first failed
against the previous implementation, then passed with the correction.

The tagged renderer treats `Result::Err` as a rank-2 provider failure.
A stale human-readable `State="error"` label cannot change the rank of
otherwise valid capacity evidence. Commit `60981a6` now uses the actual
provider error only, keeping stale status text out of the comparator.
`TestProdex04379QuotaSortIgnoresStaleErrorLabelWithValidEvidence` first
failed on the stale-error-label case; it also covers an exhausted weekly
window with a healthy five-hour window and a genuine fetch error.

**Verification:** failure-before regressions, all package quota tests,
and targeted shuffled race tests **PASS** for the three quota changes.
The full `make verify` gate **PASS**ed after all three fixes
on clean Godex commit `60981a6545b6183f277c2b76f092bb49ea258cef`.
The full gate includes format/size/TUI guards, six Linux/macOS/Windows
cross-builds, installer smoke checks, `go vet ./...`, shuffled
repository-wide race tests, and binary build.

These are **source-contract comparator regressions**, not an official
quota-command executable-to-executable differential. Additional
provider-specific display/routing decisions, all live quota sources,
the interactive terminal's exact presentation, and the entire
release gate remain unqualified. No tag or release was created.

## Native cursor rollout-path binding and cross-file replay rejection

A deterministic regression exposed an unsafe fallback in the 0.437.9
native-root recovery compatibility layer. The private SessionStart marker
previously captured the root UUID and a **logical byte offset**, but not
the path whose completed transcript supplied that offset. When the
session resolver selected a *different* rollout file that happened to
contain the **same UUID**, the valid offset could be applied to the
wrong history. A historical terminal capacity failure in the other
file was then incorrectly surfaced as a new `usage_limited` event.
The new regression **failed against the previous implementation** with
that false recovery.

Commit `feca731` binds the callback's cursor to its source rollout path
in a private, bounded `.rollout` sidecar. Identity and offset evidence
are published and read under the existing marker lock. The live monitor
only uses an offset when the resolver's absolute native rollout path
matches the captured path. Mismatches, symlinks, malformed or oversized
sidecar data, and missing path evidence fail closed to a fresh
current-tail snapshot **without replaying historical terminal errors**.
Repeated root/descendant notifications retain the original cursor;
explicit root changes clear both offset and path sidecars.

The regression now passes on both regular `.jsonl` and Zstandard
`.jsonl.zst` Codex rollouts: a same-UUID unrelated transcript does not
trigger recovery, while a newly appended terminal failure on the
originally captured path remains observable. The symlink/forgery and
root-switch negative controls passed. Full shuffled race tests for
CLI runtime, runtime use cases, and session repository passed, as did
the opt-in Codex 0.162.1 native SessionStart callback smoke and the
full `make verify` gate (six cross-builds, installer checks, vet,
and repository-wide shuffled race tests).

For clean Godex build
`feca731ed17d99929bba36c63a3f20e79c007ec9`, the
**official SHA-256-pinned Prodex 0.437.9** binary-to-binary WebSocket
differential still passed all **13 scenarios** (41 synthetic upstream
attempts). This verifies no regression on that existing WebSocket
surface; the new path-identity rule is a **Godex safety hardening**
validated with deterministic session fixtures, **not** an official
Prodex native-session binary differential. Full cross-process monitor
restart, every same-UUID compaction/migration variant, all live
provider states and the complete release gate remain unqualified.

## Pinned 0.437.9 release-oracle follow-up and scope

On clean Godex `8edb4431a8e96909665d4a43314cf3e0063f2fff`,
the release-only source oracle remained the exact Prodex tag
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`, and the official
Linux x86_64 executable reported `prodex 0.437.9` with SHA-256
`58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`.
The Godex binary's Go VCS metadata identified that clean commit.

The official **13-scenario WebSocket** differential passed on both
executables, including bounded same-owner missing-chain retry, hard
previous-response/turn-state ownership, and ownership after restart.
The strict **profile lifecycle oracle passed all 31 steps**, and the
**CLI invalid-argument/acceptance oracle passed** against the same pinned
binary. Each of the **18 DeepSeek provider differential scenarios**
then passed **individually**, with separate clean synthetic homes and
loopback-only upstreams: successful routing, tool calls, user identity,
SSE streaming/rate limits, retries, authorization and provider failures,
credential rotation, cancel/restart, and recovery after 429/503.
No real provider credentials or model inference were used.

**Unqualified gate:** `scripts/verify-release-parity.sh` reached successful
profile and CLI oracles but exceeded this test runner's 110-second
observation limit during its **combined** `--scenario all` DeepSeek stage.
The 18 separately passing cases **do not replace** the combined-mode
assertions or establish a completed release gate. This was a timeout,
not an observed parity comparison failure; its combined outcome remains
**unknown**. No release tag or package is authorized.

The same turn also added a **Godex safety safeguard** beyond the
tagged native cursor's minimal sidecar: matching a captured rollout
path before applying an offset. It prevents historical capacity
events in unrelated same-UUID regular/Zstandard files from being
misclassified as new terminal failures. It passed failing-before
regressions, malformed/symlink sidecar negative controls, true-path
recovery, native Codex callback smoke, and shuffled Go race tests.
This hardening **is not** proof of binary-to-binary parity for all
native recovery, compaction, and filesystem permutations.

## Combined DeepSeek provider oracle: all 18 scenarios in one run

The previously time-limited `--scenario all` stage has now been
requalified **as one complete executable comparison**, rather than as
18 independent calls. On clean Godex commit
`ac43de7f73cc949054234e8e4e0f24ef95745e03`, the fixture compared
the exact official Linux Prodex 0.437.9 binary (SHA-256
`58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`)
and tagged source
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`.

The **combined-mode result is PASS**: 18 included scenarios, zero
comparison differences for every scenario and at the top level, and
the status/header/event/durable-state/retry/cancellation/stdout mutation
negative controls all PASS. Scenarios exercised normal DeepSeek requests,
function calls, user identity, SSE streams and rate-limit headers,
provider authentication/error variants (401/403/429/500/503), key
rotation and persisted restart, cancellation, and recovery after
429/503. Synthetic upstream credentials and loopback servers were used
throughout, with no real provider inference.

This result **supersedes** the earlier checkpoint's statement that the
combined mode was unknown due to the short runner observation limit.
It does not imply that the whole `scripts/verify-release-parity.sh`
script has passed: the profile, CLI, provider and WebSocket release
oracles must also complete together under the same exact clean commit.
No tag or release has been created on the strength of this subgate.

## Full release-only oracle PASS with strict native-context provenance

The **complete** `scripts/verify-release-parity.sh` gate ran to
completion with exit status **0** on clean Godex commit
`49cc7fa02023c2bb26a3e769d485a18580b075da`. All of its included
oracles used the exact source tag
`0e321c92f4e580aa0b090085d5af13a8b856c3f1` and published
Prodex 0.437.9 Linux x86_64 executable with SHA-256
`58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`.
Godex was built from the same **clean commit** identified by embedded
Go VCS revision metadata with `vcs.modified=false`; source and binary
identity guards remain fail-closed.

The five release stages completed **together in their original order**:

- Managed-profile lifecycle and restart: **31/31 steps PASS**
- CLI acceptance and invalid-argument differential: **8/8 PASS**
- Native Codex context forwarding: **3/3 PASS** with cached-maximum
  promotion negative control
- DeepSeek provider `--scenario all`: **18/18 PASS**, zero material
  comparison differences, and status/header/event/state/retry/
  cancellation/stdout mutation negative controls PASS
- Responses WebSocket: **13/13 PASS**, including locked-affinity and
  stale-continuation errors, with 41 synthetic upstream attempts
  recorded across both binaries (including reference background probes)

Commits `0d392a8` and `49cc7fa` strengthened the release gate by
requiring the independently pinned native-context oracle. That tool now
checks the Prodex source revision, the clean candidate worktree, and
the candidate binary's VCS revision and dirty flag before running any
child process. Dedicated negative tests reject stale/modified metadata,
dirty source, and incorrect source revision. The separately executed
strict context oracle also **PASS**ed 3/3 and rejected an intentionally
wrong 40-character Godex commit. Full `make verify` on `49cc7fa`
**PASS**ed, including six-target cross-builds, installer checks,
`go vet ./...`, and repository-wide shuffled race tests.

This evidence **supersedes the earlier checkpoint** where a short
110-second observation window interrupted the combined release stage.
It proves parity only for the finite synthetic scenarios in the
release gate. It does **not** certify full 1:1 native-session recovery,
app-server handoff, all real providers/quota phases, or
cross-platform native execution. No real provider credentials or
model requests were used, and no tag or release was created.
