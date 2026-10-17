# Godex ↔ Prodex 0.438.1 parity checkpoint

**Target:** official Prodex tag `0.438.1`, source commit
`c3c96af2a9b87896e262e37f37cd4741a562a66f`.

The published Linux x86_64 executable
`prodex-x86_64-unknown-linux-gnu` has SHA-256
`b347889a6fa5aec6f3a0f00a1d8346eb2df30b9933361e924900904ed85e165e` and
reports `prodex 0.438.1`. The previous `0.438.0` record remains in
[`PARITY-04380.md`](PARITY-04380.md); its claims are historical and are not
silently reused as 0.438.1 evidence.

## Release delta

Prodex 0.438.1 is a maintenance release. Its externally observable changes
are the queue and recovery contracts described in the tagged
`docs/release-notes/0.438.1.md`:

- A profile recovery wait observes selection and in-flight-release revisions.
  A quota refresh, circuit clearance, or released slot wakes the request
  promptly. A notification does not bypass an authoritative quota block or
  clear a failed-profile exclusion by itself.
- One request-local watchdog covers admission, listener queueing, standard
  HTTP work, and no-sleep recovery. A queued request carries its original
  deadline to the worker; dequeueing and profile reselection never grant a new
  retry budget.
- Expired admission or queued work returns local HTTP 504 with
  `request_timeout`, makes no upstream attempt, and does not penalize a ready
  profile. A committed stream remains readable. WebSocket upgrades detach the
  handshake watchdog so later turns keep their own request deadline.

The source release notes list qualification for queue expiry, admission expiry,
fast reselection, a real 504 without an upstream send, a fresh request after a
timeout, and readiness notification interruption. These are the minimum new
scenarios for this checkpoint.

## Oracle changes

The release-only oracle now pins the 0.438.1 tag, source commit, release
manifest, Linux artifact digest, and `--version` output in:

- [`scripts/verify-release-parity.sh`](../scripts/verify-release-parity.sh)
- `tools/differential` (DeepSeek/Gemini executable differential)
- `tools/profileparity` (managed-profile and gateway executable differential)
- `tools/contextparity`, `tools/continuationparity`,
  `tools/sessionfooterparity`, and `tools/clierrorparity`

The preflight phase additionally runs the focused 0.438.1 regressions:

```text
TestProdex04381AdmissionExpiryDoesNotDispatchAndFreshRequestSucceeds
TestProdex04381ExpiredAdmissionCannotTakeReleasedPermit
TestProdex04381StandardRequestDeadlineCoversUpstreamAndReturns504
TestProdex04381BodyCaptureUsesRequestDeadline
TestFreshRecoveryWakeOnSelectionChange
```

The tests assert a local 504 and `request_timeout`, exactly one upstream
dispatch where applicable, cancellation at the original deadline, a usable
fresh request, body-capture cancellation without upstream dispatch, and
recovery that wakes after a readiness change. They are source-level
regressions and do not pretend to be a two-executable proof.

## Checkpoint evidence

| Surface | Status | Evidence |
| --- | --- | --- |
| Profile recovery early wake | VERIFIED | `ec5f984c` adds selection revision/channel wakeups; deterministic recovery regression passes, including race testing. |
| Request-local deadline through admission, body capture, and standard routing | VERIFIED for local source scope | `99623a04`, `9f046cd6`, `14df2902`, and `1ca9b69d` preserve one deadline, return local 504 `request_timeout`, cancel stalled capture/upstream work, reject expired work before a released permit, avoid expired dispatch, and detach committed streams/WebSocket sessions. |
| Official 0.438.1 source/binary oracle pins | VERIFIED | Tag `c3c96af2a9b87896e262e37f37cd4741a562a66f`, binary SHA-256 `b347889a6fa5aec6f3a0f00a1d8346eb2df30b9933361e924900904ed85e165e`, and release manifest SHA-256 `210ffd34e5e455604b172f361760074801b35b6490c0a8d39e1a4cb29f510351` are pinned and checked. |
| Cross-binary queue/recovery differential | OPEN | The current finite oracle has no controlled admission/cooldown fixture for both executables. |
| Native OAuth, Windows/macOS execution, TUI, and live quota | PARTIAL | Source and Linux synthetic evidence only; native execution or live credentials were not used. |

The clean candidate used by the final phases was pushed to `origin/main` at
`df4c448946fc458755789512e260e148470185bd`. The oracle pin checkpoint began
at `80904e2f`; implementation commits are `ec5f984c`, `99623a04`, `9f046cd6`,
`14df2902`, and `1ca9b69d`.

## Missing or previously mis-scoped evidence

The existing profile and provider fixtures do not create local admission
pressure or a controlled profile cooldown. They therefore cannot observe the
new 0.438.1 watchdog and readiness contracts. The earlier release script ran
only profile lifecycle, provider, native, Gemini, and WebSocket fixtures; a
green run of those scenarios could not detect a deadline reset while a request
was queued, a second dispatch after expiry, or a recovery wait that ignored a
ready-profile notification.

The new preflight tests close the deterministic local regressions. A separate
cross-binary queue fixture remains open: it would need an isolated loopback
gateway with controlled admission saturation, a queued request, a released
permit, and a profile-ready notification for both the pinned Prodex binary and
Godex. No real account, credential, model call, or user home is required for
that future fixture.

The existing finite oracle still does not certify full OAuth, live quota,
native Windows/macOS process behavior, TUI behavior, or arbitrary provider
recovery. Overall 0.438.1 status remains **PARTIAL** until the cross-binary
queue/recovery scenarios and the platform-specific evidence are available.

## Pin verification evidence

The pins were checked against the official repository and release API:

```text
git ls-remote https://github.com/christiandoxa/prodex.git refs/tags/0.438.1
c3c96af2a9b87896e262e37f37cd4741a562a66f

sha256(prodex-x86_64-unknown-linux-gnu)
b347889a6fa5aec6f3a0f00a1d8346eb2df30b9933361e924900904ed85e165e

prodex --version
prodex 0.438.1
```

The release API also reports the same digest in the asset metadata and the
same commit in the release manifest. The release script verifies both the
published `SHA256SUMS` entry and the downloaded bytes before executing any
fixture. The pinned release-manifest SHA-256 is
`210ffd34e5e455604b172f361760074801b35b6490c0a8d39e1a4cb29f510351`.

The pinned `--phase=preflight-tests` run passed on the pushed candidate. It
used the official Prodex 0.438.1 executable and reported
`comparison_differences: []` with negative controls passing. The exact
cross-build command `./scripts/check-cross-build.sh` also passed for Linux,
Darwin, and Windows compilation targets.

The remaining release phases also passed independently against the same clean
candidate source `df4c448946fc458755789512e260e148470185bd`:

```text
./scripts/verify-release-parity.sh --phase=preflight-tests  PASS
./scripts/verify-release-parity.sh --phase=preflight-oracles PASS
./scripts/verify-release-parity.sh --phase=deepseek          PASS
./scripts/verify-release-parity.sh --phase=gemini            PASS
./scripts/verify-release-parity.sh --phase=websocket         PASS
```

Each phase used the pinned Prodex 0.438.1 executable and retained its negative
controls. This is a finite synthetic release gate; it does not prove full
production-wide 1:1 behavior.

No credentials or provider requests were used for pin verification. No tag or
release was created by the parity gate.
