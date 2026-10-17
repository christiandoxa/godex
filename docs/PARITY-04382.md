# Godex ↔ Prodex 0.438.2 parity checkpoint

**Target:** official tagged Prodex `0.438.2` source commit
`decfed45f884fe2bef20911303beab7b198698b7`. **Published** Linux x86_64
asset `prodex-x86_64-unknown-linux-gnu` SHA-256
`3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f`.
Published `release-manifest.json` SHA-256
`93b5b5f90d59b23f800e47d17127ee7671d8a16ef72c6cabf30edd1093617725`.
These were cross-checked against the official release manifest and SHA256SUMS;
the local pinned official executable reports `prodex 0.438.2`.

The `0.438.1` baseline and its five-phase gate remain documented in
[`PARITY-04381.md`](PARITY-04381.md). A PASS against an older binary does **not**
qualify a newly changed 0.438.2 contract. Do not quietly repin that historical
record or inherit its release certification.

## Tagged release delta

The 0.438.2 release fixes two WebSocket capacity/watchdog regressions:

1. Completed Responses WebSockets release their **weighted execution permit**
   while keeping the reusable upstream socket, profile/response identity and
   turn-state lineage. A reused connection obtains a fresh execution permit,
   and saturation waits without evicting the cached socket. Realtime duplex
   retains its permit until close.
2. Precommit send/read must observe the original request-local deadline.
   Keepalives and other hidden prelude frames cannot extend that budget.
   Late frames cannot commit a timed-out request; expiry is a local typed
   HTTP `504 request_timeout`, not a quota penalty or transport retry. After
   model output commits, the existing committed-stream idle timeout applies
   and postcommit replay remains forbidden.

See the official `docs/release-notes/0.438.2.md` and
`migration/websocket-idle-permit-watchdog-04382.md` at the pinned source tag.

## First local patch — hidden prelude and idle capacity

`TestProdex04382HiddenWebSocketPreludeCannotPromoteIdleDeadline` reproduced
Godex's concrete mismatch: it raised the upstream socket timer from 8 seconds
to 300 seconds on the first **hidden** keepalive frame rather than on actual
committed output. The production patch removes this premature promotion and
keeps the existing committed timer switch after a real non-held frame.
`TestProdex04382RepeatedHiddenPreludeRespectsOriginalDeadlineAndFreshRequest`
uses a real synthetic upstream repeatedly sending metadata while the Router's
original request-scoped watchdog expires. It verifies local 504, no second
attempt, successful fresh request on the original profile and no hidden
prelude delivery.

`TestProdex04382CompletedIdleWebSocketReleasesPermitAndReuseWaits` uses the
real Godex routing/transport stack and two upstream WebSocket connections
against one synthetic ready profile. After a terminal `response.completed`,
its weighted execution permit is released while the upstream socket remains
cached; a second active turn uses that permit. Reusing the first connection
then waits until the second permit is released, retains its cached socket
through saturation and completes without a third upstream handshake. This
confirms that the existing Godex release-on-body-EOF path already covers the
scoped idle-permit behavior; no duplicate resource-owner or permit patch was
introduced. Realtime duplex behavior remains protected by its separate
existing lifecycle tests.

At the time of this initial checkpoint, these were only local loopback/
source-derived regressions. The subsequent **two-executable** idle-capacity
and absolute hidden-prelude deadline oracles below have since VERIFIED these
two **bounded** release behaviors on clean Godex `4126a3e2`. Native Windows/
macOS process execution, live OAuth/provider quota and arbitrary network
failures remain outside the finite verified scope.

## Active release oracle retarget (0.438.1 → 0.438.2)

The current `scripts/verify-release-parity.sh` and all seven executable
oracle code paths now pin **only** the published 0.438.2 tag, manifest,
source commit, Linux SHA-256 and version string. The previous 0.438.1
checkpoint remains in `PARITY-04381.md` and the pre-retarget Git history.
The active gate retains each inherited phase and its mutation controls;
`--phase=preflight-tests` additionally runs all three tagged local 0.438.2
WebSocket permit/precommit fixtures under the race detector.

