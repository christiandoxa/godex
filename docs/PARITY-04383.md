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

The expanded eight-case executable phase passed on clean
`a2c193874cacf110b0656a30285e30e9fe68e475`, including the unchanged
turn-missing replay, original hard-turn retention, new response ownership and
session conflict. All earlier transport/quota/negative/lifecycle controls passed.

Native run `39274749877` on the earlier `45d7583c` passed all Windows modes
and the seven-case macOS transport slice with durable negative-state checks.
Its sole native macOS failure remained official Prodex realtime setup.

## Unbound turn discovery can recover independently complete history

The next canonical experiment removed the turn binding and status before
startup, completed both authenticated quota probes and used an owner-pinned
final warmup to make main current. Official Prodex attempted main then second,
stripped the failed unbound turn token, preserved complete history and output,
and recorded no owner for that token. Its log independently reported
`affinity_released=false`, `reports=1 ready=1` and a healthy authenticated
`selection_pick` for second. Godex instead returned terminal 503 without recovery.
This reproduces on clean `624649ec`; it is distinct from a bound-owner setup.

Fail-before regressions now cover complete-history recovery and delta-only
refusal in memory and durable storage. Unbound discovery invokes the existing
bounded transport-recovery workflow after an actual precommit failure. That
workflow permits a truly missing owner but rejects a turn owned by another
profile, conflicting ownership, previous-response dependencies, incomplete
history and the existing commit/deadline restrictions. Replaying an unknown
request token does not create an owner for it. No new model field, exported API,
helper package, dependency or background worker is introduced.

The executable slice adds `unbound-history` and `unbound-delta`. Preparation
checks that no turn owner exists, and the positive case requires the official
no-release marker plus the Ready selection rather than a bound-owner marker.
The delta control requires one attempt, exact 503, retained main session and
unrelated response, and no turn/recovered-response binding. The resulting
ten-case slice requires clean-candidate qualification before it is counted.

The ten-case transport phase passed both pinned executables on clean
`5dec0593ef98f0a162eb7ae68d8ec5a27b906096`, including both new unbound
cases, exact Ready/no-release evidence, wire preservation and durable owners.

## Share the full-history proof with unbound quota recovery

The quota variant reproduced the same unbound discrepancy on clean `a2c19387`:
official Prodex recovered the complete history on second, while Godex forwarded
the original 429. A separate pinned negative experiment on clean `5dec0593`
confirmed that delta-only input must preserve the exact original quota frame
with one main inference and no new turn or response ownership in both products.

The shared routing workflow now accepts either a precommit transport or quota
failure after the same complete-history, owner, Ready, deadline and commitment
checks. Unbound discovery passes its existing typed quota evidence into that
workflow. The superseded bound quota replay block is removed, so both consumers
use one policy and neither can bypass its cached-quota check. The private file
and operation are named for full-history recovery; domain and layer ownership
remain `usecase/routing`, without a new interface, model field or helper package.

The quota complete-history regression failed before the repair in memory and
durable variants. Both transport and quota regressions retain delta refusal and
correct session/original-response ownership. The executable slice adds
`unbound-quota-history` and `unbound-quota-delta`, with exact 429 and no-replay
mutation controls. The expanded twelve-case slice still requires qualification
on a clean candidate before these new quota cases are counted as VERIFIED.

The twelve-case slice passed both pinned executables on clean
`20014bb5b29a5caf546cfa04e4e6b9076c4ff698`. Both bound and unbound
transport/quota histories recovered with matching exact frames and durable
owners; all delta, previous-response, missing-response and lifecycle controls
passed on that commit. This remains a finite Linux qualification.

## Deadline classification cannot become an empty HTTP 200

Native Windows run `39275609588` failed its ordinary tests on clean `a2c19387`
before reaching the executable oracle. The existing Compact watchdog regression
received HTTP 200 with an empty body instead of typed 504. Delivery read the
request's context error separately for timeout and cancellation, allowing a
local deadline to fire between those reads and suppress the error response.

`TestProdex04383DeadlineBetweenErrorChecksNeverReturnsEmpty200` deterministically
reproduced the empty 200 for both Responses and Compact before the repair. It
uses the real routing watchdog with controlled context expiration during error
classification. Delivery now samples the context error once in upstream-error,
body-capture-error and admission-error handling. Existing timeout status, public
message and cancellation behavior remain intact. The deadline helper stays
private to `delivery/http/proxy`; no gateway, repository, use-case contract,
shared helper package or dependency changes.

The repaired delivery/routing race suite passed 994 tests. Fifty repetitions of
the new race regression and original watchdog test passed 200 tests under race
detection. Native Windows re-execution remains required before the original
native failure is considered resolved.

## Complete finite gate after ownership and deadline repairs

The complete `./scripts/verify-release-parity.sh` gate passed on clean
`39c9cc1ba891047c00c7889c7e6a10da45494ee1`, including all original
finite phases and the expanded twelve-case transport/quota slice. Reference
source, executable and manifest pins were verified for that exact candidate.
Before its push, `go test ./... -count=1 -timeout=120s` passed 4,401 tests
in 81 packages and `make verify` passed its full local checks. The targeted
delivery/routing race suite and fifty repeated watchdog/race regressions
also passed as recorded above.

This is **VERIFIED finite Linux parity**, not FULL 1:1 feature parity. Actual
latest Windows confirmation of the deadline repair remains pending. The pinned
macOS official realtime-sidecar failure, a proven strict compact-followup
executable negative control, and live OAuth/keyring/provider/network coverage
remain **OPEN/PARTIAL**. The successful ten-case macOS transport run on
`5dec0593` does not qualify later commits or the failed realtime mode.

## Full-history recovery waits for a busy healthy alternate

A pinned two-executable comparison on clean `1e7ea486` held second's full
two-unit capacity with an authenticated HTTP Responses stream. Both profiles
had applied healthy quota probes before main's full-history WebSocket attempt
failed before commitment. Official Prodex released the failed turn, retained
main's session and unrelated response, and waited until the original 30-second
deadline returned exact `504 request_timeout`. Godex returned `503
service_unavailable` after 83 ms and retained the failed turn instead.

The fail-before routing regression covers transport and quota failures with
memory and durable bindings. Full-history recovery now reuses the existing
full-context candidate policy and fresh-request admission wait. Busy capacity
does not invalidate an otherwise healthy alternate; quota/auth exclusions,
history proof, stale-token scrubbing and the original cancellation/deadline
remain in force. The unsafe-replay tests now use reconstructable history so
each independent rejection condition is exercised.

The mandatory transport slice adds `busy-history`, its thirteenth scenario.
It checks the exact timeout, one main inference and handshake, no alternate
dispatch, an independently live capacity blocker and the final durable owners.
The blocker uses only synthetic authenticated loopback traffic. Clean-candidate
qualification follows local checks; this result does not establish native
macOS realtime or full feature parity.

The first expanded gate on clean `bdd10fe5` rejected a second main-owned
attempt after a capacity wait. Fresh routing reloaded the global pool and
forgot that this handoff had excluded main. Its existing retryable-candidate
map now retains that explicit exclusion across every reload. The strengthened
fail-before regression advances a controllable clock beyond cooldown expiry,
keeps second busy for two wait epochs and then cancels without another model
dispatch. Other fresh-request callers keep their dynamic pool reload behavior.

The rerun on clean `b94bdc13` rejected a second attempt on second shortly
before the waiting WebSocket's deadline. Its capacity blocker had visibly
committed, but Godex's delivery-owned HTTP deadline still canceled that
upstream body at 30 seconds. This released its permit roughly 80 ms before
the later WebSocket deadline. Official Prodex kept the committed blocker live.

The fail-before HTTP integration regression now holds visible SSE output past
the original local deadline and checks that explicit client cancellation still
reaches upstream. Delivery owns a stoppable local request watchdog, disarmed
at HTTP or WebSocket commitment. Parent deadlines and cancellation remain in
force, and admission/body capture/precommit work retain their original budget.
No retry assertion or capacity-blocker requirement was relaxed. Qualification
of the combined repairs follows local tests.

The thirteen-case phase passed on clean `a44f67b4`. A subsequent contract audit
found that its custom context changed `Deadline()` after commitment, contrary
to Go's stable-context contract. The final implementation uses the standard
cancel-cause context and keeps only the stoppable application timer in delivery's
scope. Parent deadlines remain unchanged; original accepted-time budgeting and
typed local 504 classification retain direct regression coverage. The full
gate begun on the earlier diagnostic integration was stopped for this change;
it is not recorded as passing.

## Preserve bounded native managed-launch diagnostics

Native Windows run `39278954754` on clean `39c9cc1b` passed ordinary Go tests
and the six preceding executable modes, including the watchdog regression.
The transport slice passed every official Prodex scenario, then Godex exited
before publishing the first synthetic child endpoint. The harness discarded
its stderr, so the cause remains **OPEN**; it is not attributed to the watchdog
fix or to a fixture defect without further evidence.

Managed-launch failures now include at most 8 KiB of captured stderr after
credential redaction. The child stream is fully drained and diagnostics are
read only after process completion. The focused regression rejects truncation
leaks, secret exposure across write boundaries and lost operation context.
This tooling change does not retry a failed launch or relax any parity check.

## Renew the local request budget on a reused HTTP connection

A two-executable experiment on clean `07551467` configured both stream-idle
budgets to 2 seconds, yielding the tagged 30-second precommit minimum. Both
clients reused the same TCP connection for a second request after 31 seconds.
Official Prodex returned 200 with two main-owned upstream requests. Godex
returned local `504 request_timeout` and dispatched only the first request.
The earlier default-budget experiment returned 200 in both products and was
not used to establish this discrepancy.

HTTP delivery now atomically claims the connection's acceptance timestamp
once. Its first request still includes time waiting after accept; subsequent
keep-alive requests receive a fresh admission budget. Fail-before coverage
checks both the preserved first-request queue age and two real HTTP requests
on one reused socket past a short test budget. No persistence, gateway,
model, helper-root, dependency or background-worker change is needed.

The independent `--http-keepalive` oracle requires the same TCP connection,
one connection acquisition per request, exact 200 bodies, unchanged input,
main ownership and exactly two upstream requests after the original budget.
Mutations reject timeout reuse, reconnects, automatic HTTP retries, extra or
missing inference, owner/input/output drift and a wait shorter than 30 seconds.
`--phase=http-keepalive` is mandatory in the default gate and native matrix.
Clean-candidate executable qualification follows local checks.

The complete `./scripts/verify-release-parity.sh` gate separately passed on
clean `07551467481ad853f692c0ecde305b5f4ab83f70`, including the thirteen-case
transport slice and all earlier finite phases. That gate did not yet contain
the newly discovered HTTP keep-alive case and does not qualify its repair.

## Exact clean keep-alive qualification and native checkpoint

`./scripts/verify-release-parity.sh --phase=http-keepalive` passed on clean
`2022bfb055c2cf0d152acf8286d9c20f8d392b8d`. Both products returned exact
200 responses on the same TCP socket after 31 seconds, with exactly two
main-owned upstream requests. The focused delivery/tooling race suite and
twenty repeated first-request/keep-alive regression runs passed before push,
as did `make verify`.

Native macOS run `39287310068`, job `118177903452`, passed the new keep-alive
mode on that exact commit, including both same-socket 200 responses and two
upstream requests. Its sole failing native mode remains official Prodex's
realtime sidecar: it logs `local_read_error` immediately after upgrade and
resets the session-update connection. That failure remains **OPEN**; passing
the other modes does not qualify the complete native matrix.

## Bound live-goal test writers and join canceled fixtures

Native Windows run `39285545927`, job `118170310202`, failed an ordinary
live-goal regression on clean `07551467` before reaching the executable
oracles. The test writer returned `SQLITE_BUSY`, then temporary-directory
cleanup found its SQLite file still open. The production observer already
uses read-only connections with a two-second busy timeout and closes them
after each observation; this failure did not establish a production leak.

The private CLI runtime fixture opener now gives every writer one connection,
a bounded two-second busy timeout and unconditional test cleanup. Both
asynchronous recovery tests cancel and join their runners even on failure;
their fake children honor cancellation while waiting for the trigger. Ready
alternates are established before publishing the recoverable goal update.
The existing same-goal, affinity-release and multi-generation assertions
remain intact. No production boundary, dependency or persistence format changes.

Fail-before controls independently observed an immediate locked-write error
and both child fixtures ignoring pre-trigger cancellation. They now prove a
bounded lock wait, successful write after lock release and cancellation with
the trigger still unopened. Focused race checks and twenty repeated recovery
and cancellation runs passed. Native Windows confirmation remains required;
this fixture repair does not certify the separate managed-launch issue.

The complete default release gate subsequently passed on clean
`2022bfb055c2cf0d152acf8286d9c20f8d392b8d`, including the thirteen-case
transport slice and HTTP keep-alive mode. Native Windows job `118177903636`
in run `39287310068` passed all eight executable modes on that same commit,
including managed transport launches and keep-alive renewal. This successful
run does not explain the earlier intermittent launch failures. On `68182776`,
both portable lanes passed ordinary Go tests, including the repaired live-goal
fixtures; their native executable modes were still running at this checkpoint.
The separate Sonar lane failed coverage generation in
`TestProdex04355DeepSeekStreamIdleResetsAfterEachChunk`; it did not reach its
quality analysis. That timing failure remains unresolved here.

## Recognize an empty HTTP response as a transport failure

A synthetic paired-executable experiment on clean `68182776` established two
authenticated Ready profiles, retained main's turn owner, then closed its
Compact HTTP connection before any response headers. Official Prodex retried
second with the same opaque request and returned 200; a previous-response
dependency instead returned exact 400 `previous_response_not_found` without
dispatching second. Godex returned 502 in both cases and retained main's owner.

Go's HTTP client reports this failure as a typed `url.Error` wrapping `io.EOF`.
Routing incorrectly rejected that cause, bypassing its existing transport
backoff and bound-recovery policy. The shared classifier now recognizes that
typed HTTP failure. Bare reader EOF and untyped `EOF` text remain ordinary
end-of-input evidence, and caller cancellation still prevents recovery.
Fail-before classifier and Compact session/previous-response regressions
cover the missed branch. No gateway, delivery, model or helper API changes.

