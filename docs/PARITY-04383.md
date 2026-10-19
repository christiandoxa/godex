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
Ready same-provider profile. Only then does it release the failed turn/session
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