As always, every pass must be re-established against the **same clean Godex
candidate commit** after re-pinning; old 0.438.1 gate logs cannot authorize
a 0.438.2 parity claim. Release-specific provenance is checked before the
executor attempts any synthetic profile or socket fixture. This retarget is
not itself proof that the full release differential passes.

## Complete oracle preflight without synchronous tool timeout

The initial pinned `--phase=preflight-oracles` attempt emitted a passing real
PTY footer differential but exceeded the command's synchronous 112-second
execution window during subsequent profile cases. **That attempt is NOT a
successful oracle phase.** To retain every original test, three explicitly
non-overlapping execution slices were introduced:

- `--phase=preflight-oracles-pty`: PTY footer, CLI and context oracles.
- `--phase=preflight-oracles-profiles`: complete profile lifecycle including
  three-profile restart, real FD pressure, queue wake and negative controls.
- `--phase=preflight-oracles-history`: independent executable history/replay
  boundary oracle.

The existing `--phase=preflight-oracles`, unsplit `--phase=preflight` and
no-argument full-gate behavior still execute **all three slices**. No source
pin, binary digest, test assertion, negative control, or coverage was removed.
A complete 0.438.2 preflight certificate requires all three independently
pinned slices and preflight-tests to pass on one clean Godex source commit.

## Complete pinned finite release gate — clean `046c15af`

All the following stages exited 0 against **one clean Godex commit**
`046c15af9669c6416d6efdaea70244f0f8201306`, using the exact official
Prodex 0.438.2 source commit
`decfed45f884fe2bef20911303beab7b198698b7`, published executable SHA,
and release manifest pinned above:

| Stage | Result |
| --- | --- |
| `--phase=preflight-tests` | PASS (including three new local 0.438.2 WebSocket regressions under `-race`) |
| `--phase=preflight-oracles-pty` | PASS (official PTY footer, CLI and context executable oracles) |
| `--phase=preflight-oracles-profiles` | PASS (managed-profile lifecycle, synthetic quota restart, Linux real FD pressure, and admission queue wake) |
| `--phase=preflight-oracles-history` | PASS (real executable previous-response/continuation replay boundary tests) |
| `--phase=deepseek` | PASS 30/30, `comparison_differences: []`, mutation controls PASS |
| `--phase=gemini` | PASS 13/13, `comparison_differences: []`, mutation controls PASS |
| `--phase=websocket` | PASS 58 synthetic upstream attempts, strict precommit retry, hard owner, and no committed replay |
| `scripts/check-cross-build.sh` | PASS: Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 and targeted Darwin/Windows test compilation |
| `make verify` | PASS: format, vet, source-size, shuffled Go race tests and native Go production build |

An opt-in Codex 0.163.0 handshake fixture was intentionally **not** counted
in preflight-tests: the previously pinned standalone official binary at the
historical test-only path was absent in this runtime. The script's normal
mandatory preflight executed successfully without that optional override;
Codex's native app-server has separately tested historical evidence, which
is not represented as a new 0.438.2 checksum-verified handshake here.

At the earlier `046c15af` checkpoint, the inherited 58-attempt
cross-executable WebSocket fixture covered only prior retry, status, affinity
and no-replay behavior. The **later** pinned `4126a3e2` release gate, recorded
below, adds independent official executable checks for idle capacity and
absolute hidden-prelude deadline. This historical result is superseded for
those explicitly tested cases, but untested live runtime permutations remain
PARTIAL. Similarly, native
Windows/macOS execution, live OAuth and arbitrary upstream failures are not
certified by cross-compilation or synthetic loopback tests. The complete
bounded **finite synthetic release gate is VERIFIED** for the cases named
above; overall production-wide 1:1 remains **PARTIAL**.


## Production profile inflight hard-limit configuration

