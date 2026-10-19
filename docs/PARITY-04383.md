# Godex versus Prodex 0.438.3 — behavioral parity

**Target:** published Prodex release `0.438.3` only. Tagged source commit:
`42042d765100fc06895a27e0904ffb64842092e6`. Official Linux x86_64
binary `prodex-x86_64-unknown-linux-gnu` published SHA-256:
`5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923`.
Published `release-manifest.json` SHA-256:
`300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da`.
The tags, GitHub release manifest and checksum were checked independently;
the verified pinned executable reports `prodex 0.438.3`.

The previous source-pinned `0.438.2` finite gate is documented separately
in `PARITY-04382.md`. Its release result must not be counted as passing
0.438.3 without executing the new binary/tag against a clean Godex commit.

## Delta from 0.438.2

The official `docs/release-notes/0.438.3.md` describes changes to:

- **WebSocket precommit transport recovery:** handshake, send, read and close
  errors are typed attempt outcomes. The outer affinity-aware routing owner
  alone authorizes safe cross-profile recovery; lower transports cannot
  bypass continuity or close the client prematurely.
- **Turn-state-only full-history recovery:** with no `previous_response_id`, a
  safe complete transcript can replay on an eligible Ready profile after an
  uncommitted transport failure. The failed turn binding is removed from
  headers, request metadata and persisted affinity before recovery; delta-only
  input and strict compact-followup are not eligible.
- **Quiet-stream liveness and deadlines:** bounded Ping control frames may
  keep the client alive during upstream metadata holds without committing
  output, extending the logical request deadline, resetting upstream
  progress or changing quota. Poll readiness is nonblocking on Unix and
  Windows, preserving partial TLS/WebSocket frames and socket receive state.
- **Readiness before idle decisions:** buffered `response.completed` and
  `response.failed` frames win over a stale idle-clock calculation. OS
  readiness checks themselves do not renew progress.
- **Credential keyrings and internal Mojo/domain policies:** maintenance of
  native OS stores and new compiled policy projections. Changes are scoped
  to observable behavior; raw Rust/Mojo refactors do not require Go rewrites
  when the existing client contract is already equivalent.

## First source-derived fix — transport-only full-history turn-state rotation

`internal/usecase/routing/websocket_transport_fullhistory_04383.go` implements
only the bounded 0.438.3 recovery decision for a WebSocket response failure
that is a typed **precommit transport** outcome (not a quota or application
failure). It requires a reconstructable full-history request, a turn-state
binding **verified** to belong to the failed profile and another independently
Ready same-provider profile. Only then does it release the failed turn
binding, remove the stale `x-codex-turn-state` from both HTTP headers and
nested request metadata, preserve the request's original full input and
deadline, and replay on the selected other owner.

`TestProdex04383TransportBoundTurnStateFullHistoryRotatesToReadyProfile`
reproduced the previous Godex behavior: a full-history turn remained on its
failed transport owner and returned HTTP 502 even though another synthetic
profile was Ready. It now passes with a single safely moved full-history
turn and exactly one new inference on the alternate Ready profile.
`TestProdex04383TransportTurnStateDeltaOnlyRemainsBound` and
`TestProdex04383TransportFullHistoryRejectsUnsafeRotationVariants` protect
previous-response references, delta-only tool output, realtime duplex,
strict compact-followup, committed output, quota-specific recovery, exhausted
retry budget, missing affinity ownership, negative cached quota evidence and
absent alternative profiles. These checks passed repeated Go `-race` runs
with synthetic identities and no real credentials or model requests.

**Scope:** this first stage is tagged-source/local Router parity only, not
an independent two-executable 0.438.3 network recovery certificate. Further
work must verify the release's typed connection/send/read/close failures and
full-history replay using the official binary; the existing 0.438.2 gate is
currently historical until re-pinned and re-executed. Overall production 1:1
remains **PARTIAL**, including native Windows/macOS and live OAuth behavior.

## Typed upstream WebSocket precommit transport failures

Prodex 0.438.3 returns typed **attempt outcomes** when precommit connection,
send, read or upstream close fails. Previously Godex's `ExecuteWebSocketMessage`
returned raw Go errors for fresh handshake failures and for read/close errors
on hard turn-state, previous-response or compact/session-bound requests.
Those errors escaped the typed precommit classifier, potentially closing the
local client before its affinity-aware recovery owner considered whether
safe replay was permitted. A source-derived transport regression failed on
all three hard-affinity read/close cases and on an unopened upstream socket.

The OpenAI WebSocket adapter now returns `websocketTransportFailure` with
`PrecommitFailure.Transport=true`, `FirstEventCommitted=false`, HTTP 502 and
an empty client-visible precommit body for all non-canceled connection,
send and read errors. A canceled context still preserves the original
`ctx.Err()`; the realtime duplex branch still uses its distinct committed
lifecycle. The transport **never** grants permission to rotate credentials,
replay a previous-response dependency or bypass quota. Routing owns those
decisions. `TestProdex04383BoundWebSocketUpstreamReadFailureIsTypedAttempt`,
`TestProdex04383WebSocketFailedHandshakeReturnsTypedPrecommitAttempt` and
`TestProdex04383PartialPrecommitWebSocketFrameIsTypedNotCommitted` prove the
bounded behavior against actual local TCP/WebSocket connections, including
a truncated payload header followed by EOF. The latter's partial bytes must
never be promoted to committed output. All focused tests and race detection
PASS, as do the existing noncommitted/committed owner-retry regression suites.

