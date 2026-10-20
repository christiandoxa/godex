# Prodex 0.438.3 continuation and WebSocket executable differential

This harness compares an official SHA-256-pinned Prodex `0.438.3` binary
and Godex using isolated two-profile homes, synthetic credentials, and a
loopback-only upstream. It does **not** send real model requests or use
the caller's account credentials.

The default mode checks HTTP Responses continuation ownership, turn-state
affinity, overload/quota and full-context recovery, already-committed
streaming output, cancellation, and process restart. Historical status
and retry differences may remain within this bounded HTTP observation;
the fixture is not a full feature-parity certificate.

The current runner and published native manifest are pinned to source commit
`42042d765100fc06895a27e0904ffb64842092e6` and release-manifest
SHA-256 `300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da`.
The 0.438.2 names below identify where particular compatibility fixtures
originated; their active executable reference is **0.438.3**. All release
phases must use the same clean Godex commit.

## HTTP keep-alive request budgets

`--http-keepalive` sends two Responses requests on the same TCP connection,
31 seconds apart under the configured 30-second minimum request budget. Both
must return exact 200 responses with unchanged input, account-a ownership and
two upstream requests. Reconnection or an automatic HTTP retry invalidates the
proof. Its mutation tests reject each material deviation.

Run `./scripts/verify-release-parity.sh --phase=http-keepalive` for this slice.
It is also mandatory in the complete gate and native executable matrix.

## HTTP Compact transport recovery

`--http-compact` proves both owner-pinned warmups and applied authenticated
quota evidence before closing main's Compact connection without response
headers. Turn-only and accompanying-session requests must retry second exactly
once, preserving the entire opaque payload and turn header. The failed turn
must remain absent from durable state; the unrelated response stays on main,
and the accompanying session becomes a conflict. A previous-response dependency
instead returns the exact 400 reset without replay, retains the session on main,
and removes only the failed hard owners. Mutation controls reject changes to
readiness, authentication, count, owner, wire bytes, large numbers, output,
status and every durable-owner projection.

Run `./scripts/verify-release-parity.sh --phase=http-compact` for this slice.
The complete gate and native matrix require it. This HTTP route is
separate from the still-open strict WebSocket compact-followup control.

`--http-turn-transport` reuses the same authenticated fault fixture on Responses.
Owned turns return exact 503 without replay, with or without an accompanying
session. The original turn, session and unrelated response stay on main.
The previous-response control returns exact 400, releases its hard owners and
retains the session. Its mutations reject replay, changed auth/readiness,
wire bytes, owner, status, body and durable state. Run
`./scripts/verify-release-parity.sh --phase=http-turn-transport` for this slice;
the complete gate and ten-mode native matrix require it.

## Authenticated precommit transport recovery

`--websocket-transport` uses a managed `run` launch with two profiles.
The `gateway` command fixes its pool to the active profile and cannot prove
cross-profile recovery. Two owner-pinned warmups establish authenticated
loopback quota receipts without changing the failed turn's main owner.
The fixture also waits for Prodex to log both successful quota-cache updates;
receiving a probe at the upstream alone does not prove its result was applied.
The upstream accepts a complete request, sends hidden metadata, then closes
before commitment. Both products must attempt main exactly once and the Ready
alternate exactly once, remove the failed turn from headers and nested metadata,
preserve all history/tool arguments/large integers, and emit exact client frames.
The official runtime log must confirm `quota_source=probe_cache` for the alternate.

The durable snapshot must retain the recovered response on second, an unrelated
response on main, no failed turn binding, and the accompanying session conflict.
The same ownership and exact replay assertions also cover a precommit quota
failure after hidden metadata, using the official quota-recovery log marker.
Delta-only input returns the exact terminal 503 without replay. A previous-response
dependency returns the exact full-context 400 signal without replay. Mutation tests
reject lost auth evidence, extra requests/handshakes, changed owners, rounded
integers, stale tokens, altered output and empty terminal responses.
Their durable snapshots also require the session to remain on main. Delta-only
failure retains both hard bindings; the previous-response signal releases them.
The `turn-missing` case has a turn token but no previous-response ID and receives
an upstream missing-response error. It retries on second with the entire opaque
request unchanged, retains the original turn on main, records the new response
on second and persists the session conflict. This differs from a transport fault,
which removes the failed turn token before replay.
The unbound variants remove the initial turn binding before startup, finish both
authenticated quota probes and make main current with the final warmup. Complete
history then recovers on second with `affinity_released=false`; delta-only still
returns the exact 503 without replay or creating an owner for the unknown turn.
The corresponding unbound quota cases use the same history proof. Complete
history recovers on second; delta-only preserves the exact original 429 quota
frame, one inference and unchanged durable ownership.
Run `./scripts/verify-release-parity.sh --phase=websocket-transport` for this
source/binary/manifest-pinned slice. Strict compact-followup setup and native
macOS/Windows qualification remain separate from the Linux release gate.