Tagged Prodex 0.438.2 exposes
`PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT` in its runtime tuning
parser. Godex previously hard-coded the usecase Router default of eight:
its runtime launcher provided no way to configure the **weighted per-profile
permit limit** separately from the listener/admission request cap. That
omission made the one-ready-profile WebSocket capacity behavior dependent on
an unrelated queue limit. The source-derived regression initially failed to
compile against the missing setting. The production adapter now accepts the
canonical `GODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT` with the tagged
Prodex key as an alias, validates positive bounded integers without silently
coercing zero/negative/overflow, and forwards it through the runtime launch
config to `routing.NewRouter`. No setting preserves the existing default of
eight. The malformed setting is rejected before a real request is sent.

`TestProdex04382ProfileInflightHardLimitEnvironmentMatchesTaggedPolicy`,
`TestProdex04382RuntimeProxyLaunchPropagatesProfileInflightHardLimit` and
`TestProdex04382RuntimeRoutingPreservesConfiguredProfileInflightHardLimit`
protect the complete configuration path with synthetic identities. The
routing composition was extracted to a cohesive 25-line module and the
`cmd/godex/main.go` file reduced from 415 to 397 lines, below the default 400-line guard,
removing its old special baseline rather than raising any threshold. This is **VERIFIED source-aligned configuration
coverage**; the scoped two-executable idle-capacity fixture was
subsequently completed, as shown below.


## Independent two-executable idle-permit capacity proof

`tools/continuationparity --websocket-capacity` adds a new exact-tagged
Prodex 0.438.2 versus Godex differential with five **kept-open local client
WebSockets**, each having completed a turn while only one synthetic profile
is ready. Both binaries run with a real published profile execution capacity
knob set to two (weighted Responses turn = 2). A sixth request on the first
local socket must preserve account ownership and use its cached upstream
connection without another handshake. The assertions require **six exact
completed response IDs and wire-shaped requests**, six authenticated upstream
attempts, and **only five** server handshake connections per executable.
Mutation sentinels detect failed release/reuse, extra/missing turns, owner or
body mutation, and lost or substituted completion IDs. This isolates an
observable 0.438.2 **idle** permit contract independently in both binaries;
active-turn contention, cancellation and session teardown remain covered by
local Go race/TCP tests until their own two-executable negative fixture exists.


## Stream-idle tuning for original request-local precommit deadline

Tagged Prodex 0.438.2 `runtime_proxy::recovery_deadline` obtains a single
precommit deadline from the compiled Mojo
`recovery_watchdog_budget_ms(stream_idle_timeout_ms, 30_000)`. The kernel
computes `max(stream_idle_timeout_ms, 30_000)`; a 1ms/150ms configured
stream idle still gives a **30-second minimum request budget**, while a
longer override can extend the request watchdog. An unset variable retains
the published 300,000ms default (the same existing Godex five minutes).
Godex previously ignored `PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS`
entirely, making a release-configured shorter precommit deadline silently
remain five minutes even while Prodex expired at 30 seconds. The new scoped
settings parser honors the canonical
`GODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS` or compatible `PRODEX_*` key,
rejects zero/signed/overflow durations, and forwards the calculated
**request-local precommit** budget through the actual runtime launcher and
Router. The default, timeout ownership, and postcommit stream rules remain
unchanged; this does not pretend that all other HTTP/SSE stream-idle tuning
has been exhaustively cloned.

`TestProdex04382StreamIdleTuningBoundsOriginalPrecommitWatchdog`,
`TestProdex04382RuntimeLaunchForwardsStreamIdleWatchdog` and
`TestProdex04382RouterReceivesLaunchOwnedPrecommitWatchdog` failed to
compile before the setting existed and now protect floor, override precedence,
validation and the production launch chain. The request's deadline is still
single-owner in Router; hidden prelude keepalive frames are forbidden from
creating a fresh budget. The scoped two-executable **30-second** WebSocket watchdog fixture was
subsequently completed on clean `4126a3e2`, as recorded below.


## Independent two-executable hidden-prelude deadline proof