These tests establish **source-derived local adapter parity**, not a full
published Prodex 0.438.3 two-executable transport recovery oracle. The
subsequent release gate must pin and exercise the actual new binary to
qualify cross-product behavior; release 0.438.2 differential logs cannot
be substituted.


## Active 0.438.3 executable release gate provenance

The active `scripts/verify-release-parity.sh` and all seven Prodex official
executable parity oracle entry points have been re-pinned from `0.438.2` to
`0.438.3`, using the tagged source commit, published SHA-256 binary, version
string and release manifest recorded at the beginning of this document.
The historical `PARITY-04382.md` remains unchanged. The active preflight
adds the 0.438.3 fail-before transport full-history and typed precommit
outcome regression suite under Go's race detector. Every old synthetic
profile, CLI, context, DeepSeek, Gemini and WebSocket replay/affinity fixture
remains mandatory. Changing the pins alone does **not** establish release
parity: all executable phases must pass against **one clean Godex candidate**
using the actual published 0.438.3 binary.

## Native macOS/Windows/Linux oracle manifest pins

The independent `--websocket-*` continuation oracle validates a **second**
platform-specific release asset table, rather than relying solely on the
standard Linux release-gate pin. The first exact 0.438.3 WebSocket invocation
failed at its **manifest digest preflight**, before any upstream model case,
because this table still pinned 0.438.2. This was a provenance mismatch in
the test harness, not a demonstrated Godex transport failure.

The native oracle platform table in
`tools/continuationparity/native_oracle_04382.go` now pins the six official
0.438.3 executables published in the verified SHA256SUMS, plus the new
manifest digest. `scripts/verify-native-parity.ps1` and the standalone
`tools/workspace-oracle.py` use the same independently published pins, exact
tag commit and version. The existing negative controls, source-integrity
checks and six-mode native executable matrix are retained; historic names
with the `_04382` suffix are module/test identifiers, not oracle versions.
The native macOS/Windows modes must still be **run** on those hosts before
claiming parity there; retargeting a checksum is not a native test pass.


## Quiet Responses WebSocket client heartbeat — bounded 0.438.3 liveness

The first pinned **0.438.3** WebSocket-core run encountered a new official
control frame, not a model payload: Prodex sent an **empty opcode-9 Ping**
while a committed upstream Responses stream sat idle. The inherited 0.438.2
oracle erroneously demanded that the very next client frame be the terminal
close control. This is expected from the new compiled Mojo
`websocket_response_tracking::response_wait_plan`: local client Ping
heartbeats are scheduled every 1,000ms independently of upstream progress.
They do **not** commit model output, reset the upstream idle clock, extend
precommit deadline, mutate quota or permit any replay.

The Godex HTTP Responses delivery now emits the same **transport-only**
empty Pings during an active Responses request, both while the Router is
waiting for upstream admission/precommit output and while a genuine committed
stream is waiting for more frames. Heartbeats use the existing frame-writer
mutex, a bounded socket write deadline and a scoped goroutine that is stopped
before response teardown. Realtime duplex is deliberately excluded because
it owns a different committed-session transport contract. Pong replies remain
standard masked client controls and are absorbed rather than becoming new
model requests.

`TestProdex04383QuietResponsesTransportPingsBeforeCommitWithoutReplay`
drives the actual locally upgraded HTTP Responses WebSocket and an
intentionally quiet synthetic gateway: after ~1s the client receives an
empty Ping but no application data or second upstream attempt, replies with
Pong, then receives the exact terminal result and successfully starts another
turn on the same socket. The pinned executable fixtures now accept ONLY
validated empty Ping controls, reply with masked Pong and still enforce
**a real 2-second committed idle close, a real 30-second absolute precommit
504, unchanged upstream request count and exact output/owner identity**.
Mandatory negative sentinels reject zero/excess client heartbeats, unexpected
application frames, early/late close, postcommit replay, owner rotation and
hidden precommit output leaks. A heartbeat may not convert a terminal idle
failure into indefinite progress. These changes should be requalified on
one clean Godex commit against the official Prodex 0.438.3 executable before
claiming the new liveness differential VERIFIED.


## 0.438.3 readiness check before upstream idle expiry — bounded local fix

The tagged 0.438.3 `websocket/response_tracking/readiness.rs` policy checks
for ready/queued upstream bytes before treating the upstream idle clock as a
terminal timeout. A Godex read watchdog previously closed a TCP/TLS
connection from its timer even when no application read was pending and the
OS already had a complete `response.completed` or `response.failed` frame
queued in the receive buffer. A slow synchronous state-persistence interval
could therefore hide a real terminal response behind a synthetic timeout.

