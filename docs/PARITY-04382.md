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

The above are local loopback/source-derived regressions. **Full 0.438.2
cross-executable parity is still PARTIAL** pending a new release-pinned
binary differential covering the exact two new regressions. Native Windows/
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

The existing 58-attempt cross-executable WebSocket fixture covers prior
retry, status, hard-affinity and no-replay behavior, not yet a dedicated
**two-executable concurrent idle-permit / repeated-hidden-prelude** 0.438.2
comparison. Those two *new* release features are source-aligned and verified
with the real Godex router and loopback TCP/WebSocket regressions, but their
independent Prodex executable fixture remains **PARTIAL**. Similarly, native
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
coverage**, not itself the missing two-binary concurrent WebSocket fixture.


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