A new exact tagged `tools/continuationparity --websocket-deadline` exercise
uses **both actual published executables** and isolated synthetic accounts.
With the configured stream idle at 2000ms, the Mojo recovery-watchdog floor
makes the request's precommit budget exactly **30 seconds**. The upstream
emits hidden `keepalive` frames until approximately 29s, then deliberately
holds the accepted socket open past the deadline. This forces the last
blocking precommit read to obey the **original** deadline rather than
accepting a fresh socket-idle window after its last prelude. A local client
must see an exact `504 request_timeout` with the exact public message `request timed out`; there is
one upstream request, no leaked prelude or committed model output, and no
account rotation. A followup request on the same local WebSocket must
complete once on the same profile with its exact ID.

The new negative controls reject an early/late timeout, a 502/429 or wrong
error body, extra/absent authenticated upstream requests, changed owner or
request JSON, missing completion or extra handshake. The original WebSocket
phase still exercises all pre-existing 58 attempts and the 12-attempt
completed-idle permit differential. Two nonoverlapping release slices,
`--phase=websocket-core` and `--phase=websocket-deadline`, preserve the
original complete `--phase=websocket` while meeting synchronous deadlines.
Verification must run both slices on the **same clean Godex commit**; the
old 0.438.1 binary or historical 0.438.2 results do not qualify this
expanded oracle by themselves.


## Latest complete release-pinned finite gate — clean Godex `4126a3e2`

Prodex 0.438.2 published source commit
`decfed45f884fe2bef20911303beab7b198698b7`, Linux x86_64 published
binary SHA-256
`3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f`
and the release manifest were independently verified in every phase.
The Godex source was one **clean exact commit**,
`4126a3e207e2d8ca4f9607b3dbea1ebd74d3c2e8`, for all the following:

| Independent phase | Verified result |
| --- | --- |
| `--phase=preflight-tests` | PASS, including local new WebSocket timeout/permit regressions under Go race tests |
| `--phase=preflight-oracles-pty` | PASS, binary PTY/footer, 14 CLI probes, native context and mutation checks |
| `--phase=preflight-oracles-profiles` | PASS, 34-stage managed profile lifecycle, synthetic quota, real Linux FD pressure and admission wake |
| `--phase=preflight-oracles-history` | PASS, official binary history/continuation replay boundary oracle |
| `--phase=deepseek` | PASS 30/30; `comparison_differences: []`; negative controls PASS |
| `--phase=gemini` | PASS 13/13; `comparison_differences: []`; negative controls PASS |
| `--phase=websocket-core` | PASS: 58 historical upstream attempts **plus 12 new idle-permit attempts**, exact response IDs, owner, body, no extra handshakes and negative controls |
| `--phase=websocket-deadline` | PASS: two official binaries, each returning exact public `504 request_timeout` around **30.0s**, one timed-out inference request, one successful followup on the same local socket, no retry/rotation, strict mutation controls |
| `scripts/check-cross-build.sh` | PASS: Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 plus cross-platform test compilation |
| `make verify` | PASS: format, source-size, vet, shuffled Go race tests and production build |

The complete `--phase=websocket` continues to include both non-overlapping
slices. They are invoked individually to fit synchronous execution budgets;
**a single slice is not a complete release gate**. The Codex 0.163.0
checksum-pinned handshake requires an optional external test binary and was
not included in the current run; its previous evidence is historical only.

**Status:** these finite synthetic 0.438.2 parity cases are VERIFIED. The
full production-wide 1:1 parity is still **PARTIAL**, especially native
Windows/macOS execution, live OAuth and quota/credential recovery, Codex
model compaction with real provider state, runtime realtime duplex edge
cases, and other unexercised failure permutations. Neither synthetic fixture
coverage nor cross-compilation certifies those outstanding native behaviors.


## Configured idle timeout AFTER WebSocket response commitment

Official tagged 0.438.2 `runtime_proxy/websocket/response_tracking/commit.rs`
sets the upstream socket read/write idle timeout to the effective published
`stream_idle_timeout_ms` after the first genuinely committed model output.
Godex previously kept a fixed 300-second committed idle timeout even if the
user explicitly set `PRODEX_RUNTIME_PROXY_STREAM_IDLE_TIMEOUT_MS=250` or
`2000`; its runtime setting only changed the **separate** request-scoped
precommit watchdog (`max(stream idle, 30_000)`). That was a material
behavioral mismatch for an active but stalled Responses stream, despite the
matching precommit timeout in the release oracle.