The first three-package race run timed out in the existing multi-sweep HTTP
recovery test once EOF started applying real transport cooldowns; its concurrent
`make verify` was stopped. The fixture now advances its injected clock across
those waits and also requires at least one full transport cooldown. Its five
dispatches and final output are unchanged. The repaired three-package race
suite passed 1,213 tests, twenty repeated HTTP recovery checks passed, and the
final `make verify` passed. Clean-candidate executable confirmation follows.

The experiment also exposed turn-only Compact recovery as a separate open
case: it has no session key, so its bound fallback guard currently rejects
recovery even after transport classification. No strict compact-followup
WebSocket qualification or FULL parity is claimed by these observations.

## Recover a Compact turn without replacing its old owner

The same paired experiment on clean `86a157de` confirmed the HTTP EOF repair:
the session-bearing Compact request now returns the exact reference 200 body,
and its previous-response control returns the exact 400 without replay. Two
differences remained. Godex rejected a turn-only request with local 503 while
Prodex retried second and returned 200. With a session, Godex rebound the failed
turn and session to second; Prodex removed the failed turn and persisted the
session conflict while keeping the unrelated response on main.

Routing now admits an owned Compact turn to the existing fallback workflow
without requiring a session or thread key. It reuses the existing dead-identity
release policy, which preserves the accompanying session and prevents a
successful replay from binding the failed token again. The outbound request
retains its turn header and nested metadata exactly, as the reference requires
for Compact. This differs from the WebSocket full-history recovery workflow.
No new helper, port, model, dependency or persistence format is introduced.

Fail-before regressions cover both turn-only and accompanying-session requests
in memory and through the real routing repository. They check dispatch count,
opaque bytes and large JSON numbers, failed-token removal, persisted dead
status, unrelated-response retention and the exact session conflict. Initial
durable fixture setup was corrected to use valid synthetic IDs and restore
continuation statuses as the production constructor does. The focused Compact
race suite now passes; clean executable qualification follows local checks.

The repeated Compact race regression passed 100 checks and `make verify`
passed before `774f6286` was pushed. A paired experiment on that exact clean
commit matched the two 200 recoveries, the one-request 400 reset and all
three durable-owner projections. Those observations remained exploratory.

The new mandatory `--phase=http-compact` gate turns them into strict assertions
against both pinned executables: original ownership after the two warmups,
applied Ready quota evidence, exact upstream counts and owners, unchanged wire
bytes including large integers, exact final output/status and durable state
after orderly shutdown. Its previous-response control rejects replay, and
mutation tests independently reject each material deviation. The default gate
and native matrix now include this ninth mode. Clean-candidate qualification
follows local checks; the strict WebSocket compact-followup gap remains open.

The strict oracle rejected clean `86a157de` at the expected Godex turn-only
503 after all three official Prodex cases passed. It then passed all three
cases against clean `774f6286`. The tooling race suite passed 260 tests,
including fifty Compact oracle checks; `bash -n` and `make verify` passed.
The release phase must still qualify the committed harness on its exact clean
candidate. No native nine-mode result is claimed before actual execution.

`./scripts/verify-release-parity.sh --phase=http-compact` passed on exact clean
`68a60bfd50f9bd86eb573a22826df7d5891d6beb` before its push. All three cases
and mutation controls passed with source, executable and manifest provenance
checked by the release gate. This verifies the finite HTTP Compact slice;
the complete new default gate and native nine-mode matrix remain separate.

## Make native stream-idle fixtures independent of host scheduling

Run `39289226215` failed Sonar coverage generation on `68182776` because
`TestProdex04355DeepSeekStreamIdleResetsAfterEachChunk` lost its second delta
under host scheduling delay. Its 100 ms chunk delays and 250 ms idle limit
depended on wall time. This failure occurred before quality analysis and did
not establish a production stream-timer defect.

The three private native-stream idle tests now use the standard library's
`testing/synctest`, already used elsewhere in the repository. The same fixture
chunks, delays, idle limits, timeout bound and terminal-output assertions remain
in force. Virtual time advances only while the owned goroutines are blocked;
the tests still exercise the real gateway timers and shutdown. No production
clock hook, timeout change, dependency or helper package is needed. One hundred
repeated race runs passed all 300 checks, and the gateway coverage run passed
247 tests. Native re-execution is still needed to qualify the repaired fixture.

## Preserve an owned HTTP Responses turn after transport failure

A paired experiment on exact clean `cef9c84c` reused the authenticated Ready
setup with the HTTP Responses route. With a known turn and accompanying
session, official Prodex returned exact 503 after one main-owned request and
retained the turn, session and unrelated response. Godex replayed on second,
returned 200 and replaced the turn owner. The turn-only 503 and previous-response
400 controls already matched. Compact's two-profile replay policy therefore
cannot be applied to this Responses failure.

Routing now checks the existing turn owner before recovering this HTTP
transport failure. A verified turn on the failed account returns the existing
local 503 and preserves its bindings. Unknown turns, previous-response reset
handling, quota full-history recovery, Compact and WebSocket policies retain
their existing paths. Owner-lookup failures propagate with response cleanup;
no new helper, public contract, persistence format or dependency is introduced.

The fail-before memory and real-repository tests use reconstructable full
history, require a single dispatch and check all three owners after failure.
The focused suite, including Compact and quota recovery controls, passes.
Clean-candidate executable confirmation follows local checks; this does not
qualify the separate strict compact-followup WebSocket branch.

Before `69d9b857` was pushed, `make verify`, the 26-test focused race suite and
sixty repeated memory/durable checks passed. A paired experiment on that exact
clean commit then matched all three terminal responses, single-request counts
and durable owners. These observations remained exploratory.

The new `--phase=http-turn-transport` gate reuses the existing HTTP fault fixture
with a separate Responses verifier. It asserts the two exact 503 no-replay
cases and the exact previous-response 400 control, applied authenticated Ready
evidence, unchanged full history and turn metadata, and all durable owners after
shutdown. Mutation controls reject each material deviation. The default gate
and native matrix require this tenth mode; the HTTP Compact verifier and its
controls remain active. Clean-candidate qualification follows local checks.

The strict Responses oracle rejected clean `cef9c84c` at the expected two-request
200 replay after all three official Prodex controls passed. It passed all three
cases against clean `69d9b857`. The tooling race suite passed 306 checks,
including both HTTP verifiers, and `bash -n` plus `make verify` passed. Both
HTTP phases must still qualify the shared fixture on this committed harness;
no complete ten-mode or native result is claimed before actual execution.

Both `--phase=http-turn-transport` and `--phase=http-compact` passed on exact
clean `3b5bb352c2eb9e9e17dd8f98b02449a891f03708` before push. The strict
Responses and Compact transport slices are **VERIFIED** for that commit.
Separately, the complete nine-mode default gate passed on clean `cef9c84c`,
including its Compact mode and virtual-time stream fixtures. That historical
gate predates the owned HTTP Responses repair and tenth mode; the complete
ten-mode gate remains a separate qualification.

## Observe native realtime socket inheritance

Native macOS job `118196088209` on clean `cef9c84c` again passed HTTP Compact
and keep-alive modes but failed official Prodex realtime session update directly
after upgrade with `local_read_error`. Tagged `realtime_sidecar.rs` creates a
nonblocking listener, accepts its raw stream and sets receive timeouts without
explicitly resetting the accepted socket mode. Inheritance remains a hypothesis
until the host's behavior is observed; the harness does not pipeline a frame
before the required 101 response or retry away the failure.

The realtime oracle now emits an isolated raw-socket diagnostic before opening
either product. On Linux/macOS it configures a nonblocking loopback listener,
observes the accepted descriptor flags and performs an idle read with the same
20 ms receive timeout. Connection readiness is bounded by polling; every
descriptor is closed before the executable fixture begins. Other hosts report
that this diagnostic is unmeasured. It uses the already-pinned `x/sys` module
and introduces no application hook, dependency or protocol change. The native
oracle's assertions remain intact; raw socket evidence alone does not prove
the exact cause inside the official binary or qualify native realtime parity.

Ten repeated Linux probe race checks, the 307-test tooling race suite and
`make verify` passed. The oracle test package also cross-compiled for
`darwin/arm64` and `windows/amd64`; neither compilation is a native execution.
The exact clean executable comparison and actual macOS measurement follow
these local checks.

The complete ten-mode gate passed on exact clean
`3b5bb352c2eb9e9e17dd8f98b02449a891f03708`, including both strict HTTP
transport modes and all earlier finite phases. It does not cover the later
mixed-identity discrepancy below or establish FULL feature parity.

Native macOS job `118205235858` on clean `e060f427` measured
`accepted_nonblocking=true` and `read_wait_us=1`; the Linux core gate measured
`accepted_nonblocking=false` and `read_wait_us=21614`. Host inheritance is now
observed and supports the sidecar hypothesis, but the official binary's exact
failing syscall remains uninstrumented. The native job still failed official
realtime. It also failed Compact's final session projection: the expected
conflict had not become durable when the harness stopped the child. Tagged
`lifecycle.rs` detaches workers on non-test drop without waiting for state
saves. The fixture needs a bounded durable-state observation before releasing
the managed child, followed by the existing post-shutdown assertions. That
native Compact qualification remains **OPEN** pending this fixture correction.

## Require an owned previous response before a transport reset

A paired experiment on clean `e060f427` supplied an unknown previous-response
ID beside a verified main-owned turn and session. Official Responses returned
503 after one main request and retained those owners. Official Compact retried
second, returned 200, removed the failed turn, retained the unrelated response,
left the unknown ID unbound and persisted the session conflict. Godex returned
400 and removed the turn in both routes. The known previous-response controls
already matched the exact one-request 400 reset.

HTTP transport recovery now checks the previous ID's actual owner before
choosing that reset. Responses retains its verified turn when the previous ID
is unowned. Compact reuses its existing alternate recovery and removes only
the unverified input ID from binding accounting; the entire outbound request
remains unchanged. This prevents a successful Compact replay from inventing
an owner for the unknown ID or losing the accompanying session conflict.
Other failure classes retain their existing policies. No helper, dependency,
model, persistence format or public API is added.

All four memory/durable route regressions failed before the patch. The focused
suite passed 31 checks, twenty repeated runs passed 100 checks and `make verify`
passed. Exact clean executable confirmation and the expanded strict HTTP
controls follow these local checks; no mixed-identity qualification is claimed
from the exploratory output alone.

## Strict mixed-ID HTTP controls and durable child lifecycle

Both mandatory HTTP transport modes now include an `unknown-previous` case
beside turn-only, with-session and known previous-response controls. Each
product must first prove both authenticated profiles Ready and the original
turn/session owners intact. The strict verifiers compare status, exact body,
dispatch count, account order and opaque request bytes. They also observe the
unknown input ID in both durable formats and reject any invented owner.
Responses must retain the original turn/session after its one-request 503;
Compact must retry second, return 200, release the failed turn and persist the
session conflict while keeping the unknown ID unbound.

The shared HTTP workflow now waits at most three seconds for the complete
expected durable projection before stopping the managed child. This follows
the existing bounded state-observation pattern used by the WebSocket fixture.
It then rereads state and recounts upstream attempts after shutdown and repeats
the same verifier. No durability, wire, readiness or no-replay assertion is
relaxed. A deterministic `testing/synctest` regression publishes a debounced
session conflict after the first observation, requires the wait to see it and
rejects state that never settles. Its read-once control failed before polling
was added. These changes stay in developer tooling; production layer ownership,
dependencies and persistence formats remain unchanged.

`rtk go test -race ./tools/continuationparity -count=1 -timeout=120s`
passed 348 checks and `rtk make verify` passed. The expanded strict tool rejects
clean `e060f427` at each route's unknown-ID control: the old executable returns
400 after one attempt. Both four-case modes pass against exact clean
`061043c69a5e74c8479e4377063c1345929fe5ba`, including the official controls
and all durable projections. The committed harness phases follow this check.

The complete default gate also passed on exact clean `e060f427`; that older
gate lacks the mixed-ID cases. Native Windows job `118205235978` passed all
ten modes on that commit. Native Compact confirmation with the corrected
lifecycle, the strict WebSocket compact-followup control and official macOS
realtime remain **OPEN**.

Both expanded HTTP release phases passed on exact clean
`16f3eefbed05d51183ba81b04a26122901be709b`. Each mode ran all four cases
against both pinned executables, then its mutation suite. Native Compact
confirmation with the corrected child lifecycle still requires host execution.

## WebSocket transport reset requires the actual previous owner

A targeted paired experiment on exact clean `16f3eefb` reproduced the mixed-ID
problem on WebSocket too. Both profiles had authenticated, applied Ready
evidence. An unknown previous-response ID beside the verified main turn/session
produced one main request on each product. Official Prodex returned exact 503
`service_unavailable`, kept the turn/session and unrelated response, and left
the unknown ID unbound. Godex returned the 400 full-context reset. The known-ID
control already matched the one-request 400 and retained accompanying session.

`usecase/routing` now checks that the previous ID belongs to the failed account
before authorizing a WebSocket transport reset. An unowned ID keeps the pending
terminal failure and existing owners. Known-ID resets, non-transport recovery
and previous-response no-replay rules keep their existing policies. The ten-line
change stays at the consuming routing boundary; no helper, model, dependency,
persistence format or gateway behavior is added.

The new memory/durable regressions failed before the fix on both unknown-ID
cases. They check the exact terminal event, one original-owner attempt, unchanged
opaque body/header, retained unrelated response and durable owners. The focused
race suite passed 41 checks, twenty repeated regression runs passed 100 checks
and the tooling race suite passed 356 checks. `rtk make verify` passed. The
mandatory transport mode adds
the unknown-ID negative control as its fourteenth scenario, requires its ID to
remain unbound and rejects mutations that substitute the known-owner 400 reset.
Exact clean executable qualification follows local validation. This control does
not qualify the separate strict WebSocket compact-followup branch.

The fourteen-case `--phase=websocket-transport` gate passed on exact clean
`74c48c4b1cd3b20188475961dc503a38fc9cb219`, including the known/unknown
previous-response pair. That commit was pushed and its remote HEAD confirmed.

Native macOS job `118216934132` on clean `16f3eefb` passed both four-case HTTP
modes, including Compact's debounced conflict and unknown-ID projection. This
confirms the corrected durable child lifecycle on a native host. Official
realtime still failed immediately after 101 with `local_read_error`; its isolated
raw accept probe again measured nonblocking inheritance and a 1-microsecond read.
That separate failure remains **OPEN**.

## Preserve WebSocket application failures beside an unowned previous ID