`TestProdex04383QueuedTerminalBeatsExpiredCommittedIdle` reproduces that
case with a real loopback TCP socket, two exact terminal frame variants,
and a deliberately delayed next application read. The bounded Go fix
indicates a deferred expiry when no read is active and the transport
supports `SetReadDeadline`. On the next read it performs **one short,
non-blocking-style queued-data probe** before classifying the original idle
boundary. Already buffered bytes are preserved and start a new progress
interval; a socket with no queued bytes closes and reports the configured
committed idle timeout. When a read is already blocked at expiry, the old
prompt close behavior is unchanged. Non-deadline-capable readers stay on the
legacy close path. The probe never extends a logical request's original
precommit deadline, does not create replay rights, and cannot mutate quota.

`TestProdex04383GenuinelySilentCommittedStreamStillTimesOut`,
`TestProdex04383ExpiredQuietSocketWithoutActiveReaderStillTimesOut`, and
`TestProdex04383ExpiredQuietSocketBeforeCommitIsNotCommittedIdle` are negative
controls for silence, bounded delayed reads, and distinct precommit error
ownership. All local cases passed repeated Go `-race` runs, including true
TCP queued-data delivery; they do not certify TLS partial-record buffering,
native macOS/Windows poll behavior, or arbitrary upstream handshake failures.
A native official-binary differential for this exact readiness race remains
**PARTIAL** until the same queued-terminal timing can be controlled in both
published executables.


## Partial upstream WebSocket bytes do not count as frame progress

The tagged Prodex 0.438.3
`runtime_proxy/websocket/response_tracking/frame.rs` liveness loop advances
`last_upstream_frame` only when Tungstenite returns a **complete** decoded
WebSocket message. A partial header, incomplete TLS/WebSocket payload or
unfinished fragmented text message does **not** produce a new upstream event
and cannot postpone the precommit transport progress watchdog. Godex's
`websocketReadWatchdog.Read` previously refreshed the progress timer on
**every positive socket byte count**, even while
`readWebSocketEvent` was still assembling an unfinished frame. This was a
real bounded discrepancy: a slow peer could drip invalid partial bytes to
renew the upstream idle budget without ever delivering a model response.

`TestProdex04383IncompletePrecommitPayloadCannotRenewProgressDeadline`
reproduced the bug **before the patch**: an incomplete text payload renewed
the configured timer by more than 60ms without a complete message. The
scoped fix leaves byte-oriented progress tracking intact for genuinely
**committed** output streams, but lets the precommit event parser update
progress **only after a complete, validated message** (including control
Ping/Pong messages). `TestProdex04383FragmentedMessageAdvancesProgressOnlyAfterFinalContinuation`
asserts a nonfinal text fragment does not renew the clock and the final
continuation does, while preserving the complete terminal event's bytes and
identity. `TestProdex04383CompletedPrecommitFrameRenewsProgressDeadline`
protects legitimate complete-frame liveness, and
`TestProdex04383PartialPrecommitFrameStillTimesOut` protects a truncated
message's bounded failure. Existing physical queued-terminal readiness and
silent-stream negative controls still PASS with repeated `-race` runs.

This is **source-aligned local WebSocket frame progress proof**, not a claim
that native Windows, TLS partial records, or every production network fault
has been qualified in an independent two-executable release fixture. It
preserves the original request-wide precommit deadline, hard-affinity owner,
quota classification and no-replay rules. The release's full executable
oracles must be rerun on one clean Godex revision after this change.


## Committed-stream partial bytes cannot extend upstream idle

The 0.438.3 complete-upstream-message liveness contract applies **after
commit** as well as before it. A follow-on fail-before regression showed
that Godex's committed `websocketResponseBody` could receive only the header
and a few bytes of an unfinished text frame, yet its socket-level watchdog
would reset the configured idle timer on each positive byte read. Tagged
Prodex's `response_tracking/frame.rs` instead marks progress after Tungstenite
returns a **complete** assembled WebSocket message, including the final
continuation of fragmented data or a complete control frame. Trickle partial
payloads cannot postpone upstream expiry indefinitely.

`TestProdex04383IncompleteCommittedPayloadCannotRenewProgressDeadline`
reproduced a spurious ~60ms extension on a truncated committed frame
**before** the fix. Godex's socket watchdog now delegates all upstream
progress accounting to the owning WebSocket parsers: precommit
`readWebSocketEvent` and committed `websocketResponseBody.finishFrame` and
`handleControlFrame`. A complete valid model message, binary frame or
Ping/Pong control advances the timer exactly once, while incomplete headers,
partial TCP/TLS payloads and unfinished fragments do **not**. The previously
implemented queued-terminal readiness probe retains its original bounded
behavior, and a completed control frame cannot create model output or retry
rights. `TestProdex04383CompleteCommittedControlFrameCountsAsProgress`
protects the positive Ping-control case. All focused precommit/committed
frame and queued-terminal regressions pass repeated Go race tests, including
the configured committed-idle timeout negative control. Actual TLS frame
buffering and arbitrary network/platform poll failures still require
independent full two-executable coverage; no claim of production-wide parity
is made from these local checks alone.