`TestProdex04382WebSocketCommittedOutputUsesConfiguredStreamIdle`,
`TestProdex04382RuntimeLaunchCarriesConfiguredWebSocketCommittedIdle` and
`TestProdex04382RuntimeOpenAITransportRetainsWebSocketStreamIdleSetting`
initially failed to compile against the missing knob/transport constructor;
after the change, the active committed socket timer reads 250ms/2s when
configured and retains the published 300s default otherwise. The existing
precommit hidden-frame 30s boundary is not changed. The Go transport accepts
the settings at construction (not through a mutable per-turn global), so
parallel requests cannot race to overwrite the timer policy. A separate
fixture with a physically stalled committed stream should verify exact
client-visible timeout/failure behavior of both official binaries before
claiming the **end-to-end** stalled-stream expiry VERIFIED. HTTP/SSE stream-idle
transport tuning is also not certified by this WebSocket-specific fix.


## Graceful close control on configured committed WebSocket idle expiry

An additional **exact two-executable** test exposed a client-visible 0.438.2
wire mismatch after configuring the published stream idle to **2000ms**.
Both Prodex and Godex correctly forwarded a single visible
`response.output_text.delta` and ended the stalled committed upstream read
at about 2 seconds. However, official Prodex sent a WebSocket **close
control frame** (`opcode=8`, empty payload), whereas Godex abruptly dropped
the client TCP connection and returned EOF. The latter lacks the protocol
close handshake and is not behavioral parity for native Codex clients.

Godex now distinguishes a timer-expired **committed** upstream read from
precommit expiry, arbitrary transport failures, and user cancellation via an
explicit model-layer sentinel. The WebSocket transport propagates that
sentinel through the response body; delivery translates *only* that case
into the official empty close control frame before orderly session teardown.
The watchdog's committed/precommit transition is atomic; recycling a completed
upstream socket clears the prior committed flag, so a precommit deadline in a
new turn cannot be mislabeled as the previous stream's committed idle expiry.

`TestProdex04382CommittedIdleTimeoutIsTypedWithoutReplaying` and
`TestProdex04382RecycledSocketClearsCommittedIdleOwnership` validate the
separate clock owners under the race detector. The independent pinned
`--websocket-committed-idle` differential now checks the exact visible delta,
**empty opcode-8 control frame**, the 2s elapsed idle bound, one authenticated
upstream request/handshake, original owner/body and no postcommit replay.
Mutation sentinels intentionally reject abrupt EOF, lost output,
wrong control frames and duplicate attempts. This makes the bounded
**configured committed-idle WebSocket expiry behavior VERIFIED** against two
executables, without treating untested live provider/realtime stream failures
as a production-wide 1:1 certification.


## Precommit 8KiB lookahead promotion uses the committed timeout owner

Tag `0.438.2` also promotes fresh Responses WebSocket precommit hold frames
when they exhaust the `RUNTIME_PROXY_SSE_LOOKAHEAD_BYTES` (8KiB) budget, even
without a visible model text delta. The Rust
`runtime_proxy/websocket/response_tracking/precommit.rs` promotion then
enters the same `response_tracking/commit.rs` path that restores the
configured committed upstream idle timeout. Godex previously returned a
committed response from this **8KiB promotion branch** but accidentally kept
its fixed **8-second precommit socket timer**, instead of the configured
committed 2-second or default 300-second idle clock. This could falsely
terminate a promoted stream early or delay an idle stop beyond its configured
stream budget. It also misclassified a subsequent read timeout's ownership.