A six-case paired experiment on clean `74c48c4b` covered quota, rate-limit and
overload failures with known and unknown previous IDs. The three known-ID
controls matched exact one-request 400 reset events and accompanying-session
retention. With an unknown ID beside the verified turn/session, official Prodex
instead forwarded the exact original 429 quota/rate or 503 overload frame,
retained the turn/session and unrelated response, and never bound the unknown
ID. Godex incorrectly returned 400 and removed the turn in all three cases.

The existing routing owner check now applies to every bound WebSocket retry
outcome, following the tagged full-context eligibility policy. This removes
the transport-only exception and preserves the upstream failure when the input
ID has no matching owner. The implementation shrinks by two lines. No new
helper, production boundary, dependency, persisted field or configuration is
needed. Pre-send quota and live authentication are not qualified by this slice.

The renamed memory/durable regression covers all four failure classes with
both known and unknown IDs. Its six new unknown-ID cases failed before the
change. The focused race suite passed 53 checks and twenty repeated runs passed
340 checks. The mandatory transport oracle adds all six application cases as
scenarios 15–20, comparing exact terminal frames, one authenticated main attempt,
opaque history/header and durable owners. Fixture golden checks prevent an
application error from silently becoming EOF, while mutation checks reject a
400 reset, an empty frame, changed status or model replay. The initial 429
status mutation did not alter its fixture; it was corrected and all 91 focused
oracle checks passed. Clean executable qualification follows local validation.

The final tooling race run passed 375 checks and `rtk make verify` passed.
The expanded strict tool rejects clean `74c48c4b` at `unknown-quota` after all
twenty official controls pass: the old executable substitutes the known-owner
400 frame. The complete default gate passed separately on exact clean
`16f3eefb`; it predates the WebSocket mixed-ID and application-failure additions
and does not qualify those newer cases.

## Strict Compact WebSocket missing-response keeps its original owner

The 0.438.3 WebSocket `previous_response_not_found` handling may rotate an
ordinary turn-only Responses request to a different Ready owner while keeping
its verified turn identity. The same retry is **not** legal for a strict
Compact-followup: it is a continuity-bound request whose input must remain on
its original profile. The prior routing helper applied the ordinary turn-only
rule even when `QuotaSelection.RouteKind` was Compact, and incorrectly
forwarded a missing-response failure to an alternate profile.

`TestProdex04383CompactFollowupMissingResponseCannotRotate` failed **before**
the scoped routing fix for both memory and durable affinity stores: each
fixture dispatched on owner A and then unexpectedly inferred on owner B. The
fix excludes Compact from that single previous-response rotation helper,
without changing the Responses path, release policy, provider, retry budget,
model, persistence format, or repository boundary. The regression asserts one
owner-only dispatch, the original classified precommit error, unchanged input,
and retained turn/session bindings. The existing ordinary turn-only recovery
tests remain in force.

This is tagged-source-aligned **local policy proof**. It does not resolve the
separate strict cross-executable Compact-followup WebSocket fixture, which
remains **OPEN** until an official 0.438.3 binary differential confirms the
same user-visible terminal frame, attempt count and durable owner on both
products. A passing unit test must not be substituted for that oracle.

## WebSocket direct JSON fields shadow invalid nested metadata

The tagged Prodex 0.438.3 source (`prodex-runtime-proxy/src/lib.rs`,
`runtime_request_semantic_plan`) chooses an existing top-level `session_id`
or `x-codex-turn-state` JSON member **before** inspecting `client_metadata`.
An empty, whitespace-only, null, boolean, numeric, or object-valued direct
member yields no string and must **not** borrow the nested session/turn
identity. Godex's WebSocket metadata parser previously fell back to nested
values whenever the direct member was not a nonblank string. Its shared HTTP
affinity parser independently resurrected the same discarded keys, allowing
a caller-supplied malformed field to select a different existing owner.

`TestProdex04383WebSocketDirectMetadataShadowsNestedEvenWhenInvalid` failed
before the fix for all six direct-value shapes. The real Router owner-selection
regression `TestProdex04383WebSocketInvalidDirectDoesNotSelectNestedOwner`
also failed in both memory and durable stores: its preferred Ready owner B
was incorrectly replaced by owner A from shadowed nested metadata. A scoped
change preserves the direct-member presence rule in both WebSocket metadata
and generic affinity extraction, without changing HTTP requests or the
historical nested response.previous_response_id recovery contract. Missing
direct members still use genuine nested metadata, and explicit handshake
session/turn headers still take precedence. No new public setting, persisted
field, helper package or dependency was introduced.

These are **source-aligned local regressions**, not yet a pinned two-executable
network oracle of malformed direct metadata. Independently qualify the exact
client-visible frames and owner/durable effects before marking this boundary
FULL 1:1; retain all previous continuation oracles and no-replay controls.

## Complete authenticated 20-case WebSocket transport gate — clean `cf8aafc`

The full original authenticated WebSocket transport fixture was qualified in
three **non-overlapping** synchronous release phases on the **same clean Godex
commit** `cf8aafcee1e3876fc0fd8da0fd8bbe0dea37cc75`. Each phase verified
the published Prodex 0.438.3 source tag, Linux x86_64 executable SHA-256,
published release manifest and candidate's clean Go VCS provenance before
comparing the two executables. Each exited **PASS**:

| Release phase | Original scenarios | Exact result |
| --- | ---: | --- |
| `--phase=websocket-transport-core` | 17 | PASS; authenticated Ready full-history, quota/error precedence, continuation ownership and no-postcommit-replay controls |
| `--phase=websocket-transport-lifecycle` | 2 | PASS; cancellation and original 30-second precommit deadline with no replay |
| `--phase=websocket-transport-capacity` | 1 | PASS; occupied healthy alternate, unchanged original deadline and persisted owner |

The tooling test `TestProdex04383WebSocketTransportSlicesExhaustive` enforces
that these phases concatenate to the *unchanged* twenty-case canonical suite
without omissions, duplication or reorder. Both official executables executed
identical fixture cases with the original authenticated receipts, owner,
request/history, client-frame and durable-state checks. The exact
`TestProdex04383Transport*` mutation suite was also executed under the race
detector in every phase. The original unsliced `--websocket-transport` mode,
the default release gate and native portable fixture retain **all 20 cases**;
none of their assertions or duration windows was reduced.

The unsliced fixture exceeded the synchronous connector execution window,
and an initial two-part run likewise exceeded it after both binaries reached
the 30-second deadline and the final capacity case. These **partial runs were
not claimed as passes**; the three completed clean-commit runs above supply
the new finite oracle proof. `make verify` also passed locally on the exact
source modifications before the commit, including full race tests, vet,
installer checks, cross-compilation and production build. This establishes
finite Linux/synthetic WebSocket transport parity, **not** FULL production
1:1: the independent malformed-direct-field network oracle, strict public
Compact-followup lineage, native macOS realtime and untested OS/OAuth/provider
failure permutations remain PARTIAL/OPEN.

## Pinned 0.438.3 regression gates after the 20-case split

Following the clean-commit transport qualification at `cf8aafc`, the
unchanged production candidate was requalified on clean documentation
revision `e19c04c708befb7eb0a80352eff2e3e567fbbaa4` through two
additional official 0.438.3 source/binary/manifest-pinned phases:

| Release phase | Result on exact `e19c04c` |
| --- | --- |
| `--phase=websocket-core` | PASS; historical 56-attempt response suite, idle capacity, committed idle close, 8KiB promotion and native Linux realtime lifecycle on both executables |
| `--phase=preflight-tests` | PASS; scoped regressions, mutation sentinels and provider/CLI test prerequisites |

The transport's three-part 17+2+1 differential and its source-pinned mutation
suite **already passed** on the common clean `cf8aafc` candidate. Neither a
partial/timeout invocation nor compilation for a different operating system
has been counted as an executable pass. The complete default release gate
across *every* provider and native/macOS/Windows runtime mode has not been
rerun on `e19c04c`, so the qualified result is limited to the recorded finite
synthetic phases. Full live OAuth, OS keyring and external-provider fault
parity, native macOS realtime, strict public Compact-followup WebSocket
lineage and the malformed direct WebSocket metadata network oracle remain
PARTIAL/OPEN.

## WebSocket selects a verified Compact turn lineage across quota routes

The tagged 0.438.3 source (`runtime_proxy/affinity.rs`,
`runtime_proxy/continuation.rs` and `runtime_state/lineage.rs`) resolves the
distinct `__compact_turn_state__:` lineage owner even when the inbound request
is routed through a **WebSocket** socket. That must not be confused with the
HTTP Compact quota route, nor may a missing lineage arbitrarily select second.
The upstream WebSocket handshake also carries the resolved body
`x-codex-turn-state` token whether or not the turn has a bound owner.

A new two-binary differential fixture seeds a verified synthetic Compact
turn lineage on **second**, then warms second *before* main. The order is a
strict control: main is the active profile immediately before the test and
both quota caches have applied authenticated Ready receipts. On exact clean
exploratory `b991e86`, official Prodex chose second for the lineage case and
main for the unbound control. Godex incorrectly chose main for the bound case
and omitted the upstream turn-state header. Both products received the same
opaque full history. Earlier exploratory attempts failed their own fixture
constraints (missing official handshake turn override and last-active second)
and are not counted as behavioral mismatches.

The scoped fix consults Compact's separate turn alias only when the direct
WebSocket turn owner is absent and no previous-response dependency is present.
It never turns the WebSocket quota route into Compact. It also copies the
resolved, bounded body turn-state into a cloned upstream handshake header,
without modifying the inbound request. Four memory/durable positive/negative
routing tests fail before the fix; the entire routing suite, repeated focused
race checks and `make verify` pass after the fix.

**Independent executable oracle PASS** on exact clean exploratory candidate
`caedf329fdc5c7536c56af98087083d2bd1fb306`:

- Lineage present: Prodex and Godex each dispatch once to second, retain the
  authenticated Ready evidence, exact input, `compact-turn` header and terminal
  frames, and persist the new response on second without moving the alias.
- Lineage absent: both dispatch once to active main, keep the same exact
  header/body/client frames, leave the Compact alias absent and retain the
  unrelated owner.
- The source/tag, release manifest, official executable digest and clean
  candidate binary commit are pinned. Oracle mutation tests reject wrong
  owner, stale header, missing Ready evidence, input changes, duplication,
  changed client frames and missing terminal output.

The native parity matrix now includes this finite scenario. Native execution
on macOS/Windows and production creation of every Compact lineage alias are
still unqualified independently; this bounded pass is **not** FULL 1:1 parity.

## HTTP Compact producer persists both typed lineage aliases and direct session

The preceding WebSocket oracle originally **seeded** an existing Compact turn
alias. It could not establish that Godex creates that alias from a successful
HTTP Compact response. Tagged Prodex 0.438.3
(`runtime_proxy/standard/attempts/success.rs` and
`runtime_proxy/lineage/remember/compact.rs`) calls
`remember_runtime_compact_lineage` after a committed Compact response, using
the request's session ID and the upstream `x-codex-turn-state` response header.
It also preserves the direct session identity even if a previous-response ID
had hard-pinned the Compact request's route to a known owner.

Godex's general response observer initially bound the raw turn but **neither**
Compact alias. A fail-before regression reproduced missing Compact session
and turn aliases in memory and durable stores. A second positive control with
a verified previous response then showed a missing **direct session** binding,
despite the already-correct previous-response owner. The scoped production
repair records both typed aliases only for successful HTTP Compact responses,
using the existing `rememberVerifiedAccountBinding` boundary. When the hard
previous response excluded session identity during selection, successful
Compact commitment additionally records that direct session. Ordinary
Responses do not create Compact aliases, failed requests do not acquire
lineage, and no profile authentication or state schema is changed.

The independent two-executable `websocket-compact-lineage` release phase now
includes `compact-produced` in addition to the two seeded/unbound controls.
It sends a **real** HTTP Compact request whose upstream return contains a
synthetic turn-state header. An already-verified previous-response ID pins the
producer to second on **both** products; the first fresh-owner experiment
showed different ordinary rotation choices and could not isolate the producer.
After the HTTP commit, the harness waits for **all three** durable projections
(turn alias, Compact session alias, and direct session) before returning main
to active status and sending the subsequent WebSocket request. The exact
response, upstream attempt count, history bytes, frame sequence and final
owners are compared and mutation sentinels remain enabled.

**Source/binary/manifest-pinned differential PASS** on clean exploratory
`35c2c3b3fefc25f99a218ab71b1b17c78b74bfdb`: Prodex and Godex each
passed the seeded-lineage, unbound-control, and real Compact-produced
HTTP-to-WebSocket scenarios. The result qualifies this bounded synthetic
producer/consumer contract, not every live external credential or native
platform. Complete release/native matrix and macOS realtime parity retain
their separate verification requirements.

## Independent public WebSocket oracle for shadowed direct turn metadata

The source-pinned Prodex 0.438.3 semantic plan selects a top-level JSON
member by **presence**, then tries the nested `client_metadata` member only
when the top-level member is absent. A top-level `x-codex-turn-state` with
`null` or whitespace does not inherit a nested value that names another
owner. Earlier Godex parser and routing regressions proved this behavior only
in-process; no public upgraded WebSocket had confirmed the full policy.

The mandatory `websocket-compact-lineage` executable mode now includes two
additional authenticated two-profile controls: `compact-shadow-null` and
`compact-shadow-blank`. Each fixture starts with a verified Compact turn alias
on second, applies both Ready quota receipts, and warms main last as the active
profile. The exact unmodified full-history message supplies a *present but
invalid* direct turn and a nested `compact-turn` token. Both products must
make exactly one main-owned upstream attempt, send **no** turn-state handshake
header, forward the exact original model payload and terminal frames, and
preserve second's pre-existing alias and all unrelated durable ownership.
Mutation controls independently reject rotation to second, nested token
leakage, missing Ready evidence, input changes, extra attempts and terminal
identity changes.

**VERIFIED** on exact clean `606f3cd6a0cf51c165ce67a83f009740124edc10`:
`./scripts/verify-release-parity.sh --phase=websocket-compact-lineage`
exited **PASS** across five scenarios (the two original seeded/unbound
controls, two malformed-direct controls and the real HTTP Compact producer)
for both release-pinned executables. `make verify` also passed on the same
candidate, including the complete Go race suite and cross-builds. The
remaining malformed-direct `session_id` variants, native macOS realtime and
non-synthetic integrations are **not** claimed VERIFIED by this turn-state
oracle.