## Heartbeat/model-output writer contention — bounded shutdown guarantee

The 0.438.3 response-transport liveness Ping is **advisory** control output.
It must never delay or interleave a model data/terminal frame that already
owns the local client `websocketFrameWriter` mutex. A deterministic
fail-before test found an edge case in Godex's newly introduced heartbeat:
`sendHeartbeat04383` waited **unbounded** for the writer mutex. During a
client stalled on a visible model frame, the heartbeat worker could become
stuck acquiring the same lock; its `stopHeartbeat()` then waited for the
worker, blocking Responses teardown and weighted profile permit release.

`TestProdex04383HeartbeatSkipsContendedModelWriter` acquires the data writer
mutex and verifies an advisory Ping returns immediately rather than waiting
for active model output. `TestProdex04383HeartbeatShutdownDoesNotWaitForBlockedClientWriter`
lets a heartbeat tick occur while the model writer holds that mutex, then
asserts that shutdown completes even before the model frame is drained.
Both tests failed before the fix and now pass repeatedly with Go race
instrumentation. The scoped fix uses Go's nonblocking `TryLock` for the
advisory control: when actual data output owns the writer, the Ping tick is
**skipped**, not buffered behind model output. An uncontended tick still
sends an empty opcode-9 Ping with the original bounded write deadline and
mutex protection, and the previous public Ping/Pong and exact two-binary
wire, owner, 30-second deadline, and no-replay tests remain mandatory.

This is a bounded local contention/shutdown regression, **not** proof of all
Prodex-native client backpressure permutations. It does not reset upstream
progress, change quota/account selection, or grant a retry after commitment.

## Complete pinned finite release gate — heartbeat writer contention checkpoint

Every original finite release phase exited **PASS on the identical clean
Godex candidate `9ee52e0ec7960444f3dbd646e771d847a7814104`**, checked
against official Prodex 0.438.3 tagged source
`42042d765100fc06895a27e0904ffb64842092e6`, published Linux x86_64
binary SHA-256
`5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923`
and the published release manifest SHA-256
`300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da`.
The source was clean for every binary-pinned phase. Synthetic credentials
and independent HOME directories were used throughout; no user credentials
or external model accounts were read.

| Phase | Result on exact `9ee52e0e` |
| --- | --- |
| `--phase=preflight-tests` | PASS, including added advisory heartbeat writer-contention and shutdown race regressions, official Codex 0.163.0 native app-server handshake/compaction controls when its SHA-pinned binary was available, all existing Go/provider regression gates and negative sentinels |
| `--phase=preflight-oracles-pty` | PASS, official PTY/footer, CLI (14 probes), context and newest-first workspace trust differential with negative controls |
| `--phase=preflight-oracles-profiles` | PASS, 34 lifecycle stages including auth/quota, real Linux FD pressure, admission/queue wake, and cross-binary state ownership |
| `--phase=preflight-oracles-history` | PASS, official previous-response/continuation replay and durable affinity fixture |
| `--phase=deepseek` | PASS **30/30**, `comparison_differences: []`, mutation controls PASS |
| `--phase=gemini` | PASS **13/13**, `comparison_differences: []`, mutation controls PASS |
| `--phase=websocket-core` | PASS, 56 historic model upstream attempts plus independently verified idle capacity, committed-idle/8KiB lookahead close, Pong handling, realtime duplex lifecycle and no-replay controls on **both official executables** |
| `--phase=websocket-deadline` | PASS, Prodex and Godex each report public `504 request_timeout` at the original **30-second** precommit deadline, then complete a fresh request on the same local socket without a replay |
| `make verify` | PASS on production source: format, file-size guard, vet, shuffled full Go race suite, production Go build |
| `scripts/check-cross-build.sh` | PASS, Linux amd64/arm64, Darwin amd64/arm64, Windows amd64/arm64 and targeted Darwin/Windows test compilation |

**Qualification boundary:** all listed finite source/synthetic two-executable
contracts are VERIFIED, including real local client Ping/Pong and complete
upstream model-frame progress. The new writer-lock shutdown control itself
is a Go local race fixture; do not count it as a fully independent Prodex
native backpressure differential. The earlier exploratory full-history
transport recovery fixture that returned `service_unavailable` from Prodex
and a TCP timeout from an older Godex candidate was not source-qualified:
its seeded quota/owner state and readiness were not proved equivalent, and
it is **not** a passing 0.438.3 replay oracle. That gap, native macOS/Windows
runtime permutations, live OAuth/real quota reset, and unobserved provider
faults remain PARTIAL/OPEN. The complete synthetic release gate is not a
production-wide FULL 1:1 parity certification.

## Full-history transport replay preserves opaque numeric input