`TestProdex04382LookaheadPromotedWebSocketUsesConfiguredCommittedIdle`
reproduced both timer and committed-state mismatches before the fix, then
passed after installing the same socket-commit timeout transition used by a
regular visible frame. Its negative control
`TestProdex04382HiddenWebSocketPreludeCannotPromoteIdleDeadline` still proves
that a short (under-8KiB) hidden prelude cannot promote or reset the original
request deadline. The complete local OpenAI package and focused Go race tests
PASS. This checkpoint is source-derived plus local physical WebSocket proof;
a new **two-executable** lookahead idle fixture is a distinct next step.


## Independent two-executable 8KiB lookahead promotion idle proof

Following the fail-before/pass-after 8KiB commit-clock fix, a new pinned
**official Prodex 0.438.2 versus Godex** WebSocket differential verified
its actual public wire. The synthetic upstream writes a
`response.in_progress` frame containing **8,192 bytes** of padding and
keeps the connection silent/open afterward. Both executables promote the
bounded hold to committed output, forward the complete frame exactly once,
and send an **empty close control frame (opcode 8)** around **2.0 seconds**
when `STREAM_IDLE_TIMEOUT_MS=2000`. Both make exactly **one** authenticated
upstream request with unchanged request body, account owner and handshake.

`tools/continuationparity --websocket-lookahead-idle` is independently
source/SHA-verified and now part of the release `--phase=websocket-core`
without weakening the prior continuation, idle-capacity, normal-visible
committed idle or absolute-precommit tests. The mutation sentinels reject
missing/altered padding, incorrect output type, missing close handshake,
wrong early/late clock, extra upstream attempts, changed account or request.
This **bounded 8KiB promotion/idle contract is VERIFIED across both
executables**, not an assertion that every native Codex streaming state or
platform-specific socket expiry has been certified.

## Latest complete 0.438.2 finite gate — 8KiB promotion and idle close

All mandatory finite release phases exited **PASS** on the exact same clean
Godex commit `aeb90b98db61aadea54b23986c84bc711f67286b`, against the
official Prodex `0.438.2` source
`decfed45f884fe2bef20911303beab7b198698b7`, published Linux x86_64
binary SHA-256
`3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f`
and pinned published release manifest. No actual personal OAuth credentials
or live external model requests were used.

| Individually pinned phase | Result on clean `aeb90b98` |
| --- | --- |
| `--phase=preflight-tests` | PASS, including real Go TCP/WebSocket deadline, 8KiB precommit promotion, committed timeout, graceful control-frame close, recycled-socket phase-reset regressions, and native-format mutation controls |
| `--phase=preflight-oracles-pty` | PASS, official two-executable PTY footer, 14 CLI probes, and context/negative oracles |
| `--phase=preflight-oracles-profiles` | PASS, 34 managed-profile stages, synthetic quota/auth recovery, real Linux descriptor saturation and admission wake |
| `--phase=preflight-oracles-history` | PASS, two-executable history/continuation and replay boundary controls |
| `--phase=deepseek` | PASS **30/30**, `comparison_differences: []`, negative controls PASS |
| `--phase=gemini` | PASS **13/13**, `comparison_differences: []`, negative controls PASS |
| `--phase=websocket-core` | PASS, original **58** continuation upstream attempts + **12** completed-idle permit attempts + **2** visible committed-idle graceful closes + **2** promoted 8KiB precommit committed-idle graceful closes = **74 total synthetic upstream attempts across both executables**, plus per-fixture negative controls |
| `--phase=websocket-deadline` | PASS, **4** upstream attempts across both binaries: one absolute 30-second hidden-prelude `504 request_timeout` and one successful subsequent turn on the same local WebSocket per binary |
| `make verify` | PASS, format, source-size, vet, complete shuffled Go race suite, production build |
| `scripts/check-cross-build.sh` | PASS, Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 builds plus targeted Darwin/Windows test compilation |

Both **new** bounded 0.438.2 lifetime contracts have real two-executable
wire evidence: a committed visible delta and a separately promoted **8KiB**
`response.in_progress` frame each trigger an empty WebSocket close control
frame (`opcode=8`) after about **2.0 seconds**, with no extra owner,
handshake, inference or lost client frame. Below the hold-promotion budget,
hidden prelude frames do **not** reset the **original** 30-second precommit
deadline, and late frames cannot cause another inference or profile penalty.
The previous 58-attempt and 12-attempt WebSocket fixtures remain mandatory.