## WebSocket resolves verified Compact session lineage across explicit and body sessions

In the tagged Prodex 0.438.3 WebSocket affinity plan, an existing Compact
session lineage (`__compact_session__:`) is eligible to select its original
profile even when no direct session binding exists and the quota route remains
**WebSocket**. The already-verified Compact turn lineage handles a different
continuation surface. Godex previously considered only the raw `session_id`
binding and incorrectly selected current main when the Compact alias belonged
to second.

An independent source/binary-pinned differential added three controlled cases
with both profiles authenticated Ready and main restored as the current profile:

- `compact-session-header`: the client provides `session_id: session-alias` on
  the WebSocket upgrade, with only the typed Compact alias owned by second
- `compact-session-unbound`: the same explicit header without the alias, so
  current main must handle the request
- `compact-session-body`: the typed alias belongs to second and the session ID
  appears only in the JSON message; the official 0.438.3 executable also
  selects second here, without an explicit upgrade header

The official executable independently established all three expected owner
choices before any Godex implementation change. The clean candidate with no
Compact session resolver dispatched `compact-session-header` on main; the
independent oracle rejects this wrong owner. The scoped routing fix consults
the typed alias only for WebSocket session-only messages after checking that a
direct session owner is unbound. Verified previous-response/turn identities
continue to outrank session affinity, and the alias never changes the
WebSocket quota route. Memory and persisted routing regressions failed before
this fix for both header and body aliases and passed after it; direct-session,
previous-response, unbound and unchanged-opaque-input controls remain active.

The synthetic differential establishes only the bounded session owner policy;
it does not establish full live OAuth, macOS realtime or all possible native
Compact producer shapes. Record an executable pass only on a clean committed
candidate with exact official source/binary/manifest provenance.

## Conflicting direct WebSocket turn and Compact lineage must never dispatch

The tagged Prodex 0.438.3 hard-binding resolver inspects both a direct
turn-state owner and its `__compact_turn_state__:` lineage owner. If both
verified bindings point at different profiles, the result is a **hard
binding conflict**, not precedence for either owner. On a WebSocket request
this must fail closed before any upstream connection or model invocation.

A new source/binary-pinned differential case seeds the direct `compact-turn`
on main and its typed Compact lineage on second. Both profiles retain
applied authenticated Ready evidence; main is current immediately before
the request. Official Prodex returns the exact single
`503 service_unavailable` client frame with the precommit recovery message,
**zero** upstream WebSocket handshakes and model requests, and unchanged
durable owners. The pre-fix Godex executable instead dispatched the exact
opaque history to main, proving an observable cross-profile ownership bug.

The scoped routing change considers both verified owners before selecting a
WebSocket turn owner. Different registered owners return the existing
`503 service_unavailable` precommit terminal error at the usecase boundary;
matched owners still select that same account, and an unbound direct turn may
continue using its verified Compact alias. Neither the WebSocket quota route
nor any stored keys/credential records are rewritten. Memory and durable
regressions failed before the patch and passed after it, with controls for
matching owners, preserving both conflicting bindings and zero model dispatch.
The executable fixture independently rejects any extra dispatch, changed
503 frame, altered owner or missing terminal. This finite conflict policy is
not a complete certification of provider identity/keyring semantics.

## Direct WebSocket session owner outranks Compact session alias

The pinned 0.438.3 executable now has an additional positive/negative
session-precedence oracle. It seeds a verified direct `session-alias` binding
on main and a verified `__compact_session__:session-alias` lineage binding on
second. With both profiles authenticated Ready and main active, the original
request with an explicit `session_id` header must select main, forward exactly
one opaque input, and preserve both original owners. This is distinct from the
already-verified alias-only case (second) and unbound case (main). The Router's
memory and durable owner-precedence regression remains active.

The official executable additionally confirmed that **a successful WebSocket
session-only message persists a direct session binding** on the selected
profile. An earlier exploratory oracle asserted that the direct binding must
remain absent after a successful alias-selected turn. That assertion could
fail solely because Prodex's asynchronous persistence completed before the
snapshot check. The fixture now requires the correct direct session owner
(`main` or `second`, according to the exact case) in its post-commit durable
projection, while continuing to verify the unchanged Compact alias and
unrelated hard owners. This strengthens, rather than relaxes, the oracle.

The full source/binary/manifest-pinned `websocket-compact-lineage` phase passed
all existing cases plus the direct-session conflict and post-commit durable
controls on clean exploratory candidate `51504a0e173c85d4c72df6ec252288795259af68`.
This finite session precedence result does not independently qualify live
provider login, keyrings or native macOS realtime behavior.

## Previous-response owner versus Compact turn lineage on WebSocket

The tagged Prodex 0.438.3 hard-binding resolver considers a verified
`previous_response_id` owner **together with** verified Compact-origin turn
lineage (`__compact_turn_state__:`), even though the request's quota route is
WebSocket. A previous response on main and a Compact lineage on second are
conflicting hard evidence: neither profile is permitted to infer on that
request.

A clean source/binary/manifest-pinned differential on pre-fix exploratory
`92c3401` produced the exact same authenticated Ready profiles and synthetic
full-history request on both executables. Official Prodex emitted one canonical
`503 service_unavailable` WebSocket error frame with **zero upstream model
attempts**. Godex instead sent the original request to main and forwarded
model output. This is a genuine cross-profile isolation discrepancy, not a
fixture failure.

The scoped usecase fix checks the previous-response binding and the matching
Compact turn alias before choosing an owner. Only when **both** are verified
and distinct does it return the tagged 503 recovery-exhausted error before
any model dispatch. A missing alias retains the existing previous-response
policy, while matching previous and Compact owners remain usable. New memory
and durable regressions failed before the patch, then passed, including exact
local status/message, zero upstream attempts, immutable original owners and a
same-owner control. Full native and live-provider parity remain separately
unverified; a finite differential PASS must be recorded on the clean final
candidate, not inferred from those local tests alone.

## WebSocket hard previous response commits direct session ownership

A pinned Prodex 0.438.3 differential now covers a WebSocket request with a
verified `previous_response_id` on main, a `session_id` in the body, and a
verified `__compact_session__:` lineage belonging to second. The tagged
executable selects main (hard previous-response affinity), sends one full
request, and commits the new response and direct session binding on main. It
retains the typed Compact session alias on second. This direct session
persistence is observable in the official continuation journal after the
response commits, independent of the account-routing outcome.

The pre-fix exploratory candidate `fa3eb98` routed the request to main and
emitted correct client frames, but **did not persist its direct session owner**.
It therefore failed the exact durable owner projection on the Godex side of
the independent oracle, while Prodex passed. Memory and persisted usecase
regressions reproduced the missing direct session owner before the patch.
The scoped fix preserves the request's original previous-response affinity
before recording its new WebSocket response ID. After a genuinely committed
WebSocket turn, it records a valid direct session observation on the
successful owner even though hard previous-response affinity excluded that
session from candidate selection. A previously unbound direct session becomes
owned by the successful profile. If it was already verified on another
profile, the durable owner becomes the tagged hard-binding conflict marker,
not a silent takeover or a stale single-owner binding. The separately verified
Compact session alias remains untouched; HTTP response and Compact-lineage
creation paths remain unchanged. Full routing
regressions and repeated focused race checks pass after the patch.

The native/source-pinned differential must be rerun on the final clean commit
before calling this finite HTTP-to-WebSocket direct-session persistence gate
VERIFIED. It does not prove external-provider OAuth or native macOS/Windows
runtime equivalence.

## Hard previous-response commit marks an already-foreign direct session conflicted

A further pinned 0.438.3 differential covers a WebSocket request where
`previous_response_id: resp-main` is verified on main, while its ordinary
`session_id: shadow-session` is already verified on second. Both products
select main from the hard previous-response constraint, perform one upstream
request and emit the same client frames. However, the official Prodex journal
**changes the direct session binding to**
`__prodex_hard_binding_conflict__` after the successful turn, while preserving
the unrelated previous-response bindings. The exploratory pre-fix Godex
candidate `0b25f90` left the stale single-owner `shadow-session` binding on
second. A different previous-response+unbound-session case was already fixed;
this foreign-owner transition needs the verified merge conflict policy, not a
conditional skip.

New memory and durable regressions reproduce that mismatch before the
correction and check the *exact* conflict sentinel after a successful
WebSocket turn. The scoped production change always calls the existing
`rememberVerifiedAccountBinding` on a valid committed session when hard
previous-response selection omitted it. `MergeVerified` already converts a
second, disagreeing verified profile observation into a durable conflict
sentinel; no state schema change or forced owner reassignment is needed.
The test also requires the original previous owner, newly committed response
owner and separate Compact session lineage to remain unchanged. The pinned
oracle's synthetic state reader now recognizes the official conflict sentinel
as an intentional ownership outcome and checks it for the
`session-hard-previous` case, while continuing to require a normal owner in
all other session-only controls. This is a finite synthetic continuation
safety proof, not a claim about all live credentials or native OS runtimes.

## Conflicted WebSocket session remains a soft preference, not hard error

A direct session made conflicting by two verified owner observations is
**not** a hard previous-response/turn-state continuity key. Tagged Prodex
0.438.3 discards that session as an owner preference and chooses an eligible
fresh profile for the next session-only WebSocket message. The official
`session-hard-previous` differential first commits a main-owned response
against an existing second-owned `shadow-session`, confirms the durable
`__prodex_hard_binding_conflict__` marker, and sends a second session-only
message on the **same client WebSocket**. Official Prodex dispatches the second
message exactly once on an eligible Ready account, returns the complete
response delta/completed frame pair, and retains the conflict marker. Pre-fix Godex returned a local
`409 stale_continuation` before any second upstream attempt.

The routing fix gives the affinity store's deliberate conflict an identifiable
internal error value (without changing its external wording), then treats
that error as **unbound** only while evaluating a WebSocket session-only
*soft* preference. Any separately verified Compact session alias is still
eligible; an unrelated I/O/storage error remains fatal. Hard previous/turn
conflicts remain fail-closed. Session IDs remain available for verified
post-commit observations, and `MergeVerified` keeps the durable conflict
marker. Memory/durable regressions failed before and passed after this fix.

An inherited public test named `TestProdex04379PublicWebSocketHardPreviousOutranksConflictingSession`
previously expected the old soft session owner to remain routable after a
hard-owned turn. The tagged 0.438.3 executable instead persists a conflict
marker and selects one eligible fresh Ready owner (main or second). That
regression now checks a valid response, exactly one eligible selected account
and the durable conflict marker.
The independent followup oracle rejects the original 409, extra inference,
unknown/unready accounts, malformed frames and altered input as mutations;
selection of either authenticated Ready account is allowed by the tagged
reference and must not be mistaken for a deterministic preference.

The finalized pinned two-executable phase and complete local verification
must pass on the clean candidate before this expanded finite behavior is
recorded VERIFIED. Native macOS/Windows and live provider credentials remain
separate unqualified surfaces.

## Complete 0.438.3 pinned regression checkpoint after soft-session conflict fix

The full code-bearing checkpoint
`67e995a25224869d47678a5db29a4102800e9bb3` was tested from a **clean
committed** Godex checkout against the official tagged Prodex 0.438.3 source,
release manifest and Linux executable with independently verified provenance.
The following **separate** two-binary phases all exited **PASS** on that exact
Godex SHA:

| Release parity phase | Exact result |
| --- | --- |
| `--phase=websocket-compact-lineage` | PASS: verified Compact turn/session provenance, mixed hard-owner conflict, HTTP Compact production, full-history handoff, conflicting soft-session followup and immutable durable bindings |
| `--phase=websocket-core` | PASS: response errors/retries, upstream connection reuse, precommit lookahead, committed idle and realtime duplex |
| `--phase=websocket-transport-core` | PASS: all 17 authenticated precommit transport, quota, ownership and postcommit no-replay controls |
| `--phase=websocket-transport-lifecycle` | PASS: both cancellation and the original 30-second deadline controls |
| `--phase=websocket-transport-capacity` | PASS: saturated Ready alternate with no cross-account replay and correct durable snapshot |
| `--phase=preflight-tests` | PASS: pinned synthetic regression and mutation-test prerequisites |
| `--phase=http-compact` | PASS: HTTP Compact retry, stale continuation, full-history fallback and durable ownership comparison |

`make verify` also **PASS** on the same source modifications before the
squashed code commit, including shuffled Go race tests, formatting, vet,
installer smoke checks, native build and Linux/macOS/Windows cross-compilation.
The finite WebSocket transport gate thus has a new common-SHA **20/20 PASS**
(17 core + 2 lifecycle + 1 capacity) after the soft-session conflict fix.
No phase was accepted based solely on a partial invocation or an earlier
candidate's SHA.

**Qualification boundary:** the entire default release matrix's other pinned
phases were not rerun together on this SHA, and native *execution* on macOS or
Windows, provider OAuth/keyring and live upstream fault combinations remain
PARTIAL/OPEN. Cross-compilation is not a substitute for native runtime
observations. The results above are finite source-aligned synthetic parity,
not a blanket production 1:1 release certificate.

## Expanded pinned multi-domain 0.438.3 gate on clean `799081d`

The exact clean documentation checkpoint
`799081d16ea7691de95b256d343769c9907c9580` was independently run
against the source-tag/binary/manifest-pinned official Prodex 0.438.3 Linux
executable. All of the following **additional** release-parity phases exited
**PASS** on this exact commit; none is inferred from a neighboring SHA:

| Phase | Observed scope |
| --- | --- |
| `--phase=preflight-oracles-pty` | PASS: CLI/PTY and workspace trust, including deduplicated trusted workspaces and negative controls |
| `--phase=preflight-oracles-profiles` | PASS: profile lifecycle, quota/auth startup and restart snapshots |
| `--phase=preflight-oracles-history` | PASS: durable history/continuation fixture and negative controls |
| `--phase=deepseek` | PASS: pinned DeepSeek response and CLI parity fixtures |
| `--phase=gemini` | PASS: Gemini native/chat SSE, tools, token usage, route/auth/error and negative mutation controls |
| `--phase=http-keepalive` | PASS: fresh absolute request budget on one reused TCP connection, two 200 responses after ~31 seconds on both binaries |
| `--phase=http-turn-transport` | PASS: owned HTTP Responses turn/session/previous-response transport errors and unchanged durable owners |
| `--phase=websocket-deadline` | PASS: absolute ~30-second precommit deadline, 504 `request_timeout`, followed by a successful new turn on the same client socket |