The same mode also runs three full-history lifecycle controls with an
authenticated Ready alternate: visible output followed by upstream TCP EOF
must retain both exact frames and the reference's abrupt EOF without replay;
client cancellation after dispatch must close the upstream; and hidden
keepalives must end in the original 30-second 504 despite the available
alternate. Independent mutations reject changed close framing, partial EOF,
missing output, extended/early deadlines, failed cancellation or extra inference.

## Exact HTTP history replay boundary

The opt-in `TestProdex04380ExecutableHistoryReplayBoundary` test verifies nine
request-history shapes against the pinned executables. Each case creates its
response owner with a real successful HTTP seed request before the upstream
rejects the previous ID. Only exact compaction/completed-turn/function-call
histories allow a second same-owner attempt; padded tags and
`compaction_trigger` retain the original 400. Recovery removes only the previous
ID and preserves context-window metadata and all other request fields.

`scripts/verify-release-parity.sh --phase=preflight-oracles` supplies all five
`GODEX_TEST_HISTORY_*` executable/source/commit settings and runs the test.
Ordinary tests skip this executable fixture when paths are absent. Offline
negative controls reject missing seeds, extra attempts, changed owners,
false success, changed errors, retained previous IDs, and lost context.

## Strict WebSocket differential

Use `--websocket` to exercise **nineteen** request paths against both
executables (including three paths on a freshly restarted gateway). The
original thirteen owner/continuation paths are listed first:

1. Seed a soft `session_id` preference and record its actual selected owner.
2. `response.created` and `keepalive` followed by precommit overload:
   the failed prelude must be discarded before the successful retry.
   Prodex retries the *same* first profile in this deterministic fixture.
3. A heartbeat and visible output followed by overload must not trigger a
   second upstream attempt or duplicate model execution.
4. A verified `previous_response_id` must outrank a conflicting soft session.
5. A verified `x-codex-turn-state` must also outrank the soft session.
6. An *unbound* `previous_response_id` may discover an upstream profile.
7. An *unbound* `turn_state` may likewise discover a profile.
8. An *unbound* missing previous-response chain containing a completed
   tool call must make four bounded **same-owner** attempts while preserving
   every tool field, then surface `error / 409 / stale_continuation`.
9. A *verified bound* previous-response chain with a missing completed tool
   result must follow the same bounded retry and generic error frame contract.
   Ordinary bound continuations without tool output retain their original
   response envelope.
10. A soft-session-only request after hard-affinity activity retains the
    pinned reference's observed owner selection.
11–13. Restart the gateway process and repeat hard previous-response,
   hard turn-state, and soft-session-only requests. No local socket or
   in-memory router cache survives the restart.

The four additional tagged **0.438.2 terminal-status** cases require an
`error` with a present but invalid primary `status` to remain nonterminal
when secondary `status_code` is 400. The invalid values are the string
`"invalid"`, a fractional numeric `400.5`, and overflowing u16 `65536`;
all three must forward the exact following `response.completed` frame with
**one** upstream attempt. Conversely, `status:65535` is a valid terminal
error and the intentionally delayed second completion may not be forwarded.
Every case checks exact frame bodies, strict same-owner count and negative
controls against status truncation, premature close, replay or leaked
post-terminal output. The original tagged-status gate observed **50** synthetic WebSocket
upstream attempts across both binaries. Two more cases now verify that a
**fresh** connection-limit error in either `code` or nested `error.code`
triggers exactly one new WebSocket attempt against the **same** profile.
The mixed top-level/nested code precedence must not leak the precommit error,
rotate owner, duplicate the user request or replay any committed output.
The expanded gate observes **56** model attempts across both binaries;
quota probes have their own counter. Earlier 58-request totals included two
usage probes. Negative controls still reject wrong retry count, owner,
response ID or wire frame.

