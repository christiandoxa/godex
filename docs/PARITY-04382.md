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