The same code candidate `67e995a25224869d47678a5db29a4102800e9bb3`
preceding this documentation commit was separately verified for all of
`websocket-compact-lineage`, `websocket-core`, 17-case
`websocket-transport-core`, 2-case `websocket-transport-lifecycle`, 1-case
`websocket-transport-capacity`, `preflight-tests` and `http-compact`.
Only docs changed between those two SHAs. Collectively this is a broad set
of **finite synthetic Linux differential results**, not a release-wide claim
of perfect production 1:1. Native macOS/Windows execution, live OAuth/keyring
and external-provider fault combinations retain **PARTIAL/OPEN** status.

## HTTP Responses conflict marker is a soft session preference (0.438.3)

The tagged 0.438.3 affinity planner explicitly filters the
`__prodex_hard_binding_conflict__` sentinel when deriving a **soft** session
preference for ordinary HTTP Responses and WebSocket. In contrast, verified
previous-response/turn hard conflicts retain their fail-closed guarantees.
Godex's WebSocket path already handled this distinction, but ordinary HTTP
Responses still passed the conflict marker to the general hard-affinity
resolver and returned local `409` before reaching an upstream.

An independent **authenticated two-executable** differential uses a real
managed launch with main and second, applied Ready quota receipts, and a
synthetically persisted `http-conflict` direct session marker on both products.
It then sends the same self-contained HTTP `/responses` body and compares
route, response status/body, upstream identity, exact opaque input, attempt
count and the durable conflict/response bindings. On the clean exploratory
`972ddec` fixture the official Prodex 0.438.3 binary selected a Ready profile,
returned HTTP **200** with one upstream request and retained the original
conflict marker. Pre-fix Godex instead returned HTTP **409** with zero
upstream requests, proving the discrepancy independently of Rust source.

The scoped `resolveOwner` fix excludes the deliberate verified conflict only
while selecting a *session-only* HTTP Responses request. It keeps the original
session value available for post-commit verified observations and leaves
ordinary owned sessions, previous-response/turn hard affinity and HTTP Compact
selection rules unchanged. Dedicated fail-before memory/durable regressions
cover both the conflict fallback and the positive control where a healthy
existing session still routes to its original account. The fixture's mutation
controls reject a local 409, missing/duplicate/unready/unauthenticated model
attempts, changed history, wrong owner class or altered response.

The `--phase=http-session-conflict` release gate is pinned to the exact official
0.438.3 executable digest, manifest and source tag. The native parity matrix
includes the same case, but native macOS/Windows **execution** and live
OAuth/provider fault parity must still be qualified independently. Do not
mark this phase VERIFIED until the complete differential passes on the clean
final code commit; a source-aligned local unit test alone is insufficient.

### Single-turn eligibility versus durable session repair

The inherited `TestProdex043711FullHistoryRepairsOnlyConflictingSoftSession`
used to treat a single-turn conflicted Responses request as a terminal routing
error. The official **0.438.3 binary** instead accepts a self-contained
single-turn request on a fresh eligible profile, while its durable session
remains **conflicted**. This is distinct from releasing and rebinding the
conflict marker, which is allowed only for a proved reconstructable complete
history. The historical regression now independently asserts both dimensions:
full-history requests may repair durable ownership, while a single-turn
request may return 200 without clearing the conflict. Opaque previous-response
or turn-state dependencies and Compact routes retain their no-reset controls.
This adjustment is based on the pinned executable's actual HTTP response and
persisted owner, not on a broadening of the lineage repair policy.

## HTTP Responses hard-previous commit creates the direct session (0.438.3)

A successful HTTP `/responses` request can be hard-pinned to an existing
`previous_response_id` while also carrying a direct `session_id`. The tagged
Prodex 0.438.3 standard success path calls `remember_runtime_session_id`
after selecting the hard previous owner, independently of whether the session
was eligible for candidate selection. Godex used `affinityKeysForRoute` to
exclude the session during hard previous selection **and during success
observation**, so no durable direct session was recorded.

The source/binary/manifest-pinned exploratory `http-session-conflict` phase
on clean pre-fix `c9c2093` now contains two **additional real HTTP requests**
against authenticated Ready profiles:

- `http-previous-session-new`: previous response verified on main, session
  previously absent. Official Prodex selects main, returns 200 with exactly
  one upstream request and binds the new direct session to main.
- `http-previous-session-foreign`: the same hard previous response, but the
  direct session is initially verified on second. Official Prodex still
  selects main, returns 200 with exactly one upstream request, and persists
  `__prodex_hard_binding_conflict__` instead of stealing the session owner.

Pre-fix Godex kept the new direct session **unbound** after the first case;
the Router memory/durable regression reproduced missing new ownership and
missing conflict marking for both cases. The scoped fix records a verified
session observation on the selected successful HTTP Responses owner only
when hard `previous_response_id` selection omitted that session from the
outbound candidate keys. The existing verified merge policy preserves
foreign ownership as a conflict marker. No new persistence format, auth
behavior or compact route handling is introduced. The original hard
previous-response owner and new response ID must remain attached to main.

An inherited previous-response/soft-session test still expected a foreign
session's original owner after successful HTTP hard affinity; it now checks
the 0.438.3 conflict marker and unchanged previous-response ownership.
Focused memory/durable regressions, repeated race runs and complete routing
tests pass after this change. The expanded pinned differential additionally
checks exact upstream request JSON, authentication, Ready evidence, HTTP
response and durable owners, and rejects mutation sentinels for wrong owner,
missing/duplicated requests or stale fields. Record a release-phase PASS only
when both executables pass on the final **clean committed** candidate.

Native execution on macOS/Windows and live provider/keyring faults remain
PARTIAL/OPEN despite cross-compilation and the finite synthetic Linux gate.

## HTTP Responses hard turn-state commits direct session ownership (0.438.3)

The tagged standard Responses success path remembers request session IDs
independently from the hard owner used for candidate selection. The preceding
`http-previous-session` fixture covered `previous_response_id` but not an
otherwise identical request pinned by `x-codex-turn-state`. Both continuation
shapes intentionally exclude session IDs from candidate-owner selection to
avoid crossing upstream histories. That exclusion must **not** suppress
post-success verification of the request's direct session owner.

A source/binary/manifest-pinned exploratory extension of the
`--phase=http-session-conflict` gate ran two independent hard-turn fixtures on
clean pre-fix `8e2d1be`: each had two applied authenticated Ready accounts, a
verified `turn-a` on main, exact original JSON with the top-level token, and
an upstream HTTP `x-codex-turn-state` header. Official Prodex returned HTTP
200 on main with **one** request and correctly persisted both the new response
and the session. For a previously **absent** session, the session became
main-owned; for a session previously verified on **second**, the direct
session became `__prodex_hard_binding_conflict__` while preserving the original
turn owner. Pre-fix Godex sent the right request to main and returned 200 but
left the previously absent direct session unbound, establishing a durable
parity mismatch.

The minimal routing correction extends the already-source-qualified HTTP
hard-previous success observation to the **hard turn-state** shape, only when
the ordinary HTTP Responses route excluded an otherwise valid request session
from its candidate keys. It reuses the verified affinity merger, which
preserves existing other-account ownership as a conflict marker. Memory and
durable regressions failed before the fix for both absent/foreign sessions and
passed after it; focused race and full routing controls are retained.

The pinned oracle now checks five scenarios in one bounded HTTP gate:
soft-session conflict, hard-previous new/foreign session, and hard-turn
new/foreign session, each with one authenticated Ready upstream request,
exact untouched JSON/header, HTTP response and durable owner projection.
Mutation sentinels also reject stale/missing turn headers, modified bodies,
wrong provider, missing quota readiness and replay. Native macOS/Windows and
real OAuth/keyring/provider fault permutations remain separately OPEN/PARTIAL.

## HTTP direct `session_id` shadows nested metadata on malformed values

The tagged 0.438.3 `runtime_request_semantic_plan` chooses a direct JSON
`session_id` **by presence** before consulting `client_metadata.session_id`.
A present `null`, blank or otherwise non-string direct field yields no session
owner; it must not silently borrow a different account's nested identity.
Earlier 0.438.3 WebSocket tests covered that rule, but Godex's shared affinity
parser applied the direct-field presence check only to WebSocket messages and
continued to use nested session metadata for ordinary HTTP Responses.

A source/binary/manifest-pinned independent HTTP differential on the clean
pre-fix exploratory candidate `07ea56e` seeded a verified `shadow-session`
on second, warmed second before main and verified both authenticated Ready
quota receipts. Official Prodex dispatched the original opaque HTTP request
exactly once to **main** when the direct session was `null` or whitespace,
returned 200, kept the second-owned session intact and persisted its response
on main. With the direct field absent, the same Prodex binary selected second
from `client_metadata`, proving this is a shadowing rule rather than a ban on
nested sessions. Pre-fix Godex incorrectly dispatched the `null` case to
second. Memory/durable fail-before regressions independently reproduced the
wrong owner for six malformed shapes, including blank, `null`, boolean,
number and object values.

The scoped Go parser correction honors the direct-member presence condition
for HTTP and WebSocket alike while leaving explicit session headers, missing
direct-field fallback and opaque request-body bytes unchanged. The new
`--phase=http-session-conflict` differential includes the three validated
HTTP direct-field cases (`null`, blank, absent) and mutation controls for
owner, quota readiness, upstream count, header, input and response integrity.
Full parity for the remaining provider/OS/keyring scenarios is still OPEN; a
new phase is VERIFIED only after an exact clean code commit passes the binary
oracle and the suite's negative mutation checks.

## HTTP Responses ignores nested previous-response identity

The official Prodex 0.438.3 semantic plan reads the request's
`previous_response_id` from the **direct** JSON member only, not from a
nested `response.previous_response_id` object. The latter is request payload
and must never silently pin the request to a different account's verified
response ID. Godex's generic affinity extractor previously adopted that
nested member whenever the direct value was absent, `null` or blank,
converting unrelated payload into hard routing ownership.

The independent 0.438.3 source/binary/manifest-pinned differential on a clean
pre-fix exploratory candidate `d5deed0` seeded a verified `resp-nested`
previous response on second, warmed second before main, and proved applied
authenticated Ready receipts on both profiles. Four otherwise identical HTTP
Responses requests carried a nested `response.previous_response_id`:

- Direct previous-response **missing**, `null`, or blank: official Prodex
  selected current main, dispatched once and preserved the second-owned
  original response binding.
- Direct previous-response **valid** and equal to the verified nested ID:
  official Prodex selected second, its actual hard owner.

The exact raw input body, one upstream attempt, HTTP 200 reply and durable
new/old response owners are compared. Pre-fix Godex incorrectly selected
second even with the direct field missing. Memory/durable fail-before tests
reproduced all three invalid/absent direct cases, with a valid-direct
positive control. A scoped HTTP request-affinity correction disallows the
nested hard previous-owner fallback for ordinary HTTP Responses. The
existing legacy WebSocket envelope remains isolated under its previously
tested WebSocket request boundary; the HTTP fix does not rewrite or remove
client JSON. Mutation controls reject wrong owners, replay, invalid quota,
wrong headers, response changes and altered nested/content history.

This is a bounded synthetic HTTP hard-affinity contract, not proof of native
macOS/Windows runtime or external OAuth/keyring/provider fault parity. Report
PASS only after the exact final clean Godex commit passes the official two-
executable oracle and local regression suite.

## HTTP turn-state uses headers, not request JSON (0.438.3)

The tagged `runtime_request_turn_state` reads the `x-codex-turn-state` HTTP
**header**. The separate JSON semantic planner is used for WebSocket request
messages; applying its direct-member precedence to HTTP was an incomplete
source inference. The exact official 0.438.3 Linux binary exposed a larger
pre-existing Godex mismatch: HTTP Responses with an absent direct turn-state
member still ignored a *verified* nested `client_metadata` turn and selected
active main, while Godex incorrectly pinned the request to second.

Godex now limits JSON turn-state extraction to WebSocket messages. The HTTP
header stays authoritative for Responses and Compact; no request body bytes
or previous-response/session metadata are rewritten. Memory and durable
Router regressions cover six malformed direct values, missing-direct JSON
and a positive verified header-affinity control. The source, manifest and
executable-pinned `--phase=http-session-conflict` oracle now independently
runs `null`, blank, absent, and header-present cases with two authenticated
Ready profiles. It checks the exact request body, preserved header, single
upstream dispatch, HTTP 200 and both durable response and turn ownership.
A negative oracle suite rejects changed owners, forged readiness, dropped
or duplicated model attempts and modified wire evidence. The initial paired
run caught the missing-direct discrepancy before this correction; a complete
clean-commit rerun is required before marking the phase VERIFIED. Native
Windows/macOS and non-synthetic provider behavior remain separately open.

## Do not promote `conversation_id` or `thread_id` into request session ownership

The official 0.438.3 `runtime_request_semantic_plan` resolves request session
identity from the exact direct JSON `session_id`, or the exact
`client_metadata.session_id` member when direct is **absent**. Unlike a legacy
response-output parser, it does not interpret `conversation_id` or `thread_id`
as alternate session keys. Godex did, so an otherwise independent HTTP
Responses request mentioning a second-owned session in either unrelated field
could be hard-routed to the wrong account. A fail-before memory/durable
regression demonstrates the foreign selection for both aliases separately and
together; canonical direct and nested session identifiers remain positive
controls. WebSocket parsing applies the same canonical input shape.

The request-affinity parser now uses only the exact nested session key when
no direct member exists. Raw JSON and session headers are not changed. The
pinned `--phase=http-session-conflict` differential adds the alias-only
cases beside the original direct-shadow cases, independently checking two
Ready authenticated profiles, exactly one upstream request, original opaque
bytes, HTTP status and the unchanged verified session owner. This is a
bounded synthetic parity slice, not a claim about all provider or OS paths.

## Turn-metadata header parses nested `client_metadata.session_id`

The pinned 0.438.3 `runtime_request_session_id_from_turn_metadata` calls the
**same** presence-sensitive semantic planner as a request body. Its parsed
`x-codex-turn-metadata` JSON header may contain a nested
`client_metadata.session_id` even without a direct `session_id`; a present
invalid/blank direct member must shadow that nested field. If the whole
header yields no session ID, request body session resolution remains available.
Godex previously read only direct `session_id` from the header, overlooking a
verified nested session owner and silently routing an otherwise independent
HTTP request on the current profile. A fail-before memory/durable regression
reproduces that wrong-account selection. The shared existing
`semanticNestedText` helper now resolves header session identity consistently
without changing opaque forwarded headers, body bytes or other route keys.