The pinned Prodex executable selected account-a on all these synthetic
requests; **this fixture does not prove that soft session IDs always select
account-a**, or that a hard previous-response owner may be discarded.
The harness now rejects mismatched selected owners even if attempt counts
and client event types still match.

Run from a **clean committed Godex checkout**, with a locally built candidate
containing Go VCS metadata:

```sh
go build -trimpath -o /tmp/godex-04383-candidate ./cmd/godex
go run ./tools/continuationparity \
  --prodex /path/to/official/prodex-0.438.3 \
  --prodex-source /path/to/clean/prodex-tag-0.438.3 \
  --godex /tmp/godex-04383-candidate \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)" \
  --websocket
```

The official reference executable must report `prodex 0.438.3`, have
SHA-256 `5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923`,
and correspond to source commit
`42042d765100fc06895a27e0904ffb64842092e6`.
The WebSocket mode additionally rejects a dirty Godex source tree, a
different expected commit, or missing/modified Go executable VCS metadata.

The harness fails on mismatched event order, missing visible output,
extra upstream attempts, leaked precommit response IDs, wrong hard-affinity
owner, missing-chain error type/code/status, rewritten tool outputs,
and invalid framing. Unit negative controls intentionally mutate these
observations. See `scripts/verify-release-parity.sh` for the exact-tagged
release-only invocation. That release script still blocks on *all* other
oracle checks; local scenario passes do not authorize a tag or release.


## Tagged Prodex 0.438.2 completed-idle WebSocket permit oracle

The `--websocket-capacity` flag is a separate published SHA/source-tag
verified two-executable fixture, not a substitute for the original 56-model-attempt
WebSocket continuation differential. Both products receive identical isolated
synthetic profiles and a **weighted hard per-profile execution limit of 2**.
Five distinct local client WebSocket connections each complete a native
Responses turn and then **remain open**; the fixture checks one authenticated
upstream dispatch per turn on `account-a`. A sixth turn on the **first**
client WebSocket must reuse the previously cached upstream transport.
Expected per product: exactly **6 completed turns**, **6 upstream requests**,
**5 upstream handshakes**, no client re-connect, no duplicate dispatch, and
no owner or request-body changes. Keeping five idle connections open makes a
leaked execution permit observable instead of accidentally masking it by
closing every completed client session. Reuse when capacity is again ready
must not throw away session lineage or create a new upstream handshake.

The Go package's `TestProdex04382WebSocketCapacityOracleRejectsMaterialMutations`
requires the fixture to detect lost/extra dispatch, changed owner or model
request bytes, missing/mutated completion IDs, and a sixth handshake. No
real OAuth access or model API traffic is used. The independent test of
**saturated active-turn reuse waiting** remains in Godex's real Router/TCP
integration suite; this cross-executable fixture specifically proves the
completed **idle** resource release/reuse lifecycle.


## Prodex 0.438.2 original-deadline precommit executable oracle

`--websocket-deadline` runs another independent tagged source/SHA-verified
binary differential. Prodex's `recovery_watchdog_budget_ms` computes
`max(stream_idle_timeout_ms, 30_000)`; both executable launchers receive
**2000ms** stream idle, so one client-side `response.create` must time out at
**30 seconds**. The synthetic upstream sends `keepalive` text frames about
245ms apart until roughly 29s, then stays **open and silent** past the
original deadline. This distinguishes the 0.438.2 bounded I/O behavior from
an extra idle timeout granted after the last hidden frame. Neither product
may expose a hidden prelude as committed model output.

The exact client-visible terminal error is `HTTP 504`,
`error.code=request_timeout`, with the exact **client-visible** message `request timed out` (the Router's
longer internal error message is not part of the WebSocket wire). The
fixture asserts that expiry occurred within a 27.5–31s bounded interval;
only one authenticated upstream request was issued, no owner rotation or
replay occurred, and a new turn on the **same local WebSocket** completed on
`account-a` with `response.id=resp-after-watchdog`. One upstream connection
for each turn means exactly two handshakes and two upstream requests.
`TestProdex04382WebSocketDeadlineOracleRejectsMaterialMutations` actively
rejects early/renewed timeouts, wrong status/code/message, duplicate or
missing upstream requests, changed request body, foreign profile owner,
missing followup completion and any extra handshake.