The Codex 0.163.0 official app-server handshake is an **optional** fixture
that requires a separately verified binary which was not available at its
historical location; it was not executed or counted PASS in this run. Do not
interpret cross-build as native Windows/macOS execution, or bounded synthetic
release-gate success as complete production 1:1. Native platform lifecycle,
real OAuth/multi-profile quotas, foreground/background Codex compaction,
realtime duplex edge cases and unexercised upstream failure permutations
remain **PARTIAL/OPEN**. These finite tested contracts are **VERIFIED**.


## Native realtime duplex permit held until close — local Router/TCP proof

The tagged Prodex 0.438.2 Rust caller test
`websocket_realtime_connection_retains_its_live_duplex_permit` asserts that
a `/live` realtime connection **does not** release its weighted execution
permit when it receives or forwards frames, unlike a completed Responses
turn. Godex's local
`TestProdex04382RealtimeDuplexRetainsWeightedPermitUntilClose` now uses
a real Router, a single synthetic ready profile with hard inflight capacity
of two, and a duplex `net.Pipe` gateway. It verifies a live session takes
weight **2**, forwards a real synthetic model-output text frame without
releasing any permit, and blocks a competing Responses request **before**
upstream dispatch. Closing the realtime session releases the exact permit,
wakes the waiting request, permits it to dispatch once on the same owner,
and leaves the inflight count at zero with two admissions, two releases and
**zero** underflows. The fixture runs repeatedly under Go's race detector
and does not use real accounts, external services or private tokens.

A separate exploratory *two-executable* realtime `/live` loopback probe was
not considered authoritative: the official Prodex 0.438.2 process was also
conducting internal `wham/usage` probes while the test was trying to infer
admission from total backend request count; its close handshake did not
complete in the synthetic fixture's short timeout. That exploratory test
was discarded without patching runtime behavior or altering the existing
58/12/2/2 WebSocket differential assertions. An independent deterministic
**two-executable realtime permit/close** fixture is still **PARTIAL/OPEN**,
not silently inferred from the local Router test.

## SHA-pinned official Codex 0.163.0 native app-server and compaction proof

An additional optional preflight coverage gap was closed on clean Godex
`39580286e1db65ae330a6d5db76489526b28cf16`: the locally installed
`codex` launcher was a small Node.js wrapper, but its private npm native
package `@openai/codex-linux-x64` contained the **unmodified Linux musl
Codex 0.163.0 executable**. The executable's independently verified SHA-256
`399ff69c431c066e40de318fba45c2b07f45a76b19939d247ee4828fc70c4215`
matched the source-pinned `officialCodex0163LinuxSHA04380` test oracle, and
it reported `codex-cli 0.163.0` as required. No installation was changed.

Opting in with `GODEX_TEST_CODEX_0163_APP_SERVER_BIN` targeting that
verified native binary produced **PASS** for all six official-binary tests:

- `TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList`
- `TestProdex04380OfficialCodex0163LoopbackForegroundCompactionRetainsHistory`
- `TestProdex04380OfficialCodex0163LoopbackAutoCompactionLifecycle`
- `TestProdex04380OfficialCodex0163LoopbackAsyncCompactionLifecycle`
- `TestProdex04380OfficialCodex0163LoopbackIncompleteUsageDoesNotReplay`
- `TestProdex04380OfficialCodex0163LoopbackInterruptCancelsWithoutReplay`

The **entire** tagged `--phase=preflight-tests` (including new realtime
weighted permit regression) then **PASS** on the same clean
`39580286` with that SHA-pinned optional binary enabled. Earlier checkpoint
reports saying the optional official binary was absent described the
*historical fixture path*, not a permanent platform limitation. These are
real native Codex 0.163.0 app-server sessions with an isolated, synthetic
loopback provider and credential-free HOME. They do **not** prove live
external-provider compaction, full production quota state, or native
Windows/macOS app-server execution; those remain PARTIAL/OPEN.