The official two-executable HTTP session-conflict gate now also covers a
nested-header positive, direct-header positive, null/blank shadow controls and
body fallback after a null header. Its synthetic fixtures require authenticated
Ready evidence on both profiles, a single upstream inference, the exact
request body and header, HTTP 200 and intact old/new durable ownership.
Header-mutation tests ensure an erased or rewritten metadata header cannot
pass unnoticed. The full non-synthetic 1:1 parity conclusion remains OPEN.

## Unsupported `x-codex-thread-id` HTTP header must not pin an owner

The tagged `runtime_request_thread_id` accepts the exact `thread-id`
header (for request tracing) or `thread_id` in the JSON
`x-codex-turn-metadata` header; it does **not** accept
`x-codex-thread-id`. Godex previously treated the unsupported header as a
hard routing key. A synthetic three-request regression demonstrates how a
successfully routed second-owned previous response could create a spurious
`thread` binding; after an independently pinned main response restored the
current profile, reuse of that unsupported header would wrongly reroute an
independent HTTP request back to second. The memory and durable regressions
fail before the fix.

Godex now excludes only the unsupported request-header spelling from the
routing parser, leaving valid `thread-id`, request body bytes and upstream
headers intact. An official source/binary/manifest-pinned three-request
fixture builds its thread context naturally from a previous-response-owned
request on second, restores main, and checks the subsequent independent
`x-codex-thread-id` request stays on main. Its negative oracle rejects
owner diversion, missing/duplicated upstream calls, absent Ready receipts,
changed request/header bytes, and modified response or authentication.
Remaining thread-ID logging and cross-OS/provider behavior are not thereby
certified as complete parity.

## Canonical HTTP `thread-id` is trace metadata, not a hard request owner

The exact tagged `runtime_request_thread_id` resolves the supported `thread-id`
header (or the canonical `thread_id` value inside a turn-metadata header) for
continuation tracing. A trace label alone does **not** grant routing ownership:
Prodex's Responses candidate selection is governed by previous-response,
turn-state and session evidence. Godex previously promoted the supported
`thread-id` header to hard affinity. A three-request source-derived fixture
reproduces the error: a previous-response-pinned second-owner request bearing
`thread-id` creates a misleading owner; a separate valid previous-response
request restores main; the next independent request with the same trace is
incorrectly routed to second. The identical unsupported `x-codex-thread-id`
control is retained.

The Godex input parser now excludes thread trace labels from Responses,
Compact and WebSocket owner selection, including read-only admission probes,
while retaining existing generic/standard transport behavior and passing
through the original header for upstream trace attribution. Memory and durable
regressions cover both header spellings; the official executable oracle checks
the natural second/main/main HTTP sequence independently for each spelling,
two authenticated Ready profiles, original wire and three durable response
owners. Native OS and real external-provider permutations remain separate.

The earlier integration test `TestRotatedConversationSurvivesProxyRestart`
had accidentally used the trace header `thread-id` as a durable routing key.
It now exercises the same restart, initial 429 rotation, successful response,
subsequent HTTP request and private hashed persistence with the canonical
`session_id` header. The test was not disabled: its real transport and
process-restart owner assertions are preserved. An unrelated generic routing
test still covers serialized first-owner selection for legacy commands.

## HTTP Responses must not mint session owners from upstream response payloads

The tagged Prodex 0.438.3 `response_forwarding.rs` records
`remember_runtime_session_id` from the successful **request** session
semantics, then extracts only response IDs and turn-state from response
framing. Godex's shared response observer previously promoted a session-like
field from the response body, nested `response` or `data` object, legacy
`conversation_id`/`thread_id` alias, or upstream `session-id` header to a
new **verified** session owner. This could make an unrelated later request
select an attacker- or provider-supplied foreign session lineage.

The source/binary/manifest-pinned exploratory HTTP differential on a clean
pre-fix commit confirmed that Prodex 0.438.3 leaves all six response-only
session shapes unbound while persisting the genuine response ID on main. A
positive control shows that an identical session string explicitly supplied
by the **request** does become main-owned. The pre-fix Go Router failed all
six negative memory/durable variants; its response observer now discards
session and thread ownership extracted only from upstream output. The
existing request-owned success path and response-ID/turn-state/usage
observation are preserved, including committed SSE and WebSocket event
observation. Admission fixture setup now seeds its real `session_id` using a
successful independent request instead of an artificial response annotation.

The mandatory `--phase=http-session-conflict` oracle compares seven synthetic
response variations per product, with two independently authenticated Ready
profiles, one exact unchanged inference, HTTP output/status and durable
response/session projections. Its negative controls reject wrong account,
forged readiness, duplicated inference, changed request/response, injected
metadata, and invalid synthetic auth. As with the other finite fixture
slices, native cross-OS runtime and live OAuth/provider permutations remain
outside this verified contract.

## Record every supported response ID alias, not only the first

Prodex 0.438.3's compiled Mojo `runtime_response_metadata_json_v1` extracts
up to three distinct IDs from a JSON response: `response.id`, top-level
`response_id`, and top-level `id` when `object` is `response` or ends in
`.response`. It deduplicates values but does not treat arbitrary event-root
IDs as continuations. The HTTP Responses success path remembers the full
ordered set. Godex's response parser selected only the first, so a later
continuation referencing another valid ID could lose its verified owner.

The new memory/durable Router regression failed before the fix for second
and third aliases, including duplicate and event controls. The response
observer now retains all IDs (bounded by the existing affinity limit) while
still preserving the single-ID public parser contract, byte-exact downstream
responses, and Copilot's separate native ID policy. Complete SSE metadata
batches also retain every supported ID; the original response-owner path
continues to associate the response turn-state with each verified alias.

The source/binary/manifest-pinned HTTP differential now adds triple-ID,
duplicate-ID, legacy/root-ID, and event-root negative controls. It checks
the actual persisted response-owner projection, a single authenticated Ready
upstream request, the exact unmodified response bytes, and the existing
response-only session protections. Native and live-auth fault equivalence
remain separate parity work.

## Deterministic ASCII-only JSON response turn-state header matching

Tagged compiled `runtime_response_metadata_json_v1` examines response
`headers` objects in sorted serde_json key order and scans array entries in
the original array order. The matching name must have the exact byte length
of `x-codex-turn-state` and is folded only for ASCII letters; Unicode
casefold lookalikes such as `ſ` and keys padded with whitespace are not
accepted. Godex's earlier map iteration was randomized and its
`strings.EqualFold(strings.TrimSpace(name), ...)` could accept both impostor
classes. It could thereby verify a different turn-state and later route
continuations to an incorrect profile.

A fail-before parser regression exercises duplicated-case keys, Unicode
impostors, padded names, ordered array pairs, array object/name variants and
nested-response precedence across repeated iterations. The Go observer now
sorts decoded JSON object keys for this one metadata concern and checks
exact-length ASCII-compatible header names, while preserving array order.
The source/executable-pinned response-metadata differential includes six
binary cases that compare the selected and rejected turn bindings, original
wire payload, authenticated Ready inference count and persistent response ID.
No alternate TUI/framework or persistence layout is introduced.

## Prompt-cache affinity uses whole Unix seconds at the retention boundary

The tagged `affinity/prompt_cache.rs` stores `bound_at` as an `i64` epoch from
`Local::now().timestamp()` and prunes only entries strictly older than
`now - 6 * 60 * 60`. The oldest-to-newest eviction of an over-capacity
`BTreeMap` is stable for equal epochs, so equal-age entries use lexical key
order. Godex previously kept fractional `time.Time` values, causing an
otherwise valid binding to expire up to one second earlier, and allowing
subsecond ordering to discard a different key when the 2,048-binding cap
was exceeded. The local regression reproduces both differences before the
fix, with an inclusive six-hour boundary, a just-expired control, and two
bindings in the same epoch competing for the final slot. Godex now normalizes
prompt-cache timestamps to Unix seconds before any lookup, update or pruning.
This is a clock-source and bounded-memory policy comparison with the exact
published 0.438.3 implementation; it does not assert a new live-provider or
cross-binary network differential for subsecond timing.

## WebSocket input `turn_state` is not a hard continuation alias

The tagged 0.438.3 `runtime_request_semantic_plan` accepts an exact
`x-codex-turn-state` JSON member, or `client_metadata.x-codex-turn-state` when
the direct member is **absent**. A separate `turn_state` field is valid in
**response** metadata, not as a WebSocket request continuation identity.
Godex's general-purpose request decoder formerly promoted direct or nested
`turn_state` into hard-affinity, permitting unrelated JSON to select the
wrong profile. A two-account memory/durable fail-before test reproduces the
foreign owner selection for both legacy locations and their combination;
canonical direct/nested turn and direct-null shadowing are positive controls.

The scoped correction reuses the same presence-aware metadata parser already
used by the WebSocket dispatcher. It neither modifies original JSON bytes nor
adds a handshake turn-state header from the unsupported aliases. The pinned
`--phase=websocket-compact-lineage` differential includes three independent
legacy-alias negative cases: a verified Compact-origin turn on second cannot
steal a self-contained request from active main, and must not be rebound or
released. The oracle checks authenticated Ready evidence, one exact WebSocket
upstream message, original history, no injected turn header, visible output,
and durable Compact/response owners. This finite parity slice does not certify
native socket fault behavior on macOS/Windows or live OAuth integrations.

## Accept compiled cached-token fields and the full unsigned-64 count

The tagged Mojo `runtime_response_raw_usage` chooses the first valid unsigned
cached-token count in this exact precedence: `cached_input_tokens`, then
`input_tokens_details.cached_tokens`, then
`input_tokens_details.cached_input_tokens`, then
`prompt_tokens_details.cached_tokens`. An explicit zero blocks the lower
priority fields. The raw `prompt_cache_hit_tokens` provider alias is not part
of this canonical extraction (provider translators separately normalize it).
The parser accepts the full `0..18446744073709551615` unsigned range plus
negative zero; decimal, negative and overflow values do not qualify.

Godex's prompt-cache observation previously handled only the first nested
field plus a raw provider alias, omitted three official field forms, and
rejected valid counts above signed 64-bit range. Source-derived table tests
reproduce all positive, precedence and invalid-value cases before the fix.
The scoped routing parser now implements the canonical order and uses checked
unsigned parsing, preserving the exact returned count for prompt-cache owner
comparisons. An observation-flow test independently checks that a larger valid
native cached count can promote a new owner, while smaller or invalid counts
cannot. This is tagged-source/local-use-case evidence; it does not establish
live-model cache-hit measurement or complete 1:1 transport parity.

## Native terminal usage markers honor compiled cached/reasoning field order

The same pinned 0.438.3 `runtime_response_raw_usage` parser uses direct
`usage.cached_input_tokens` ahead of the cache-detail alternatives, and
`usage.reasoning_tokens` ahead of both reasoning-detail alternatives.
**Presence**, including a valid zero, determines precedence. The compiled
usage planner also accepts direct root usage fields and only returns an
observation when at least one recognized unsigned field is present. A raw
`prompt_cache_hit_tokens` field by itself does not satisfy that contract.

Godex's terminal usage marker previously missed direct cached/reasoning
counters, lost an explicit zero against lower-priority fields, emitted an
all-zero event for empty/unsupported usage, and could not recover a native
usage record whose fields were at the root. Fail-before table tests document
all of those differences. The routing use case now shares a single
presence-sensitive unsigned field reader for its cached, input, output and
reasoning counters while retaining its established terminal-event gate.
A real `Router.Observe` regression checks full `uint64` precision in the
runtime event and suppression of a forged raw-provider-only count. These
are source-aligned and synthetic local checks, not a certificate for every
stream layout, live provider or native platform.

## Bounded, ordered nested usage scanning and strict JSON framing

The tagged Mojo `runtime_response_raw_scan_usage` inspects the immediate
`usage` member first, then direct fields, then arbitrary nested object/array
values. Input JSON object keys are serialized by serde_json in sorted order
before compiled scanning. Its guard admits no more than 256 nesting levels
and 2,048 visited values; a further field after the bound cannot contribute
usage evidence. Godex previously followed only four named wrappers and did
not scan arrays or unknown wrappers, so it could omit cached-token affinity
and usage markers carried in otherwise valid nested envelopes.

A single narrowly owned `internal/usecase/routing/usage_scan_04383.go` scanner
now serves both prompt-cache affinity and terminal usage observations, with
the same precedence, key ordering, depth and visited-value bounds. Regressions
fail before this consolidation for unknown wrapper objects, arrays, lexical
key precedence, valid 255-level nesting and bounded value traversal. The
scan also uses a complete JSON decode rather than accepting Go Decoder's
first JSON value when a second object, array or invalid trailing fragment is
present. The latter could previously promote an invalid upstream payload to
a cache owner. Positive controls preserve valid trailing whitespace, root
arrays and all previously qualified native usage fields. Terminal-event
eligibility remains a separate unchanged Godex lifecycle rule; these tests
do not certify externally authenticated streaming behavior or native OS
specifics.

## Paired executable cache-affinity selection after native usage observations

A new source/manifest/executable-pinned `--phase=http-prompt-cache` gate
checks whether the compiled cached-token parser affects actual account
selection rather than only Go parsing. Each isolated scenario applies Ready
quota receipts to two authenticated synthetic managed profiles, records a
cached hit on previous-response-pinned main, sends the same prompt key to a hard-pinned second profile,
restores the active main profile, and issues one fresh prompt-key request.
The selected profile for the fourth request must follow the source's cached
hit comparison, including canonical direct/nested fields, zero/legacy
negative controls, checked overlarge input with fallback, and the full
unsigned-64 maximum. The oracle checks exact request/response bytes, four
single owner-directed upstream attempts and all four durable response IDs.
This finite HTTP selection proof does not substitute for live-model cache
measurement, provider authentication, or native Windows/macOS fault parity.

The native `scripts/verify-native-parity.ps1` matrix now executes
`--http-prompt-cache` on the same canonical release pins for each supported
macOS and Windows host. A targeted repository test prevents the native matrix
from silently omitting the new executable oracle in subsequent changes. The
release workflow step label also identifies 0.438.3 consistently with its
pinned script. Native results must still run on actual native hosts; a Linux
execution or cross-build cannot be counted as a macOS/Windows PASS.