The full `--phase=websocket` release gate retains both WebSocket fixture
families. For synchronous execution the release script provides two
**non-overlapping** slices: `--phase=websocket-core` (56 prior model attempts,
completed-idle permit and mutation tests) and `--phase=websocket-deadline`
(this two-product 30s fixture and negative controls). Both must pass against
the same clean candidate to establish the complete bounded 0.438.3 gate.


## 0.438.2 committed-output WebSocket idle close-frame differential

`--websocket-committed-idle` compares the official SHA/source-pinned Prodex
0.438.3 binary and clean Godex using the **configured** `stream_idle_timeout_ms
= 2000`. Both launch isolated synthetic homes and connect a local Responses
WebSocket to a real loopback upstream that sends one committed
`response.output_text.delta` with `delta:visible-idle`, then remains silent
for up to seven seconds. The tagged Rust `response_tracking/commit.rs` socket
must close at about 2s after this visible output. Both executables must
forward that exact delta and then send an **empty WebSocket close control
frame** (opcode 8) to the client. A TCP EOF without a close frame is NOT
wire-equivalent: it was the concrete Godex mismatch before the scoped patch.

The oracle verifies exactly **one authenticated upstream request**, one
upstream handshake, original opaque model request bytes, account-a ownership,
visible output once, graceful empty close frame, and elapsed 1.5–4.0 seconds.
Negative controls reject silent loss/changed delta, early/late close, a
response.failed frame masquerading as a close, extra handshakes or replay,
owner rotation and any changed request JSON. The transport labels committed
idle expiry distinctly from precommit timeout/cancellation/other upstream
errors; delivery emits a close frame **only** for this committed idle case.
For reused upstream sockets the committed flag is reset atomically when the
next request enters precommit, preventing an old stream timeout from
mistakenly closing/retrying a fresh turn. Local `-race` tests exercise this
state transition. The fixture is now part of the `--phase=websocket-core`
and inherited full WebSocket gate alongside the prior 56 and 12 model attempts.


## Tagged 0.438.2 8KiB held precommit promotion into committed idle

`--websocket-lookahead-idle` is an independent official Prodex 0.438.2
source/SHA-pinned two-executable differential. One freshly opened local
Responses WebSocket sends a single synthetic `response.create`. The upstream
replies with a **precommit-only** `response.in_progress` padded with exactly
8,192 `x` bytes, exhausting the bounded lookahead budget. It then remains
open and silent until after the 2-second configured stream-idle timeout.
The tagged Rust `precommit.rs` promotes the hold to a committed Responses
stream and uses the same `commit.rs` timeout restoration as an ordinary
visible delta. Godex originally left that promoted stream on an 8-second
precommit progress timer rather than the real configured committed timeout.

Both official Prodex and clean Godex must emit one identical full text frame,
then an **empty opcode-8 WebSocket close** in 1.5–4 seconds. The fixture
requires exactly one original opaque upstream request, one handshake, and
unchanged `account-a` ownership. The independent mutation controls reject a
lost/shortened hold, wrong event type, failure to promote, late/early close,
abrupt TCP EOF, unexpected close body, duplicated inference, reconnected
transport, changed owner or rewritten request JSON. The 56-model-attempt core,
12-attempt idle-permit, two-attempt committed visible idle, and separate
absolute 30-second precommit tests remain unchanged and mandatory.

## Native realtime duplex admission and close lifecycle

Before the realtime fixture, the oracle reports raw accepted-socket flags and
idle-read behavior on Linux/macOS. This isolated loopback diagnostic uses a
nonblocking listener and 20 ms receive timeout, and closes its descriptors before
either executable starts. Other hosts report `measured=false`. It helps diagnose
the official macOS reset while keeping every protocol and parity assertion.

`--websocket-realtime` compares four isolated cases per executable: client
close, upstream close, abrupt client cancellation, and graceful gateway
shutdown. Prodex uses the private registry's loopback `realtime_ws_addr`
sidecar; its ordinary HTTP gateway WebSocket listener runs Responses mode.
Godex serves the realtime route on its existing listener. Each case imports
three synthetic profiles, marks two weekly-exhausted, and limits the ready
profile to two weighted inflight units.

The live socket exchanges exact session-update, text, audio-control and binary
audio frames, including a completed realtime model turn. A separate Responses
request must record `profile_inflight_saturated` before any upstream handshake
or model turn. Client/upstream close or cancellation then admits exactly one
waiting turn on the same owner; shutdown must exit successfully and cancel it
without inference. Quota probes have their own counter and cannot affect the
handshake, control, audio, inference or close assertions. Mutation tests reject
altered ownership/authentication/payload, missing or duplicate traffic, bypassed
admission, reconnects and shutdown replay. `--phase=websocket-core` includes
these cases and their race-tested negative controls.