A subsequent 0.438.3 source-aligned Router audit found that removing a failed
`x-codex-turn-state` from a reconstructable full-history WebSocket request
re-decoded the **entire** request through Go's `map[string]any`. This silently
rounded the user's large JSON integers before dispatching the replay to the
Ready profile: `9007199254740993` became `9007199254740992`, and
`18446744073709551615` became `18446744073709552000`. Unlike a permitted
turn-binding removal, changing opaque input/tool arguments violates the
full-history replay contract.

`TestProdex04383TurnStateRecoveryPreservesOpaqueNumericHistory` and the
extended end-to-end Router fixture
`TestProdex04383TransportBoundTurnStateFullHistoryRotatesToReadyProfile`
both failed before the fix. The scoped change now decodes only the outer
object and `client_metadata` into `json.RawMessage`, removes the exact stale
turn-state keys, and leaves all other JSON subtrees/numbers opaque. HTTP
headers are cloned; the original request is not mutated. Affinity ownership,
retry eligibility, deadlines, and strict no-postcommit-replay gates remain
unchanged.

This is a local precision/identity regression, **not** an independent
published-Prodex executable differential for large numeric payloads. The
previous pinned finite-gate result belongs to `9ee52e0e`; it must be rerun
on the new clean commit before being attributed to this change.

## Full pinned finite release gate after opaque-history precision repair