## Reject raw invalid UTF-8 before deriving JSON request/usage identity

The exact 0.438.3 Rust `runtime_request_semantic_plan` is reached through
`serde_json::from_slice` and rejects malformed UTF-8. Go's `encoding/json`
instead replaces invalid string bytes with U+FFFD; before this correction, an
invalid Responses body containing a would-be `session_id` or
`previous_response_id` could borrow a verified owner corresponding to that
replacement string. The same decoder difference could promote a malformed
WebSocket `response.create` message's turn/session/previous identity or give
an invalid request a prompt-cache affinity or cached-token usage marker.

Memory and durable Router regressions demonstrate the foreign-owner risk
before the fix; the WebSocket parser test independently covers all three
message identifiers. Godex now rejects raw invalid UTF-8 from **semantic
inspection** of request bodies, WebSocket message metadata and
prompt-cache/usage JSON, while preserving upstream request bytes and valid
HTTP header-based continuation identity. A deliberate well-formed U+FFFD
string remains valid, ensuring this does not reject legitimate Unicode.
The source/manifest/executable-pinned `http-session-conflict` oracle now also
includes an otherwise canonical session request containing invalid UTF-8 in
an unrelated JSON field: it must not select the foreign session owner, and
upstream raw bytes and durable bindings remain unchanged. Other malformed
Unicode escape permutations and live external-provider behavior remain
separate parity questions.

## Reject unpaired Unicode surrogate escapes before JSON affinity inspection

The same tagged 0.438.3 `serde_json::from_slice` parser rejects an unpaired
UTF-16 surrogate escape inside any JSON string or member name. Go's
`encoding/json` instead accepts `\\ud800` or `\\udc00` by replacing the
invalid scalar with U+FFFD. Raw byte-level UTF-8 validation alone cannot
catch these fully ASCII escape sequences, allowing an invalid request to
claim a verified session/turn/previous-response owner or to supply prompt
cache and token-usage evidence.

A focused, allocation-free validation scan now checks properly paired
surrogate escapes in JSON strings before any request/body, WebSocket metadata,
prompt-cache or native usage semantic extraction. Ordinary JSON decoding
retains responsibility for all other syntax. Tests reproduce five invalid
surrogate shapes and unpaired escapes in object keys, then protect valid
`\\ud83d\\ude00` pairs and literal escaped-backslash text (`\\\\ud800`).
No invalid request is rewritten; the original upstream wire bytes are still
preserved. The source/manifest/executable-pinned HTTP session-conflict oracle
adds lone-high and lone-low invalid-body controls plus valid-pair and literal
escaped-text controls, verifying the selected owner, untouched HTTP body and
existing durable bindings on the exact published 0.438.3 binary. This finite
JSON semantic boundary does not certify every provider and OS integration.

## Invalid Unicode response metadata must not create continuation owners

The tagged `prodex-runtime-proxy/src/payload_detection/response_metadata.rs`
extracts response IDs, turn-state and token usage only after a successful
`serde_json::from_slice` of the **entire** upstream response or SSE event.
Godex's response-ID observer already rejected raw invalid UTF-8, but its
`encoding/json` parser still accepted unpaired `\\ud800`/`\\udc00` escapes,
including malformed member names. A fake upstream could therefore cause a
malformed response to mint a verified `previous_response_id` owner that the
published Prodex executable leaves unbound.

A memory/durable Router regression first reproduced the missing boundary for
malformed high/low surrogates and invalid JSON keys, with valid pair and
escaped-backslash positive controls. An independent SSE event regression
checks the same policy after commitment. The response metadata observer now
reuses the existing scoped JSON Unicode validator, leaving header handling,
original response bytes, successful canonical aliases and usage accounting
unchanged. The source/manifest/executable-pinned `--phase=http-session-conflict`
fixture also checks that a malformed upstream response returns its original
HTTP body while its response ID remains **unbound**; the valid pair still binds
on the selected Ready owner. This synthetic fixture is not a claim of complete
provider, OAuth or native platform parity.

## WebSocket nested previous-response JSON is not hard continuation affinity

Tagged `parse_runtime_websocket_request_metadata` invokes
`runtime_request_semantic_plan`, which reads the exact direct
`previous_response_id` member only. Godex preserved an older generic
`response.previous_response_id` fallback in its WebSocket router, allowing a
nested JSON field to borrow a foreign profile's verified response owner.
This became a behavioral mismatch even when request metadata parsing itself
correctly ignored the nested member.

A source/manifest/executable-pinned exploratory `--phase=websocket-compact-lineage`
comparison on the clean pre-fix candidate `48eb2619` reproduced the divergence:
two authenticated Ready managed profiles held `resp-ready-b` on second while
current main was active. The exact full-history WebSocket request carried only
`response.previous_response_id: resp-ready-b`. Official Prodex dispatched one
unchanged request to **main** and completed; Godex incorrectly dispatched
that request to **second**. The bad candidate was not pushed.

Godex's request-affinity parser now uses only the direct previous-response
member for WebSocket as well as HTTP Responses, without modifying the request
bytes, outer HTTP headers, or the separate response-ID extraction policy.
Memory and durable regressions check missing/null/blank direct fields versus
an owned nested value, hard-direct positive controls, precise WebSocket policy,
a single inference and the original foreign/created durable owners. Legacy
WebSocket tests that intentionally exercised a real continuation now send its
canonical direct member; an explicit nested-only negative control remains.
The pinned WebSocket Compact lineage oracle checks the exact upstream request,
Ready quota evidence, no injected turn header, the original second-owned
response and successful new main response, with mutation controls for wrong
owner, lost history, extra inference and missing completion. Passing this
finite case does not close native socket, live OAuth/provider or all 1:1 gaps.

## Reject non-finite JSON numbers before native usage observation

The exact 0.438.3 `Cargo.lock` pins `serde_json` **1.0.151**. Its
`serde_json::from_slice::<Value>` rejects a non-finite floating literal
such as `1e309`, `-1e100000`, or a nested `1.5e999` **anywhere** in the
JSON document. Godex's whole-response usage scanner used
`json.Decoder.UseNumber` to preserve full unsigned-64 cached-token counts,
but that Go decoder also accepted otherwise out-of-range numeric syntax.
Consequently an unrelated overflowing numeric field could incorrectly
promote prompt-cache affinity or emit a terminal usage marker from another
field in the same invalid upstream response.

An isolated offline Rust probe using the **exact pinned serde_json 1.0.151**
confirms that `1e309` and negative overflow are invalid, while `1e308`,
`1e-99999` (finite underflow) and unsigned-overflow integer literals remain
valid JSON. Fail-before routing tests cover overflow before, after, and
inside an arbitrary nested object alongside legitimate cached-token usage.
The response scanner now verifies every decoded `json.Number` is finite
before yielding any usage evidence. It retains `Decoder.UseNumber` for exact
`uint64` cached counters and accepts finite/underflow numeric controls.

The source/manifest/executable-pinned `--phase=http-prompt-cache`
comparison now includes a four-request authenticated Ready fixture for a
second-owned malformed response with valid cached tokens plus an unrelated
`1e309`. Both binaries must preserve the exact HTTP 200 body, leave its
response ID unbound, reject the cached hit, and keep the fresh prompt-key
request on main. Separate `1e308` and `1e-99999` controls must allow a
verified larger cache hit to select second. The durable previous-response
owners of every valid stage are independently checked. This is a bounded
synthetic parser/selection parity slice, not proof of full live provider or
native-platform equivalence.

## Enforce the serde_json container recursion limit before Mojo's usage scan

Prodex 0.438.3 pins `serde_json` 1.0.151. Its runtime response extraction
first parses the **complete** JSON into a Rust `serde_json::Value`, then
passes it to the compiled Mojo metadata planner. This Rust parser accepts
127 nested object/array containers but rejects the 128th. The Mojo metadata
scanner independently has a 256-level / 2,048-node traversal budget; that
larger budget does **not** authorize an input the outer parser has rejected.
Go's `encoding/json` previously decoded these deeper documents and could
therefore mint session/previous-response owners or cache usage from malformed
request or response data whose unrelated nested field crossed the Rust limit.

A source-version-pinned **offline serde_json 1.0.151 execution** confirmed the
127/128 boundary for objects and arrays, including an otherwise valid usage
record preceding the invalid nested field. New fail-before tests demonstrate
the discrepancy across request affinity, WebSocket message metadata,
response IDs, prompt-cache keys, and terminal cached usage. A private
validator now checks container nesting after Go's decoder completes and
before those semantic projections are applied. The independent Mojo walker
retains its own node budget, and original transport bytes remain untouched.

The exact source/manifest/executable-pinned `--phase=http-prompt-cache`
differential now includes a second-owned response with valid cached tokens
but an unrelated 128th nested container. Both products must forward the
response, leave its response ID and cached-hit **unrecorded**, then select
main for the next fresh prompt-cache request. The 127-container positive
control must record the response ID and select second. All four authenticated
Ready upstream requests and durable state projections are checked; this
finite Linux fixture does not prove native macOS/Windows or live model parity.

## Verify hard binding components before persisting response/session ownership

Prodex 0.438.3's tagged `RuntimeHardBindingIdentity::response`, `::session`
and `::turn_state` share compiled Mojo's component classifier: a normalized
identity must contain at most **1,024 UTF-8 bytes**, be valid UTF-8, and
contain no C0/C1 control characters. The separate **4,096-byte composed key**
limit is not the component limit. Godex previously permitted 4,096-byte
binding components and control-bearing strings, so a successful HTTP request
or response could mint a verified hard owner that the pinned Rust runtime
rejects. The larger limit still has uses for internal composed Compact aliases;
blindly reducing the global key limit would corrupt valid 1,024-byte aliases.

Memory and durable fail-before tests demonstrate wrongly persisted response
IDs at 1,025 bytes and both control classes, with valid 1,024-byte and
Unicode controls. A second independent test asserts the same boundary on
canonical **request-owned session IDs**. The narrow correction reuses the
already-source-audited Compact `RuntimeHardBindingIdentity` component check
before successful request/response ownership observations, including additional
response ID aliases. It deliberately leaves internal namespaced Compact keys,
request wire bytes and upstream transport headers unchanged.

The pinned two-executable `--phase=http-session-conflict` oracle now also
compares a successful authenticated Ready request receiving a 1,024-byte
response ID (must bind), a 1,025-byte response ID (must remain unbound), and
two valid JSON strings encoding forbidden C0/C1 characters (must remain
unbound). Their original HTTP bodies, owner choices and durable ID projections
are independently checked. It does not establish every live provider or native
operating system scenario.

## Opaque response wrappers cannot mint response IDs or turn owners

The compiled tagged `mojo/prodex_core/runtime_response_metadata.mojo`
only extracts the immediate `response.id`, top-level `response_id`, or a
qualifying top-level `id` as a hard previous-response identity. Turn-state
comes from immediate response/root headers or their direct `turn_state` /
`turnState` fields, not from arbitrary `data`, `result`, or `payload` wrappers.
Godex's generic `collectNestedAffinity` recursively adopted IDs and turns
from those unrelated wrappers, permitting synthetic provider payloads to
create new verified account owners. A fail-before Router regression reproduced
wrongly bound IDs for `data.id`, `result.id`, `payload.id`, nested
`data.response_id`, and turn-state through all three wrappers in both memory
and durable stores.

The scoped fix removes that generic nested affinity recursion and uses the
source-defined response ID and turn-state projections. It retains legitimate
`response.id`, top-level `response_id`, and nested `response.turn_state`
positive controls, without changing the original returned JSON. A separate
SSE event test enforces the same rule after streaming commitment. The exact
manifest/source/executable-pinned `--phase=http-session-conflict` gate now
compares eight independent opaque and canonical variants, requiring two
applied authenticated Ready profiles, one exact upstream dispatch, preserved
HTTP response bytes, unchanged session lineage, and empty or correctly
verified durable response/turn owners. This verifies a bounded 0.438.3
contract, not full live-provider and native-platform parity.

## Embedded response headers: `null` shadows alternate key/value members

The tagged Mojo `runtime_response_raw_headers_state` walks each object in a
JSON `headers` array by looking up `name` first, then `key` **only if name is
absent**. Similarly, it selects `value` first, then `values` **only if value
is absent**. A present JSON `null` is a valid member-presence marker but an
invalid header name/value; it cannot be replaced by the alternate field.
Godex previously used `entry["name"] == nil` and `entry["value"] == nil`,
which conflated missing and explicit `null` and could silently bind an
upstream-supplied foreign `x-codex-turn-state` token.

Deterministic fail-before regressions reproduce this for null names and
values, with subsequent valid entries and genuinely absent-member fallback
as positive controls. Separate memory/durable Router observations prove that
null-shadowed tokens cannot mint turn owners while the actual response ID
and later legitimate turn token retain their ownership. The scoped parser
correction uses Go's map `(value, present)` lookup, matching the source's
member-presence rule without modifying response bodies or HTTP headers.

The source/manifest/executable-pinned `--phase=http-session-conflict` gate
now compares six header-array variants (null name/value, subsequent real
entries, and missing-name/value controls) in both binaries. It checks one
correct Ready upstream attempt, the original response bytes, and the exact
durable response/turn owners, including absence of the forged `foreign-turn`
owner. This does not by itself certify every live provider or native OS path.

## Preserve per-event response/turn-state association in batched SSE

Prodex 0.438.3's tagged `RuntimeSseTapState::observe_stream_event` updates its
current turn token as SSE events arrive, then records each **new** response ID
with that current state. Previously bound IDs are not rewritten when a later
event changes the turn; when the first turn arrives late, IDs without a turn
are backfilled once. Godex's batched SSE observer instead merged all response
IDs with the **first** observed turn-state, so a two-event stream with
`resp-first / turn-first` and `resp-second / turn-second` could give both
responses the `turn-first` lineage.

A fail-before memory/durable Router regression demonstrates the drift and
protects both same-turn inheritance and late-turn backfill. The scoped
`observeSSEBindings04383` routine now processes complete SSE events in order,
retains already-bound ID-turn pairs, and applies each event's own native usage
observation. Copilot's provider-specific response ID policy remains on its
existing route. The original SSE response bytes and downstream framing are
unchanged, with profile-home sidecars still available only to the owning
managed profile.