## Published native macOS and Windows executables

`scripts/verify-native-parity.ps1` runs the same six executable fixture modes
on a clean native checkout: continuation, completed-idle capacity, visible
committed idle, lookahead committed idle, realtime lifecycle and the absolute
precommit deadline. The portable CI matrix invokes it after its native build.
The script downloads only exact 0.438.3 assets, checks the platform binary and
manifest against independent fixed digests and published SHA256SUMS, verifies
the clean tagged source and candidate Go VCS metadata, then runs the loopback
fixtures and their mutation controls. A cross-build is not an execution pass.

All operating-system home/config/cache variables and Codex/profile paths are
isolated under the synthetic fixture; ambient provider API keys are removed.
The compiled oracle supplies its own no-op Codex stub, avoiding `/bin/true` on
Windows. Windows gateways receive a private console. A short-lived sender
attaches only to that console and sends its actual Ctrl-C event, allowing the
same graceful shutdown assertions without signalling the CI runner. A forced
kill still fails the realtime shutdown case. Existing Linux release slices
retain all scenarios and additionally verify the adjacent published manifest.

Fixture auth/state files use mode 0600 on Unix and a protected current-user
ACL on Windows, including rewrites. Profile import uses its strict path;
`--insecure` would skip the reference's Windows ACL sealing and leave files
that a subsequent strict gateway refuses. Native file-security code stays
private to this developer tool without widening the Codex gateway API.

Published Windows Prodex import currently leaves managed auth with an inherited
Administrators ACL even with strict input. Runtime fixture preparation seals
those reference files after import; Godex output is not normalized. This
qualifies runtime behavior only and does not certify native CLI import privacy.
All six native modes and negative controls run even if an earlier mode fails;
any failed mode still makes the script fail.

### Scoped 0.438.3 WebSocket transport release phases

The full `--websocket-transport` native/default oracle continues to run all
**20** authenticated two-executable cases, including the original 30-second
deadline and post-commit no-replay controls. For bounded synchronous Linux
release-gate calls, run **both** independent source/binary/manifest-pinned
phases on the *same clean Godex commit*:

```sh
./scripts/verify-release-parity.sh --phase=websocket-transport-core
./scripts/verify-release-parity.sh --phase=websocket-transport-lifecycle
./scripts/verify-release-parity.sh --phase=websocket-transport-capacity
```

The core slice runs the first 17 retry, ownership, quota/application-error and
postcommit scenarios; lifecycle covers cancellation and the original 30-second
deadline, while capacity independently keeps the alternate Ready profile busy
through its own full deadline. Every case retains
the exact upstream owner, authenticated Ready, raw request/history, client
frame, durable-state and negative mutation assertions. The release script also
runs the complete `TestProdex04383Transport*` mutation suite in either phase.
`TestProdex04383WebSocketTransportSlicesExhaustive` rejects omitted,
duplicated or reordered cases. **A single slice is not a release certificate**;
full source, native-platform and external-credential parity must still be
qualified independently.

### Strict WebSocket Compact turn lineage — executable oracle

The pinned 0.438.3 runtime stores a distinct `__compact_turn_state__:` binding
for Compact-produced continuations, and the WebSocket message selector reads
this binding without changing its WebSocket quota route into a Compact route.
`--websocket-compact-lineage` supplies **two** authenticated, Ready profiles,
a verified lineage owned by second and a deliberately active main profile.
It compares the exact one-attempt owner decision, the model input and
turn-state handshake override, two client frames, preserved unrelated owner,
and durable Compact/response bindings on **both** executables. The unbound
negative control uses the same warmup order and request without the lineage,
so both products must select active main. Warm second **before** main; otherwise
the positive result could be explained entirely by the active profile.

A third, **end-to-end producer** scenario starts without a seeded Compact
lineage. It uses a verified second-owned previous response to hard-pin an
actual HTTP `/responses/compact` request. The upstream then returns a
synthetic `x-codex-turn-state` header. Both binaries must persist the
`__compact_turn_state__:` binding, `__compact_session__:` binding and direct
session binding on second *before* any WebSocket request can run. The harness
then commits a main-owned warmup to restore active main and checks that the
new WebSocket turn still selects second, preserves exact history/frames and
leaves those three durable owners intact. The prior-response pin prevents
fresh rotation heuristics from masquerading as Compact lineage creation.

