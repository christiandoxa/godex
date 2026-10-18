# Godex ↔ Prodex 0.438.2 parity checkpoint

Latest qualification: [Windows native qualification and first-message calibration](#windows-native-qualification-and-first-message-calibration).

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

## Native realtime two-executable permit and close lifecycle

**VERIFIED, bounded synthetic Linux scope.** The independent
`tools/continuationparity --websocket-realtime` fixture now passes against
the SHA-pinned official Prodex 0.438.2 executable and clean Godex baseline
`ea1ecce83f6c703f61b9d3c6ccebb923be77defb`. Four isolated cases per product
cover client close, upstream close, abrupt client cancellation and successful
gateway shutdown. No production mismatch was demonstrated, so no speculative
permit or realtime transport patch was made.

The tagged source explains the earlier non-authoritative fixture failure:
`runtime_proxy/dispatch/websocket.rs` calls the HTTP listener's session loop
with `realtime_duplex=false`, whereas `runtime_proxy/realtime_sidecar.rs`
uses `true` and a bounded duplex pump. `runtime_broker/live_source.rs`
publishes its separate loopback `realtime_ws_addr`. The new fixture resolves
that actual native endpoint from the isolated runtime registry. Connecting
`/live` to the ordinary Prodex HTTP listener reproduced an audio-control
timeout before this fixture correction; connecting the advertised realtime
sidecar passes. This was fixture evidence, not a Godex production failure.

Each case uses one ready profile, two weekly-exhausted profiles and hard
weighted capacity two. Session/text/audio/binary frames and one realtime
model completion must preserve the exact wire. A competing Responses socket
must emit a production `profile_inflight_saturated` marker while the backend
still observes zero Responses handshakes and inference dispatches. Closing
either realtime peer or cancelling its TCP connection releases capacity for
exactly one waiting turn, preserving `account-a` and the original body.
Shutdown instead exits successfully, closes the upstream and cancels waiting
work without a second inference. Each case has exactly one realtime handshake,
two controls, one binary audio frame, one live model turn and one closure.
Usage probes are separately counted (one per Prodex case, zero per Godex case
in the observed run) and never classified as inference.

`go test -race ./tools/continuationparity ./internal/usecase/routing -run
'^TestProdex04382Realtime' -count=5 -timeout=45s` passes. The Router regression
also closes the old live exchange again while the follower holds capacity:
the follower keeps weight two, then final accounting is two admissions, two
releases and zero underflows. Negative controls reject changed owner/auth/
request bytes, lost/duplicate live or waiting traffic, missing/double close,
reconnect, bypassed waiting and shutdown replay. The two-executable fixture
also passed when run under Go's race detector. It is now mandatory in
`--phase=websocket-core`; a complete fresh release gate remains a separate
same-clean-commit requirement. Native Windows/macOS execution, live OAuth,
real audio inference and arbitrary production failures remain PARTIAL/OPEN.

## Fresh complete finite gate with native realtime — clean `97cd030a`

All eight release phases, `make verify` and the explicit cross-build script
PASS on clean `97cd030aab45d3b85bd09e655a3f23d9d164aae0`, accepted by remote
`main`. The SHA-verified native Codex 0.163.0 fixture runs all six cases.
DeepSeek is 30/30 and Gemini 13/13 with zero differences. WebSocket core keeps
74 prior attempts plus eight live turns and six admitted realtime followers;
deadline retains four attempts and exact 30-second 504 errors. Negative
controls PASS. This supersedes the earlier finite gate for its named cases.

## Recent session workspace trust — reproduced production gap

Tagged `prodex-session-store/src/recent_workspaces.rs` and
`mojo/prodex_core/session_selector.mojo` select newest initial session CWDs,
preserve discovery order on mtime ties, skip relative paths, deduplicate exact
strings and cap selection at 4,096. `runtime_tools/super_trust.rs` injects
these alongside the current workspace as temporary Codex project overrides.
Godex previously injected none. `tools/workspace-oracle.py` confirms the
official binary's current plus four expected recent workspaces, then fails
the clean pre-fix Godex executable with `expected exactly one temporary
projects override`. Its negative controls reject missing, duplicate, changed,
untrusted or malformed project entries. Manifest, source, binary and clean
candidate VCS provenance are mandatory; no real credentials/model calls occur.

The implementation uses session entity selection, session repository metadata
and alias reads, runtime use-case orchestration and composition-root wiring.
Focused Go race tests PASS for ordering, limits, initial versus legacy JSON
metadata, archive inclusion, cancellation, aliases, physical PWD, escaping,
user override precedence and direct Super dispatch. The reader retains bounded
I/O (65,536 directory entries, 4 MiB/file, 512 KiB/line); a failed optional
history scan keeps current-workspace trust. The 128 KiB argument bound and
saved profile configuration remain intact. The independent Python oracle is
mandatory in the PTY release slice; fresh clean-candidate qualification follows
the scoped implementation commit. Native Windows/macOS execution and histories
beyond the documented scan bounds remain outside this proof.

## Imported workspace names and native Windows assertion

The extended oracle reproduced another concrete gap on clean `aabb99b0`:
tagged `session_file_name_policy_v1` accepts every `.json`, `.jsonl` or
`.jsonl.zst` metadata filename, whereas Godex's reused catalog filter required
`rollout-`. Prodex selected the synthetic `imported-session.jsonl` workspace;
Godex omitted it. The repository regression failed with that missing path and
the official two-executable oracle rejected Godex's workspace list. Workspace
discovery now uses the tagged suffix rule while retaining regular-file,
symlink, byte, cancellation and directory-entry bounds. The session catalog's
existing filename contract is unchanged.

Native CI run [39112394206](https://github.com/christiandoxa/godex/actions/runs/39112394206)
on `aabb99b0` passed Linux race tests and the macOS portable test/build lane,
but failed the new Windows workspace test. Its production TOML correctly
escaped backslashes; the test incorrectly searched the encoded value for a raw
Windows path. A synthetic backslash path reproduces the same failure on Linux.
The assertion now parses TOML and checks one exact trusted workspace key,
preserving its original invariant. Focused local race tests PASS. Historical
run [39110921049](https://github.com/christiandoxa/godex/actions/runs/39110921049)
on `97cd030a` passed both native portable lanes. Neither result is a native
two-executable Prodex-versus-Godex differential or live-provider certificate.

## Non-WebSocket 0.438.1..0.438.2 delta audit

**Gemini timing: VERIFIED source scope; bounded public behavior VERIFIED.**
The exact tagged `provider_constraints.mojo` was compiled locally with Mojo
1.1.0. An independent ctypes check passed 524 combinations: retry indices
0–8 use 5/10/20/30-second defaults, the maximum of header/body/default is
capped at 300 seconds, index 9+ stops, inline accepts only `(0, 30000]` ms,
and invalid-stream backoff is `1000 * 2^min(index, 8)` ms. Tagged native
callers retain their three-attempt invalid-stream limit. This proves the
reference kernel, not an unimplemented Go OAuth runtime. Gemini OAuth/native
CLI routing is disabled in this release. Public API-key launches use the
separate `local_rewrite_gemini_openai.rs` Chat-compatible path. Four loopback
cases per official executable and clean Godex `aabb99b0` confirm: plaintext
429 preserves status/body/Retry-After with one attempt; structured 429 with
5s or 301s Retry-After advances immediately to the next model and succeeds
with exactly two attempts; an unrecognized SSE stream ends with one attempt.
No native inline sleep/backoff was copied into the different public path.

**DeepSeek parameter reporting: VERIFIED source scope.** Compiling the tagged
`openai_compat` kernel and calling its reporting operation produces the exact
five ordered fields and reasons in `translators/deepseek/supported_params.rs`.
They match the pre-0.438.2 Rust report byte-for-byte; the release moved ownership
without changing the report. Production CLI code has no caller of that
provider-core reporting trait; its only external caller in the source tree
is the developer `provider-contract-matrix` example. Runtime bridge validation
is separate: existing Go tests cover false parallel controls, text-only input,
function tools, safety/user mapping and web-search validation; the pinned
30-scenario executable gate checks observable request/error behavior. No new
Godex reporting API or stricter runtime rejection was invented from descriptive
report entries.

**Native Codex/platform limits: PARTIAL.** Six Linux SHA-verified native Codex
0.163.0 app-server cases cover foreground/automatic/asynchronous compaction,
incomplete usage and interrupt without replay. Native CI exercises Go platform
tests and builds, with the Windows assertion issue explicitly recorded above.
Published macOS/Windows Prodex-versus-Godex realtime/process differentials,
real OAuth renewal, real provider quota/audio/compaction and arbitrary network
failures are still OPEN. Synthetic accounts cannot establish those live
contracts. Overall production-wide 1:1 remains PARTIAL.

## Malformed workspace JSON cannot create a substituted trusted path

The extended two-executable workspace oracle reproduced a Unicode boundary
gap on clean `43fe307e`: an initial metadata CWD containing an unpaired
`\uD800` escape is rejected by tagged serde_json, but Go previously replaced
it with U+FFFD and added the substituted path to the trusted workspace list.
The official binary kept five expected recent workspaces; Godex's additional
path failed the exact list assertion. The focused repository regression also
failed before the fix on that same lone surrogate.

The existing DeepSeek Unicode/depth validator now lives in stateless
`helper/jsonutil`, consumed by both the DeepSeek gateway and session repository.
There is one implementation, with the original 127-container bound, valid
surrogate-pair and escaped-backslash behavior preserved. Session metadata also
uses Go's finite-number JSON decode before extracting any path, matching the
tagged rejection of overflow numbers. Focused tests reject high/low unpaired
surrogates, excessive nesting, malformed UTF-8 and overflow; valid surrogate
pairs and literal escaped `\\uD800` paths remain accepted. DeepSeek's stable
public malformed-tool error and terminal SSE behavior are unchanged. No
credential, saved session or profile config is rewritten.

## Native Windows ownership fixtures retain assertions with owned I/O budgets

Native run [39112990057](https://github.com/christiandoxa/godex/actions/runs/39112990057)
on `43fe307e` passed the corrected workspace assertion and macOS portable lane.
Windows then failed two existing proxy fixtures: first-conversation owner
serialization exhausted its five-second context, and the six-turn owner
precedence test exhausted one shared two-second socket deadline on its second
turn. These fixtures assert ownership and dispatch counts, not storage latency.
The serialization test keeps its deterministic channel barriers with a bounded
30-second filesystem budget. The WebSocket fixture now assigns a bounded
10-second I/O budget to each turn. Every original account, route, binding,
attempt-count and no-replay assertion remains present; no test was skipped or
weakened to accept a wrong owner. This changes test observation budgets only,
not production admission/recovery deadlines or provider retry timing.

## Fresh full finite gate — clean `700183ba`

All eight mandatory release phases PASS on clean
`700183ba4272171d1ef340daba57786e65961981`, with the SHA-pinned native Codex
0.163.0 option enabled. `rtk test make verify` and the separately executed
`./scripts/check-cross-build.sh` PASS. The independent workspace oracle now
includes imported filenames and malformed-surrogate metadata; both official
executables produce current plus exactly five expected recent workspaces.
DeepSeek remains 30/30 and Gemini 13/13, zero differences, negative controls
PASS. Realtime client/upstream close, cancellation and shutdown remain mandatory
alongside the prior WebSocket 74-attempt core and four-attempt deadline proof.

The exact phase commands executed on that same clean candidate were:

```sh
./scripts/verify-release-parity.sh --phase=preflight-tests
./scripts/verify-release-parity.sh --phase=preflight-oracles-pty
./scripts/verify-release-parity.sh --phase=preflight-oracles-profiles
./scripts/verify-release-parity.sh --phase=preflight-oracles-history
./scripts/verify-release-parity.sh --phase=deepseek
./scripts/verify-release-parity.sh --phase=gemini
./scripts/verify-release-parity.sh --phase=websocket-core
./scripts/verify-release-parity.sh --phase=websocket-deadline
```

Native CI [39113607312](https://github.com/christiandoxa/godex/actions/runs/39113607312)
on `700183ba` confirms Windows portable tests/build and both security lanes
PASS. The macOS lane failed the existing foreground cancellation fixture:
its native readiness marker existed, but its descendant was already gone
before cancellation. That test helper blocked forever in an unowned empty
`select {}`. It now explicitly waits for its owned termination signal and
reaps the native child, keeping the same process-group, descendant-liveness,
cancellation and unrelated-process assertions. This is test lifecycle cleanup,
not a production process-policy patch. A focused non-race Linux repeat of the
old case passed 40 times; the native macOS failure is recorded rather than
misrepresented as a locally reproduced production bug. A new native CI run
is required to qualify the corrected fixture. Full production-wide parity
remains PARTIAL at the live-provider and native differential boundaries above.

## Final source-qualified checkpoint — clean `81adbb06`

**VERIFIED finite synthetic gate.** Every exact phase command listed above
was re-executed on the same clean
`81adbb064e9c16e84be010f20f422e687a8415f7`. All eight phases PASS, including
the SHA-verified native Codex 0.163.0 preflight option, workspace mutation
oracle, 30 DeepSeek cases, 13 Gemini cases, eight realtime close/admission
cases, the 74-attempt inherited WebSocket core and four-attempt absolute
deadline proof. Both provider reports have `comparison_differences: []`;
negative controls PASS. `rtk test make verify` and the separately executed
`./scripts/check-cross-build.sh` PASS for the final code. No dependencies,
versions, tags or releases changed.

Native [CI run 39114142898](https://github.com/christiandoxa/godex/actions/runs/39114142898)
also PASS on this exact commit: Linux shuffled race suite/build, macOS portable
tests/build, Windows portable tests/build/installer smoke, SonarQube and KICS.
This resolves the recorded Windows test-budget and macOS helper-lifecycle
failures without dropping assertions. Pushes occurred after local validation,
before waiting for CI, and remote `main` accepted `81adbb06`.

**PARTIAL overall parity:** native Go CI is now real execution evidence,
but it does not compare the official macOS/Windows Prodex executables or run
live external authentication/model traffic. Those platform differentials,
real OAuth refresh, actual multi-account provider quotas, real audio/model
compaction and arbitrary failure permutations remain OPEN. No personal
credentials were accessed and no live inference was performed. The primary
dirty checkout remains on its original local commit with its WIP preserved;
all changes were made in isolated worktrees.

## Native official-executable platform oracle — qualification in progress

**PARTIAL until the named native runs complete.** The preceding portable CI
lanes execute Godex tests on Windows/macOS but do not compare the corresponding
published Prodex executable. `scripts/verify-native-parity.ps1` now pins the
published 0.438.2 platform assets and manifest, clones the exact tagged source,
builds a clean Godex candidate and reuses all six continuation/WebSocket modes.
The portable CI lanes run this same script on their actual native platforms.

Before the fixture correction,
`TestProdex04382OracleIsolatesNativeHomeAndCodexExecutable` failed because the
oracle inherited an ambient `USERPROFILE`; its Codex command also depended on
Linux `/bin/true`. The environment now isolates native user/config/cache roots,
strips ambient provider keys and supplies a compiled no-op Codex child. Windows
gateway shutdown uses a private console and its real Ctrl-C event, with a
separate attached sender. Microsoft's [console control contract](https://learn.microsoft.com/en-us/windows/console/generateconsolectrlevent)
does not target Ctrl-C to a nonzero process group; the fixture therefore
isolates the console instead of broadcasting into the CI runner. Forced kills
remain failures of the realtime graceful-shutdown assertion.

`go test -race ./tools/continuationparity -count=1 -timeout=90s` PASS: 106 tests,
including native-home isolation, all six platform pins, unknown-platform
rejection and missing/mutated-manifest rejection. Windows test cross-compilation
PASS. PowerShell was absent locally; an official 7.4.6 tarball was downloaded
to a private temporary directory and its published SHA256SUMS verified before
the script parser PASS. No host installation or production dependency changed.
Actual native executable results are recorded only after CI execution; live
OAuth, real model inference and production quota claims remain outside scope.

Before committing the platform harness, all six modes PASS locally against
clean Godex `0d7ed766f9d6199c162d8d4bd3cdfcbafd51fbe6` and the SHA-verified
official Linux executable: continuation 58 attempts, idle capacity 12,
committed idle 2, lookahead idle 2, all four realtime closure paths, deadline 4.
`rtk err make verify`, `git diff --check`, the PowerShell parser and the
script's dirty-candidate rejection self-check PASS. The focused
`rtk go test -race ./tools/continuationparity -run '^TestProdex04382' -count=1 -timeout=45s`
also PASS (67 tests). These preserve the Linux baseline; they do not substitute
for the forthcoming native platform runs.

## Canonical native fixture roots and first platform failures

On clean `f90c7a9b8a9d518b0312f334d63136726fc940ca`, all eight Linux
`verify-release-parity.sh` phases PASS again, including the six SHA-verified
Codex 0.163.0 app-server tests, DeepSeek 30/30, Gemini 13/13 and realtime.
The complete `rtk err /tmp/godex-parity-pwsh-drWiP0/pwsh -NoProfile -File ./scripts/verify-native-parity.ps1`
also PASS on Linux. This uses a temporarily unpacked, verified PowerShell;
it does not install or modify the host's tools.

The first official-native [CI run](https://github.com/christiandoxa/godex/actions/runs/39115337465)
passed Linux and both security lanes. Native Go tests/build passed on both
portable platforms, but the new executable differential did not: macOS
rejected its `/var`-aliased fixture auth path as an invalid secret location;
Windows reached the gateway and returned 503 before any model dispatch.
Neither failure is counted as native parity. The Windows cause is still
under investigation; production routing has not been patched from that
unexplained reference-side result.

`TestProdex04382OracleCanonicalizesNativeTempRoot` reproduced the macOS
fixture defect on Linux with an explicit temporary-directory symlink and
failed before the fix. Fixture creation now canonicalizes its owned root
before constructing any secret/profile paths; reference symlink checks stay
enabled. All four realtime closure differentials then PASS through a
synthetic symlinked TMPDIR with private ancestors on Linux. An initial local
probe with a group-writable ancestor correctly failed the reference security
check and is not reported as a parity pass.

Native failure diagnostics now read only bounded fixture runtime-log files,
using the existing credential redactor; a regression rejects exposed bearer
values and excessive file/output counts. `rtk go test -race ./tools/continuationparity -count=1 -timeout=90s`
PASS: 108 tests. The changes remain developer tooling, with no new production
package, helper, dependency, persistent format or CLI contract.
`rtk err make verify`, `git diff --check` and the PowerShell parser also PASS.
The native script retains all six modes and executes realtime first so a
continuation failure cannot hide that higher-priority lifecycle result.

## Strict native fixture imports and protected file permissions

The second [native run](https://github.com/christiandoxa/godex/actions/runs/39116381897)
on `10d522488f1a165780324ce7115b4d8de1206bdd` passed native Go tests/builds,
but both official-executable realtime attempts failed before successful
upstream model traffic. Windows diagnostics now identify
`auth_not_quota_compatible` and an invalid secret location reading the managed
profile's auth file. macOS passed the formerly rejected import path but logged
private-state read/write failures and a realtime-sidecar connection reset.
Neither is a native parity pass.

Tagged `prodex-secret-store/src/secure_file/windows.rs` requires protected
owner ACLs for private files and deliberately skips sealing when the import's
`--insecure` guard is active. The fixture had relied on Unix-only creation modes
and imported under that guard. It now seals all auth/state writes explicitly
with mode 0600 or a protected current-user Windows ACL, and all three profile
imports use strict validation. No security exception is passed to the gateway.

`TestProdex04382FixtureRewriteRestoresPrivatePermissions` failed for both auth
and state before the correction: `os.WriteFile(..., 0600)` preserves the old
0644 mode when the file already exists. Its corrected implementation PASS;
the Windows regression separately inspects the owner, protection bit, ACE
count and granted principal for both file kinds. Windows test cross-compilation
PASS; actual protected-ACL execution awaits the next native CI run.
`rtk go test -race ./tools/continuationparity -count=1 -timeout=90s` PASS:
111 tests. All four strict-import realtime closure differentials PASS against
both Linux executables using clean Godex `10d52248`.

The small platform-specific sealing code remains inside the oracle: it seals
owned, already-written fixture paths, while the production Codex gateway's
private helper owns a live file handle. No application API or shared helper
was expanded for these developer fixtures, and no production behavior changed.

The complete six-mode strict-import Linux differential PASS: continuation
58 attempts, completed capacity 12, committed idle 2, lookahead idle 2,
all four realtime closes and absolute deadline 4. `rtk err make verify`
and `git diff --check` PASS. All eight release phases also PASS on the
unchanged clean `10d52248` baseline with the verified Codex app-server opt-in.
The corrected Windows/macOS executable runs still require native execution;
the preceding native failures remain explicitly recorded above.

## Native runtime boundary after strict imports — still PARTIAL

All eight Linux release phases and the full native script PASS on exact clean
`afc37dfa1a4f8ae7f970258861c37f1f972bf818`, retaining the SHA-verified native
Codex tests and every differential negative control. In the corresponding
[native CI run](https://github.com/christiandoxa/godex/actions/runs/39117016361),
Linux, SonarQube and KICS PASS. Windows/macOS Go tests and builds also PASS;
this includes actual execution of the protected-owner Windows ACL regression.
The official native executables still fail the first realtime scenario:
Windows rejects managed auth during runtime; macOS records private-state
failures and resets its realtime client. Strict import itself succeeds.
The preceding fixture corrections therefore do not establish native parity.

The next diagnostic checkpoint records bounded file/directory metadata,
including native Unix UID/mode data and Windows security descriptors, without
reading auth contents. It examines at most 12 ancestors and 32 fixture entries;
Windows descriptors are capped at 512 characters. A regression creates 40
synthetic profiles and verifies the metadata bound and absence of secret-file
content. This is evidence collection, not a routing, permission-validation or
reference-binary workaround. No assertion, checksum or pinned artifact changed.
`rtk go test -race ./tools/continuationparity -count=1 -timeout=90s` PASS:
112 tests. Windows test cross-compilation, `rtk err make verify` and
`git diff --check` PASS.

## Reference import ACL defect and separate WebSocket quota observations

Native [metadata run](https://github.com/christiandoxa/godex/actions/runs/39117651872)
on `57f022c9d7e72ee3465925e9faa851f88e89ec11` confirms: macOS auth/state
files have mode 0600 and the runner's UID; private directories have mode 0700
and trusted ancestors. Its realtime reset remains unexplained. Windows raw
fixture inputs have a protected current-user ACL, but the official import's
managed auth files have owner Administrators and inherited, unprotected ACLs.
The runtime's private-file rejection is therefore justified; it is not evidence
that a Godex routing policy should change.

Runtime fixture preparation now seals only the reference's managed auth after
import, preserving bytes. Godex output is not repaired. Native CLI import
privacy remains outside this runtime oracle's verified scope and its observed
reference defect is retained here. No gateway receives `--insecure`.
`TestProdex04382ReferenceFixtureSealsManagedAuthAfterImport` failed before this
correction with an intentionally readable imported file.

`TestProdex04382WebSocketBackendSeparatesQuotaProbesFromModelTurns` also failed
before correction: two usage probes and one WebSocket turn were recorded as
three model attempts. WebSocket fixture mode now records quota probes
separately and retains exact handshake/inference/owner/wire assertions. The
same continuation traffic is now reported as 56 model attempts plus two quota
probes, rather than 58 mixed requests; no scenario or negative control was
removed. The no-flag HTTP continuation oracle keeps its explicit usage checks.

`rtk go test -race ./tools/continuationparity -count=1 -timeout=90s` PASS:
114 tests. `rtk err make verify`, `git diff --check` and PowerShell parsing
PASS. The native script now runs every mode and all negative controls before
reporting the failed-mode list and exiting nonzero, retaining failure evidence
and any independent successful slices. Native qualification remains PARTIAL.

## Native five-mode results and realtime portability

All eight Linux release phases PASS on exact clean
`011d8cc394fe638a347521ac5a9e94eeccfd0f99`, including the six verified native
Codex 0.163.0 app-server cases, DeepSeek 30/30, Gemini 13/13 and realtime.
The complete PowerShell native script also PASS on this clean Linux commit.

Actual native [CI execution](https://github.com/christiandoxa/godex/actions/runs/39118467016)
now PASS **five of six executable modes on both macOS ARM64 and Windows AMD64**:
continuation (56 model attempts with quota probes separate), completed-idle
capacity (12), committed idle (2), lookahead idle (2), absolute deadline (4).
All native negative controls PASS. Linux and both security lanes PASS.
Realtime remains **PARTIAL**: macOS resets its initial control exchange;
Windows completes control/audio/turn exchange but the fixture then tries to
read an exclusively held `.log.lock` file while waiting for saturation.

The saturation observer now opens only `.log` and `.jsonl` event files.
`TestProdex04382RealtimeSaturationReadsLogsWithoutOpeningLockFiles` failed
before correction with an unreadable lock sentinel, then PASS. Small realtime
control frames and fixture replies now buffer their header/payload into one
transport write before the peer's 20ms pump timeout; the single-write regression
also failed before correction. Wire payloads, ownership, permit admission,
close and replay assertions remain unchanged. This does not certify arbitrary
packet segmentation or delayed first messages.

`rtk go test -race ./tools/continuationparity -count=1 -timeout=90s` PASS:
116 tests. `rtk err make verify`, `git diff --check` and all four realtime
closure differentials on Linux PASS using the clean `011d8cc3` candidate.
Corrected native realtime results still require another native CI execution.

## Native shutdown admission mismatch

The clean `f0c66c8856c87e0a86a7ab13407103875f6aac34` candidate PASS all eight
Linux release phases and the complete PowerShell native script, retaining
native Codex 0.163.0, DeepSeek 30/30, Gemini 13/13 and all realtime closes.
Native [CI execution](https://github.com/christiandoxa/godex/actions/runs/39119182542)
PASS the same five non-realtime executable modes on macOS ARM64 and Windows
AMD64. Linux and both security lanes PASS. macOS still resets its first
realtime control exchange; coalescing frames alone has not resolved it.

The Windows log-file correction now exposes a genuine Godex shutdown mismatch.
All four official Prodex realtime closures PASS; Godex client/upstream/cancel
PASS, but Godex shutdown dispatches one waiting Responses turn after releasing
the live permit. The reference closes without that handshake or inference.
The oracle correctly fails the `1/1` observation instead of its required `0/0`.

`TestProdex04382ProxyShutdownStopsAdmissionBeforeReleasingLivePermit`
reproduces the race deterministically on Linux: a saturated follower is already
waiting, and closing the active socket releases its permit before teardown
continues. Before the fix, the assertion records two gateway dispatches.
Routing now stops new weighted admission under its existing mutex and wakes
waiters with cancellation before HTTP delivery closes any active tunnel.
Final gateway cleanup remains after transport teardown. The regression and
the existing realtime permit-lifetime regression PASS with the race detector.

Ownership remains explicit: `usecase/routing` owns admission policy and
`delivery/http/proxy` owns teardown order. The stop state is private and is
never persisted; there is no new dependency, helper package, configuration,
CLI flag or credential access. Full focused/race/build and clean-executable
qualification results for this production correction are recorded below once
they complete. Native overall parity remains PARTIAL.

Before committing the shutdown correction,
`rtk go test -race ./internal/delivery/http/proxy ./internal/usecase/routing ./internal/usecase/runtime -count=1 -timeout=120s`
PASS (1,120 tests), and `rtk err make verify` plus `git diff --check` PASS.
The production change adds one routing lifecycle operation and a private stop
flag, reusing the existing permit-generation channel and mutex. No shared
helper or speculative cancellation subsystem was introduced.

## Windows native qualification and first-message calibration

The shutdown production correction is now **VERIFIED on native Windows AMD64**
in [CI run 39120366803](https://github.com/christiandoxa/godex/actions/runs/39120366803)
using exact clean `8f72e2ec0a49bc49e1222157e9288a38a6a24038` and the published
Windows Prodex executable. All six executable modes and native negative
controls PASS. Both products' realtime shutdown record zero waiting Responses
handshakes and zero waiting model turns, with one live handshake/control/audio/
turn/close lifecycle and unchanged `account-a` ownership. Linux and both
security lanes PASS. macOS ARM64 retains five passing modes and its initial
realtime reset; the complete CI run therefore still fails that native lane.

The next fixture-only calibration prepares the first masked control frame
before connecting, then writes it only after the HTTP 101 upgrade. This keeps
mask preparation outside the reference sidecar's initial 20ms pump without
changing the payload, timeout policy or protocol ordering. Read failures now
include bounded preparation/write/reply timing, retaining the underlying error.
The shared reply reader stays private to the oracle. This is not a macOS PASS
until its named native execution succeeds.

The release preflight now also runs the deterministic shutdown admission
regression explicitly. `rtk go test -race ./tools/continuationparity -count=1 -timeout=90s`
PASS (116 tests); `rtk err make verify`, shell syntax, `git diff --check`,
and all four realtime close differentials on Linux PASS using the clean
`8f72e2ec` production candidate. No additional production policy, dependency,
shared helper or configuration changed in this calibration.


## Request deadline versus weighted permit release race (0.438.2)

Prodex 0.438.2's original request-local watchdog must remain authoritative
through profile admission. A real Godex routing gap was reproduced by three
**fail-before** regression groups: a canceled request could acquire a fresh
permit and dispatch inference, a saturated waiter could wake after another
turn released its permit even though the waiter was already canceled, and
cancellation in the interval between an admission marker and gateway send
could still start upstream traffic. The previous `tryExecuteWithProfileInflight`
relied on downstream context handling and did not check context expiry before
or after weighted permit acquisition. A deliberately context-insensitive
synthetic gateway showed the late dispatches without any external account.

The fix performs an initial `ctx.Err()` check before capacity acquisition,
then a second check immediately after acquisition/marker and **before**
upstream dispatch, releasing an acquired permit exactly once if cancellation
arrived during admission. The full request-local deadline and ordinary cancel
both preserve their original error (`context.DeadlineExceeded` or
`context.Canceled`); they do not cause quota penalties, transport retries,
new owner selection or inference replay. The regression suite covers fresh
and hard-affinity requests, saturation wake concurrent with cancellation,
cancellation during synchronous admission observation, and already-expired
wall-clock deadlines. Negative controls require zero downstream calls,
zero leaked inflight weight and zero release underflows. All cases passed
repeatedly under Go's race detector after the scoped fix. This is
**source-aligned local Router behavior**; its own two-executable admission
expiry boundary fixture remains separate from the existing 30-second hidden
prelude differential and is not silently certified by those older results.


## Cancellation across WebSocket auth and owner retries (0.438.2)

The tagged 0.438.2 precommit watchdog covers **every** upstream send, not
only the first permit acquisition. An additional fail-before regression
showed that Godex's `executeWebSocketMessage` and normal upgrade auth-reload
loops could issue a second upstream request after their original context
had already been canceled. The same flaw affected hard owner transport
recovery: a first synthetic precommit transport error returned *after*
cancellation still authorized another owner-affine WebSocket attempt. Even a
request whose context had expired before the first iteration could be
forwarded by a context-insensitive gateway. The existing Router-wide permit
admission guards do not protect cancellations arriving **during** the first
upstream attempt, so that retry boundary requires its own policy check.

The minimal fix checks `ctx.Err()` at the start of each WebSocket upgrade and
message-attempt iteration, preserving the original context rather than
allocating a new per-retry deadline. A canceled or deadline-expired loop now
returns `context.Canceled`/`context.DeadlineExceeded` without any additional
upstream dispatch, owner rotation, quota penalty or transport replay.
`TestProdex04382CanceledWebSocketAuthReloadNeverStartsSecondAttempt`,
`TestProdex04382CanceledWebSocketOwnerTransportRetryNeverReplays`,
`TestProdex04382AlreadyExpiredWebSocketNeverStartsInitialAttempt`, and
`TestProdex04382ExpiredWebSocketDeadlineNeverStartsHandshakeOrMessage`
exercise the exact failure using deliberately context-insensitive synthetic
gateways. Their negative controls require the first obsolete response body
to be closed exactly once, zero later requests, and original cancellation/
deadline errors. All local tests pass under repeated Go race detection after
the fix; active-context retry behavior remains governed by existing Router
WebSocket tests. This is source-aligned local retry/cancellation protection,
not an untested claim of full native provider session parity.