The source/manifest/executable-pinned HTTP session-conflict oracle includes an
authenticated two-Ready-profile SSE response: one upstream request on main,
two exact client-visible events, two durable response IDs, and the correct
per-response turn-state provenance without cross-assignment. Negative controls
reject altered response events, forged Ready evidence, wrong account, extra
inference, and lost terminal output. This finite synthetic oracle is not a
certificate for every live provider or native OS stream/fault permutation.

## WebSocket tool-output type suffix must preserve trailing whitespace

The tagged `runtime_proxy_input_item_is_tool_output` compiler checks the
**decoded, untrimmed** item `type` for its exact `_call_output` suffix,
while checking `call_id` separately for nonblank text. A trailing space or
tab after `function_call_output` invalidates this suffix; a leading space
alone does not. Godex's WebSocket request-metadata decoder previously
applied `strings.TrimSpace(type)` before checking the suffix, which
incorrectly converted a non-tool input into a hard previous-response
dependency and could spend the three locked-owner retry attempts on a
stale `previous_response_id`.

A fail-before table regression exercises canonical/custom types, trailing
space/tab, leading whitespace, blank call IDs and legitimate IDs. An
independent memory/durable Router regression verifies actual behavior after
`previous_response_not_found`: canonical or leading-space tool-output
messages spend the tagged bounded owner retry schedule, whereas trailing
space/tab messages report stale continuation after one exact upstream
attempt. No user JSON is trimmed or rewritten.

The source/manifest/executable-pinned `--phase=websocket-compact-lineage`
fixture additionally compares four tool-suffix variants against the
published Prodex 0.438.3 executable. Two authenticated Ready profiles are
warmed and the same response ID is verified on main. The synthetic upstream
returns a precommit `previous_response_not_found` for each request; the gate
checks owner, exact input bytes, number of upstream attempts, client terminal
frame, unchanged durable previous ownership and no unintended turn header.
A mutation oracle rejects wrong owners, missing Ready evidence, altered
request, unexpected replay count or forged completion. Native OS and live
provider faults remain independent parity questions.

## Full-history HTTP recovery respects direct session member presence

Tagged `runtime_request_semantic_plan` resolves the canonical request
`session_id` by checking whether the *direct* JSON member exists before
considering `client_metadata.session_id`. A direct null, blank or other
non-string value shadows the nested field, even though it does not supply a
usable session ID. The same presence rule applies to JSON from the
`x-codex-turn-metadata` header. Godex already honored this policy when
selecting the initial account, but `fullHistoryRecoveryRequest` used a
separate `sessionIDInMetadata` helper that treated an unusable direct member
as *absent*. After the upstream rejected a known `previous_response_id`,
Godex could therefore replay an otherwise independent full history under a
borrowed nested session.

A fail-before predicate regression covers null/blank/boolean/object direct
members, valid direct/nested controls, and the corresponding metadata-header
shadows and overrides. An HTTP Router regression independently verifies that
null/blank direct members retain the one original upstream rejection rather
than issuing a second recovery inference, while legitimate nested session
context still permits a bounded replay with `previous_response_id` removed.
The scoped correction applies direct-member presence to the recovery helper
without rewriting the initial request body or relaxing owner checks.

The source/manifest/executable-pinned `--phase=http-turn-transport` oracle now
includes seven authenticated two-Ready-profile HTTP recovery controls. It
checks the selected main owner, exact original request, preserved turn-metadata
header, one versus two upstream attempts, correct terminal response and
independent durable recovered-response ownership. Its mutation suite rejects
foreign account selection, lost Ready evidence, altered original input,
extra/missing replays and forged terminal output. This finite synthetic case
does not certify every external provider or native macOS/Windows failure mode.

## No full-history HTTP replay from a malformed complete request document

Prodex 0.438.3 builds its previous-response/full-history replay plan only
after successful `serde_json::from_slice::<Value>` of the **entire** inbound
request. A valid `session_id`, `previous_response_id` and reconstructable
`input` do not make an otherwise malformed JSON document replayable.
Godex's recovery path separately used `json.RawMessage`; Go's decoder
accepted unpaired Unicode-surrogate escapes, invalid raw UTF-8, an unrelated
out-of-range `1e309` numeric literal, and a 128th nested container without
rejecting the complete request. On a stale previous-response error, it could
incorrectly reserialize the request and issue a second inference that the
published Prodex planner forbids.

A fail-before regression demonstrates these mismatches alongside valid
Unicode, finite exponential notation and max-unsigned-integer controls.
A second Router regression checks that a malformed request forwarded to a
known previous-response owner preserves the original HTTP bytes and never
sends a recovery replay. Godex now reuses the existing tagged-source complete
JSON validation—UTF-8, Unicode surrogate pairing, numeric range, container
depth and EOF—before extracting stale previous IDs or attempting a full
history rewrite. The same guard rejects malformed JSON in the
`x-codex-turn-metadata` header as session evidence without blocking an
otherwise valid explicit source.

The source/manifest/executable-pinned `--phase=http-turn-transport`
comparison now extends the prior HTTP full-history replay gate with raw
invalid UTF-8, unpaired surrogate, non-finite float, over-depth JSON,
malformed metadata header, and valid Unicode/finite-number positives. It
verifies two applied authenticated Ready profiles, one exact original model
request, one or no valid bounded recovery attempt, unchanged header bytes,
terminal 400/200 and independent durable response ownership on both official
Prodex 0.438.3 and Godex. This is a bounded synthetic HTTP recovery slice,
not a claim about all OAuth/provider errors or native OS integrations.

## WebSocket/turn-state recovery requires valid complete JSON before scrub/replay

The tagged 0.438.3 `runtime_request_has_reconstructable_full_history`
first parses the **entire** request as `serde_json::Value`; invalid JSON
cannot establish a complete-history replay shape. The related
`runtime_request_without_turn_state` removes stale turn-state handshake
headers, but only re-serializes the request body when full serde parsing
succeeds. In contrast, Go's `json.RawMessage` accepted otherwise malformed
requests with unpaired Unicode surrogates, invalid raw UTF-8, non-finite
numeric literals and excessive container nesting. Before this correction,
Godex could classify such a message as reconstructable, rotate it on a quota
failure, or transform its invalid original JSON into different bytes while
scrubbing obsolete turn-state metadata.

A fail-before table regression reuses the independent Rust 1.0.151
compatibility fixtures from HTTP previous-response recovery and checks both
reconstructable-history classification and unchanged invalid body bytes.
Positive controls preserve valid escaped Unicode, finite exponents and full
unsigned-64 precision. A separate WebSocket Router test verifies that an
owned turn-state request carrying an invalid surrogate/overflow does **not**
use the quota-based alternate, whereas fully valid history still makes the
original bounded owner-to-alternate recovery. The scoped Go correction reuses
the already audited whole-document JSON validator, and keeps the tagged
outbound turn-state header removal independent of body re-serialization.

These are pinned-source and local end-to-end tests. Although the general
`--phase=websocket-transport` oracle remains part of the 0.438.3 release
matrix, the exact malformed-turn recovery variants are not yet paired across
native Prodex and Godex executables; this case is **source-aligned**, not a
cross-binary certificate for the entire recovery stack.

## Reject invalid structured previous-response errors before stale owner recovery

The tagged `prodex-runtime-proxy/src/payload_detection/error_messages.rs`
implements `runtime_proxy_body_is_invalid_previous_response_id` by first
parsing the entire upstream HTTP response as `serde_json::Value`. An
otherwise matching `invalid_request_error` and exact
``Invalid `previous_response_id`.`` message cannot be trusted if an unrelated
field contains malformed UTF-8, an unpaired Unicode surrogate, or excessive
JSON nesting. Go's `encoding/json` previously accepted those invalid shapes
and caused `invalidPreviousResponseID` to trigger the owned-history replay
and release an otherwise verified previous-response owner.

A fail-before classification regression reproduces incorrect positives for
invalid UTF-8, surrogate pairs and over-depth documents, with normal error,
valid Unicode and finite-number controls. A response-recovery Router test
independently checks that an invalid 400 error does not send an additional
upstream model inference; a canonical valid error still permits one bounded
same-owner full-history replay. The scoped correction shares the audited
whole-document serde-compatible decoder used for native response usage and
request semantics, and retains the original response status and bytes.

The source/manifest/executable-pinned `--phase=http-turn-transport` fixture
now includes three synthetic malformed **upstream error response** bodies
alongside the existing request-recovery cases. Both Prodex 0.438.3 and Godex
must retain the response and its original verified ownership, not manufacture
a second model inference from invalid structured error evidence. The fixture
checks applied authenticated Ready receipts, original request/response bytes,
terminal HTTP status and durable binding projections. This is a bounded
synthetic protocol comparison rather than a claim about all live providers.

## Invalid whole SSE JSON cannot forge quota or rate-limit precommit signals

The tagged 0.438.3 `parse_runtime_sse_payload` parses the complete SSE event
with `serde_json::from_str`; malformed JSON returns the neutral/default
inspection event, not its partially readable `response.failed` quota/rate
signal. The compiled `runtime_sse_inspection_step_v1` then treats that
unrecognized event as commit-ready rather than rotating a Ready upstream
profile. Godex's `streamOutcome` previously used Go's permissive
`json.Unmarshal`, allowing an otherwise recognizable `response.failed`
message with an unrelated unpaired surrogate or excess JSON container depth
to falsely trigger a quota/rate-limit rotation.

Fail-before parser regressions reproduce the fake precommit retry on
unpaired high/low surrogates and over-depth SSE JSON; they also cover
non-finite floats and valid structured quota/rate-limit controls. An
independent end-to-end HTTP Router regression ensures malformed quota events
are forwarded as their original SSE bytes from the first selected account
without replay. Godex now validates complete serde-compatible JSON before
applying structured SSE error signals, preserving its previously distinct
raw-invalid-UTF8 handling and upstream bytes.

The source/manifest/executable-pinned `--phase=http-turn-transport` oracle
adds paired valid-output and malformed-SSE controls. Each case holds an
independently verified main previous-response owner, applies authenticated
Ready quota receipts to two profiles, and verifies one original main attempt,
exact HTTP 200 event bytes, no cross-profile replay, and unchanged durable
previous ownership. Mutation tests reject forged Ready evidence, changed
request/event bytes, additional inference and incorrect terminal status.
This synthetic comparison is not an all-provider or native-OS certificate.

## Plain-text previous-response errors and malformed JSON do not share affinity semantics

The tagged Prodex 0.438.3 `extract_runtime_proxy_previous_response_message`
first searches a complete `serde_json::Value`, then uses the compiled Mojo
**text** classifier on the full UTF-8 response if structured extraction has
no match. Standalone `previous_response_not_found`, its explanatory prefix,
`Previous response ... not found`, exact invalid-backtick-previous-ID text,
and the narrow `no tool/function call found` cases therefore remain meaningful
precommit errors even when the upstream HTTP 400 is not JSON. Godex
previously inspected only JSON object `code` fields, and silently missed
these literal text errors; a verified previous-response owner would keep the
raw 400 response rather than triggering the tagged **stale-continuation**
contract. The exact pinned 0.438.3 binary revealed a second important rule:
when a verified previous-response owner reports that its chain is missing,
Prodex returns one HTTP **409** `stale_continuation` and refuses to submit
that chain to a different Ready profile—even if the request includes a
reconstructable-looking user/assistant/user history and a valid session.

The same parser must not promote otherwise matching **malformed structured
JSON** to previous-response evidence. The source rejects unmatched UTF-8,
surrogate escapes, overflowing numbers and excessive container nesting, and
its text fallback cannot interpret a JSON body starting with `{` as a
standalone previous-response message. A fail-before regression covers both
directions, using a bounded 2,048-node depth-first, key-sorted traversal that
mirrors the tagged `runtime_json_find` walk for valid structured errors.
An independent HTTP Router test verifies selected owners, an unchanged
original request body, **one** model inference on its verified owner,
and the untouched upstream 400 body when no stale signal is recognized.
Pre-0.438.3 cross-owner rotation expectations were updated to enforce this
observed fail-closed contract, while retained bounded turn-state retry
tests still verify the three same-owner delays before a terminal 409.

The pinned `--phase=http-turn-transport` oracle now also checks multiple
standalone plain-text previous-response variants and invalid structured
JSON cases with two authenticated Ready accounts, byte-exact source and
downstream 409 stale-continuation (or preserved 400 for unrelated text),
and independently verified existing response ownership. Negative mutation
sentinels reject injected readiness, owner diversion, changed error payload,
missing/replayed inference and forged successful completion. This adds a
finite synthetic behavior slice; full external-provider and cross-OS parity
remain open.

## Structured previous-response tags require exact bytes and a bounded walker

The tagged 0.438.3 `previous_response_structured_class` Mojo kernel compares
`code`, `type` and `param` **without trimming or case-folding**: an exact
`code: previous_response_not_found` takes priority over other tag fields,
and only `type: invalid_request_error` plus the recognized invalid-ID
message produces the distinct invalid-ID class. The message permits only
**ASCII** whitespace trimming and ASCII case folding. Go's older recursive
`invalidPreviousResponseValue` instead trimmed/code-folded tags and used
Unicode-aware `strings.EqualFold`, which could mistake `INPUT`, an uppercase
or space-padded type, or a Unicode long-s token for a valid structured tag.
It also descended an unbounded JSON tree, while the tagged Rust
`runtime_json_find` scans at most 2,048 nodes in depth-first lexical-key
order. Those mismatches could cause an unnecessary same-owner HTTP replay,
miss an authorized invalid-ID replay, or route a missing chain incorrectly.

Godex now shares one bounded, deterministic structured-error walker for
previous-not-found and invalid-ID detection. The two classifiers derive
separate actions from the same exact-byte class: NOT_FOUND must fail closed
for a verified owner, INVALID_ID may make one bounded same-owner recovery
with a valid complete history, and unrelated JSON is forwarded untouched.
Fail-before regressions prove exact code precedence, leading/trailing
whitespace, Unicode case-fold traps and the 2,048-node limit. A separate
Router regression verifies actual inference count and HTTP 400/409/200
outcomes alongside unchanged upstream request bytes.

The manifest/source/executable-pinned `--phase=http-turn-transport` oracle
adds structured response errors with padded/uppercase/unicode tags, exact
NOT_FOUND precedence, an over-budget JSON search and valid controls. Each
case verifies authenticated Ready evidence on two accounts, the byte-exact
initial request, the presence/absence of a bounded same-owner recovery,
terminal response status/body, and durable recovered-response ownership.
This verifies a finite 0.438.3 slice rather than every provider/OS path.