```sh
./scripts/verify-release-parity.sh --phase=websocket-compact-lineage
```

The phase requires a clean committed candidate and the exact official tag,
release manifest and executable SHA pin. The native cross-platform gate also
runs the same mode. This finite fixture proves ownership of **existing**
Compact lineage; the HTTP producer scenario proves creation for its one
synthetic successful Compact response, not every alternate output shape or
live OAuth/provider conditions.

Two additional `--websocket-compact-lineage` controls prove the *presence*
rule for top-level WebSocket `x-codex-turn-state` members independently of
in-process parsing. `compact-shadow-null` and `compact-shadow-blank` each
place a real verified nested `compact-turn` owner on second, leave main active,
and send a top-level invalid direct turn next to that nested value. Both
executables must select main with no upstream turn header, leave the nested
owner unchanged, and return the same two client frames from a single original
request. Focused mutation tests reject cross-owner selection, token leakage,
wire corruption and replay. The complete phase currently includes five
ordered, independent producer/consumer/shadowing scenarios.

The strict WebSocket Compact oracle additionally compares three synthetic
**session-lineage** controls against the pinned official binary: a verified
`__compact_session__:` alias with an explicit upgrade `session_id`, an unbound
alias control and a body-only session ID with the same verified alias. They
keep main active and both profiles Ready, so the selected owner must come
from the actual continuation binding rather than warmup order. The body-only
expectation is based on the executable's observed selection, not on an
assumption about the handshake header. Existing direct session and
previous-response precedence is exercised by Router memory/durable tests.

The Compact-lineage oracle now includes a strict **turn-owner conflict**
negative control. A verified raw `compact-turn` belongs to main while its
verified `__compact_turn_state__:compact-turn` alias belongs to second. The
reference executable must produce a single canonical `503 service_unavailable`
frame with no upstream WebSocket request, keeping both durable owners. A
successful inference by either profile is a parity failure even if all
response bytes happen to look valid. Router regression tests also require
same-owner aliases to remain dispatchable.

The Compact session slice also verifies an explicit-header case where a
direct raw session binding belongs to main but the typed Compact alias belongs
to second. Tagged Prodex selects the direct owner, unlike the hard *turn*
conflict (which fails closed). Successful session-only WebSocket responses
then materialize a direct binding on the selected owner; the oracle waits for
that durable transition before declaring PASS. It must not incorrectly assume
that no raw session binding will be created by the committed response.

The WebSocket Compact lineage oracle also checks a hard **mixed-ID conflict**:
`previous_response_id: resp-main` belongs to main, while the accompanying
`x-codex-turn-state: compact-turn` has a verified Compact lineage on second.
Unlike a reconstructable full-history request with only one failed owner,
this input must produce the official single `503 service_unavailable` error
frame, zero upstream WebSocket handshakes, and unchanged durable owners. Both
valid accounts have applied authenticated Ready receipts, so exhaustion cannot
be explained by missing quota evidence. Paired Router tests enforce the same
boundary for memory and durable stores, with a same-owner positive control.

A WebSocket request hard-pinned by `previous_response_id: resp-main` but
carrying `session_id: session-alias` also has a durable post-commit effect:
main acquires the direct session binding, while a previously verified
`__compact_session__:session-alias` lineage stays on second. The fixture now
checks this owner projection in addition to upstream frame count, exact
history and original alias persistence. A routing-only PASS is insufficient.

For `session-hard-previous`, a verified previous response pins main while a
plain session ID still belongs to second. The official 0.438.3 runtime first
selects main, then records the newly committed direct-session observation as
`__prodex_hard_binding_conflict__`; retaining the stale second-only owner
would be incorrect. The reader maps this conflict sentinel explicitly and
requires it after commit without changing other verified ownership records.

`session-hard-previous` also sends a session-only **second turn on the same
public WebSocket** after the first main-owned response commits a conflicting
direct session observation. Tagged Prodex treats that conflict as an unusable
*soft* preference, not a hard 409, and dispatches once on an eligible fresh
Ready owner (main or second). The fixture checks both delta/completion frames, exact owner,
two total authenticated upstream attempts, the intact original request and
continued durable conflict marker. This must not weaken the independently
pinned hard turn/previous-response conflict cases, which fail before dispatch.