The **complete 0.438.3 finite source/synthetic gate** was independently
rerun on one clean Godex candidate, commit
`b6e91489d1881fc02aac78c2e6100a08efefb59c`, after merging the numeric
turn-state replay repair. Every individual release script phase exited
**PASS** against the SHA-256-pinned official 0.438.3 binary, manifest and
tagged source (pins in this document's opening). No live OAuth, user home,
paid model endpoint or real credential was used.

| Release-script phase | Result on exact `b6e91489` |
| --- | --- |
| `--phase=preflight-tests` | PASS, including deterministic and race regressions |
| `--phase=preflight-oracles-pty` | PASS, PTY/CLI/context/workspace controls |
| `--phase=preflight-oracles-profiles` | PASS, lifecycle/quota/admission/FD controls |
| `--phase=preflight-oracles-history` | PASS, native previous-response replay |
| `--phase=deepseek` | PASS, pinned provider response oracle and mutation controls |
| `--phase=gemini` | PASS, pinned provider response oracle and mutation controls |
| `--phase=websocket-core` | PASS, 56 model-attempt baseline plus capacity, idle-close, 8KiB lookahead, realtime and race controls |
| `--phase=websocket-deadline` | PASS, both executables returned 504 at the original 30-second precommit deadline and accepted a followup |

Separately, `make verify`, `go test ./... -count=1 -timeout=90s`,
`go test -race ./internal/usecase/routing -count=1 -timeout=105s` and
`./scripts/check-cross-build.sh` all exited PASS on this same candidate.
This qualifies the full pinned **finite** gate after the production fix; it
is not a claim that all native runtime faults, Linux keyring interoperability,
Windows/macOS live execution, OAuth, provider outages, or arbitrary
full-history transport failure shapes have achieved production-wide 1:1
parity. A cross-binary large-numeric/full-history transport replay fixture
remains unproven independently of the local causal regressions above.

## Transport replay removes noncanonical HTTP turn-state header aliases

The tagged 0.438.3 `runtime_request_without_turn_state` removes **every**
ASCII-case-insensitive `x-codex-turn-state` header, rather than relying on a
canonical map-key lookup. A Go-caller-built request could contain a canonical,
lowercase, uppercase and mixed-case copy at once. The old `Header.Del` removed
only the canonical key, leaving a failed owner's turn token on the replay.
`TestProdex04383TransportReplayRemovesEveryTurnStateHeaderCase` failed on
that exact leak before the fix and passes with all variants removed, while
preserving unrelated headers and the original request. This is a local
source-aligned regression and does not qualify the unresolved independent
full-history transport failure fixture.

## Case-insensitive header replay checkpoint — clean commit `7710f076`

The header-alias replay fix was pushed to `main` at
`7710f076774890d9782b0eb75349fb5b12a30bd9`, independently of CI status.
The original per-domain reproduction failed before the patch and passed
`go test -race ./internal/usecase/routing -run '^TestProdex04383' -count=1`
afterward. The clean candidate then passed `make verify`, including the full
shuffled Go race suite, vet, installer smoke checks, cross-build and targeted
cross-platform test compilation. The SHA/source/manifest-pinned official
Prodex 0.438.3 release gate was rerun on this **exact commit** for these
phases, all PASS:

- `--phase=preflight-tests` — all source/local negative controls
- `--phase=websocket-core` — 56-attempt transport oracle, committed/8KiB
  idle-close, capacity/reuse, native realtime and no-postcommit-replay fixtures
- `--phase=websocket-deadline` — matching 30-second original precommit 504
  and a successful followup turn on the same client socket

Other release phases were previously verified on `b6e91489`, not re-attributed
to this new commit. The independent *transport full-history* replay proof is
still **OPEN**: an exploratory two-profile, precommit-upstream-close run on
Prodex 0.438.3 reported one failed owner and `503 service_unavailable`
because the synthetic selection had no authenticated Ready alternate
(`selection_plan: reports=0 ready=0`). Manipulating the active profile instead
changed the initial selection owner, invalidating the causal comparison. This
is insufficient evidence of a Godex/Prodex behavior mismatch or successful
cross-profile recovery. The experimental fixture remains off `main` and out
of the release gate until Ready-profile evidence and exact owner provenance
are independently established on both executables. Native Windows/macOS,
live OAuth, keyring and unbounded production fault coverage remain partial.

## Terminal precommit transport failure must reach the client

The independent recovery investigation found why the earlier `gateway`
experiment had no alternate: that command intentionally fixes the runtime
pool to its active profile in both products. A managed `run` launch, with
two isolated synthetic profiles, authenticated loopback quota receipts and
an explicit main-owned turn, instead performs the expected two-attempt
full-history recovery on both pinned executables. This also preserves large
JSON integers, tool arguments, and the exact visible completion frames.

Its delta-only negative control exposed a real discrepancy on clean baseline
`46d8b0fe`: published Prodex returns a terminal `503 service_unavailable`,
while Godex forwards an empty typed transport attempt and leaves the client
waiting. Both the bound-owner and exhausted-fresh finalization paths now
replace a terminal precommit WebSocket transport attempt with the same public
503 envelope and retry-budget message. Routing remains the sole retry owner;
the transport still returns typed empty attempts for safe recovery decisions.
No alternate inference or affinity release is added for delta-only input.

`TestProdex04383DeltaOnlyTransportFailureReturnsTerminal503WithoutReplay`
and `TestProdex04383FreshExhaustedWebSocketTransportFailureReturnsTerminal503`
failed before this repair with HTTP 502 and an empty body. Focused regression
and race checks pass after it. Committed output, realtime duplex, ordinary
HTTP and application-error responses retain their existing finalization.
The investigation's new executable fixture is still being qualified,
including durable session-state and unsafe-replay controls; this checkpoint
does not close the full transport-recovery gap or certify FULL parity.

## Transport recovery retains an accompanying session binding

The now-authenticated two-profile executable experiment also compared durable
ownership after successful full-history recovery. Prodex's tagged
`runtime_lineage.release_plan` releases a matching turn-state binding but
deliberately retains session ownership whenever a previous-response or turn
identifier is present. When the alternate returns a successful result,
the conflicting session claims persist as the hard-owner conflict sentinel.
The new response belongs to the alternate, the original unrelated response
still belongs to main, and the failed turn binding is absent.

Godex instead released both the failed turn and its accompanying session,
silently assigning that session to the alternate. The transport-only recovery
now releases only its exact failed turn. No persistence format, gateway,
shared helper or quota-recovery policy changes. The regression
`TestProdex04383TransportTurnRecoveryRetainsAccompanyingSessionOwner` failed
before the patch in both memory and durable-store variants, then passed
with the preserved conflict, removed failed turn, unchanged original response
and exactly one alternate inference. This is a fail-closed ownership change;
the complete independent fixture remains under qualification, especially its
strict compact-followup setup.

## Persist response identity first observed after WebSocket commitment

The stronger executable oracle now reads both durable ownership snapshots.
It exposed another existing gap: a recovered stream can emit a visible delta
before revealing its response ID in `response.completed`. Prodex persists
that terminal ID's alternate owner; Godex previously forwarded it without
recording the new binding, losing continuation ownership after restart.

`TestProdex04383LateWebSocketResponseOwnerPersistsBeforeTerminalDelivery`
failed before the patch for ordinary, fragmented, and 128 KiB terminal
envelopes: the client received `resp-late`, but the durable owner map was
empty. The existing gateway parser now exposes complete late metadata through
a borrowed model event. HTTP delivery invokes routing's existing observation
workflow before forwarding the final payload bytes. This reuses the parser's
already bounded buffer, adds no goroutine or dependency, preserves exact frame
bytes, and does not write affinity state for every unchanged token delta.
Truncated messages produce no observation. A persistence error stops the
committed response without replay. Focused integration and race checks pass;
full release qualification must use the resulting clean commit before it is
attributed to this repair.

## Independent authenticated transport oracle

`tools/continuationparity --websocket-transport` now exercises a genuine
multi-profile managed launch rather than the fixed active-profile gateway.
Both synthetic profiles receive authenticated, healthy loopback quota
responses before the failed main-owned turn is sent. The official runtime
log must independently report the alternate's healthy `probe_cache` source
and the exact main-owned transport replay decision. A hidden upstream
`response.created` followed by TCP EOF deterministically fails before commit.

The full-history case requires exactly two authenticated handshakes and
inferences, main then second; exact client delta/completion frames; removal
of stale turn-state headers and nested metadata; and preservation of tool
arguments plus integers `9007199254740993` and `18446744073709551615`.
Both durable stores must contain the recovered response on second, the
unrelated original response on main, no failed turn, and the accompanying
session conflict sentinel. Delta-only and previous-response controls require
exact 503 and 400 terminal frames respectively, with one main inference and
no alternate dispatch. Mutation controls reject each material deviation.

The new finite slice is available as
`./scripts/verify-release-parity.sh --phase=websocket-transport` and is also
mandatory in the default all gate and native PowerShell matrix. Qualification
of this committed harness follows on a clean candidate; native macOS/Windows
execution and a proven strict compact-followup fixture remain OPEN. The
three production repairs were independently checked on clean `abc86d03`
with the exploratory six-case executable harness and `make verify`.
The complete original finite release gate separately passed on `f41611c9`;
that result is not attributed to the later late-ID repair.

## Full-history replay lifecycle controls

The authenticated `--websocket-transport` slice now also checks the original
30-second request deadline, explicit client cancellation after upstream
dispatch, and upstream EOF after visible output. All three use the same
reconstructable history and an independently authenticated Ready alternate,
so replay would otherwise be eligible. They require exactly one main-owned
inference, intact request context, and no alternate dispatch. The postcommit
case requires exact created/delta frames followed by TCP EOF, matching this
specific official transport-close fault; an idle-timeout close frame is a
different contract. Deadline mutation tests reject early/renewed budgets or
changed public errors. Cancellation requires observed upstream teardown.

These controls passed the exploratory pinned two-executable run on clean
`abc86d03`, together with the matching durable ownership snapshots. The
expanded committed phase must be requalified before being counted as VERIFIED.
Strict compact-followup setup remains OPEN: the earlier compact experiment
had an ordinary turn owner, which suppressed the strict compact-followup
selection slot and allowed normal full-history recovery in official Prodex.
That experiment is preserved outside the repository and is excluded from
the passing oracle. It is not evidence of unsafe strict-compact replay.

## Native realtime fixture quota schema repair

Actual macOS CI on `abc86d03` failed inside the official Prodex realtime
sidecar before Godex ran that case. The same log showed persisted usage
snapshots rejected with `runtime_proxy_recovery_fallback`; source audit found
the fixture's third profile omitted required `five_hour_reset_at`.
`TestProdex04383RealtimeFixtureKeepsEveryRequiredQuotaField` reproduced the
missing field before the repair. Realtime preparation now clones an existing
complete synthetic snapshot before marking both alternates weekly-exhausted.
The focused schema regression passes. This repairs invalid oracle state;
it does not establish the cause or resolution of the separate immediate
native realtime connection reset. Native qualification remains OPEN pending
actual execution of the repaired fixture. No Godex production behavior changes.

## Complete Linux gate and applied quota-probe prerequisite

The complete `./scripts/verify-release-parity.sh` gate passed on clean
`f293407a098e5253ac3fd32452090aed4043c201`, including all six authenticated
transport recovery and lifecycle scenarios. The pinned source, executable and
manifest checks passed on that exact candidate. This qualifies that finite
Linux slice; strict compact-followup and full feature parity remain OPEN.

Native run `39270223855` passed the expanded transport phase on macOS but
failed on Windows because the official log did not establish the required
authenticated Ready selection. The fixture counted incoming usage probes
before Prodex had applied their asynchronous results. It now waits within its
existing five-second preparation budget for both `profile_probe_refresh_ok`
events before starting the fault. The fail-before regression rejects pending,
partially applied and failed probes even with both authenticated receipts.
The exact recovery selection, wire and durable-owner assertions remain in force.
The repaired prerequisite still requires clean-candidate executable qualification.

That same native run also failed macOS realtime session setup in official
Prodex and one Godex committed-idle close/Ping observation. These remain
unresolved native evidence; neither is certified by the passing Linux gate.

The applied-probe repair passed
`./scripts/verify-release-parity.sh --phase=websocket-transport` on clean
`714e82f1e53a7f619c9d57437e6e118c4ab30bd5`. All six scenarios matched both
executables, including durable ownership and the original deadline. That result
does not establish a repaired native matrix.

## Quota full-history recovery keeps accompanying session ownership

An authenticated two-profile executable experiment on clean `714e82f1`
replaced the transport close with a precommit `429 insufficient_quota` event.
Both products attempted main then second and preserved exact request/output
content, but Godex silently persisted the session on second. Official Prodex
retained the accompanying session claim, yielding the same hard-owner conflict
as transport recovery. The unrelated response remained on main in both stores.

`TestProdex04383QuotaTurnRecoveryRetainsAccompanyingSessionOwner` failed before
the repair in both memory and durable-store variants. The shared routing
affinity-release operation now retains an accompanying session whenever a hard
response or turn identity is present. Exact hard-identity bindings still release;
session-only release still works. This also covers the existing HTTP quota
consumer without duplicating policy at each call site. The change stays in
`usecase/routing`; no persistence format, dependency, gateway or helper changes.

The committed transport oracle adds `quota-history` with the same authenticated
Ready prerequisites, exact wire assertions and durable snapshot requirements.
Its official log must show the quota recovery event rather than a transport event.
Clean-candidate qualification of this expanded seven-case slice follows the
local regression checks. Strict compact-followup remains unverified: even a
compact-only turn lineage was resolved by the official executable through its
ordinary turn-owner path, which allowed full-history replay. That experiment
must not be counted as the strict no-replay control.

## Native transport checkpoint and close/Pong observation

Native run `39272192121` on clean `714e82f1` passed the six-scenario transport
phase on macOS and all seven native modes on Windows. The Windows lane's final
native-executable parity marker passed. The macOS realtime mode still failed
inside official Prodex before a Godex comparison; the quota snapshot schema
repair did not resolve it. The native transport results do not cover the later
quota-session production repair.

The expanded seven-case transport/quota phase passed both pinned executables on
clean `624649ec8df4b64b7a92df60072dc4fd24a83dcf`. It includes the newly fixed
quota-recovery session conflict plus all previous transport lifecycle controls.
`go test ./... -count=1 -timeout=120s` passed 4,367 tests in 81 packages and
`make verify` passed before that production repair was pushed.

The earlier macOS committed-idle failure stopped on a Pong write error without
reading any queued close frame. A deterministic harness regression reproduced
that loss with a complete empty CLOSE already buffered. The liveness reader now
consumes that complete close before deciding the result. A failed Pong still
rejects following application output, another Ping, abrupt EOF and a truncated
close. Existing wire assertions still require the exact empty close and idle
budget. This is an observation repair; it does not prove the exact cause of the
earlier native failure or change Godex's transport behavior.

The close/Pong observation repair passed the pinned
`./scripts/verify-release-parity.sh --phase=websocket-core` on clean
`b947d1d65fac0dabd86919278b6015ab8bd8a68e`. All Linux core observations,
including committed idle close and realtime permit lifecycle, passed. This
does not resolve the separately observed macOS official-sidecar failure.

## Full-context retry signals preserve accompanying session ownership

A new durable snapshot check on clean `624649ec` exposed a separate deletion
after the exact `400 previous_response_not_found` WebSocket retry signal.
Official Prodex removed the failed response and turn bindings but retained
the accompanying session on main; Godex removed all three. Neither product
dispatched an alternate inference. Both snapshots were read after process
shutdown, so this difference was not a pending-write observation.

The retry-signal consumers now use the existing shared dead-binding release
operation, which enforces the tagged hard-identity/session policy. This covers
post-upstream WebSocket failures, pre-send quota signals and the equivalent
HTTP Responses/Compact signal. Ordinary soft-session rotation keeps its existing
release operation. Fail-before regressions cover transport, quota and pre-send
WebSocket paths in memory and durable storage, plus HTTP transport/quota paths.
Older quota assertions now require the retained exact session owner and still
verify that a following full-history request recovers on the alternate.

The executable negative controls now verify durable state as well as exact
frames and no replay: delta-only retains its original hard bindings, while the
previous-response signal releases them; both retain main's session and create
no alternate response binding. No new model, repository format, helper package,
gateway, dependency or background worker is introduced. Clean executable
qualification of this repair follows the local tests.

The strengthened seven-case phase passed on clean
`45d7583ce6e18ecd2ae09b061cf28a5f30035b02`. Both executable snapshots retain
main's session for delta-only and previous-response terminal failures; only
the latter releases the original response and turn. Exact frame, request count,
handshake and lifecycle assertions also passed on that commit.

## Missing response without a previous ID retains the original turn owner

An independent two-executable experiment on clean `624649ec` sent a known
main-owned turn and full history without `previous_response_id`, then returned
an upstream `400 previous_response_not_found`. Both products dispatched main
then second and preserved the opaque turn header, nested metadata, large
integers, tool arguments and exact visible completion frames. Official Prodex
retained the original turn and unrelated response on main, recorded the new
response on second and persisted the accompanying session conflict. Godex
instead removed the session and reassigned the original turn to second.

The fail-before memory/durable regression now covers that exact ownership
difference. Routing preserves the existing hard turn on this missing-response
path and removes only a session-only preference when appropriate. After an
alternate succeeds, the requested token itself is excluded from new ownership
evidence; any returned response/turn identity is still observed normally.
The original request's soft-session status is captured before recovery mutates
its binding keys, preserving the established transport/quota session conflicts.
No new model field, gateway contract, helper, persistence format or dependency
is needed.

The committed transport slice adds `turn-missing` with exact unchanged replay
bytes, original-turn retention and matching durable snapshots. Its official log
requires the missing-response decision and authenticated Ready alternate. The
existing transport/quota cases still require stale-token removal. Mutations
reject removing either the turn header or metadata from this distinct replay.
Clean-candidate executable qualification follows the local checks.

The complete default release gate separately passed on clean
`624649ec8df4b64b7a92df60072dc4fd24a83dcf`, including all original finite
oracles and the seven-case transport/quota slice. Native run `39273563445`
also passed every Windows mode on that commit and the expanded transport slice
on macOS; macOS realtime still failed in official Prodex. Those results are
not attributed to the later retry-signal or missing-response ownership fixes.
