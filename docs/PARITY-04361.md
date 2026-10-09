# Prodex 0.436.1 parity checkpoint

Reference: exact Prodex tag `0.436.1`, commit
`4c61dc0a84a6cf4852feb08e3a8406c1846bb5ad`. This checkpoint records the
behavior implemented in the current Godex worktree; the older
[`PARITY.md`](PARITY.md) document remains the historical 0.435.5 audit.
The canonical 0.436.0 to 0.436.1 delta extraction is in
[`PARITY-04361-DELTA.md`](PARITY-04361-DELTA.md).

Closed in this checkpoint:

- DeepSeek Responses error and request shaping follows the tagged provider
  contract, including bounded native fallback behavior. With one configured credential
  and one explicit model, a structured HTTP 429 is terminal; automatic
  same-key/same-model retry previously diverged from the tagged Prodex runtime
  and is now regression-tested separately from Codex-client retries.
- Gemini thought-signature precedence, Gemini 3 first-call hardening, and
  streamed signature fields are preserved across native request, buffered
  response, and SSE translation.
- OpenAI 429 header classification, scalar quota errors, and 402/403 workspace
  quota classification preserve retry and pass-through decisions.
- Profile in-flight admission releases on terminal EOF/close, retains capacity
  during `previous_response_not_found` recovery, and keeps weighted limits
  deterministic under concurrent routing.
- Routing restores validated last-good affinity snapshots after restart and
  filters upstream `Content-Length` framing headers.
- WebSocket transport uses a 15-second standard handshake deadline, an independent
  upstream key, frame/message validation, ping/pong and close handshakes, and a
  half-open watchdog. Its idle pool caps at 128. Reuse failures and connection
  limits retry once on the owner. Continuation retries stay bounded and
  precommit; committed frames are never replayed. Pinned source:
  `crates/prodex-app/src/runtime_proxy/dispatch/websocket.rs`,
  `crates/prodex-app/src/runtime_proxy/websocket/response_tracking.rs`, and
  `crates/prodex-app/src/runtime_proxy/websocket_message/continuation_handling.rs`.
- Goal-aware runtime recovery, compressed rollout prefix checks, and native
  session recovery remain bounded and cancellation-aware.

Validation completed on the current worktree:

```text
go test ./...
make verify
./scripts/test-installers.sh
bash -n install.sh
./scripts/check-cross-build.sh
goreleaser check
goreleaser release --snapshot --skip=publish --skip=announce --skip=validate
(cd dist && sha256sum -c checksums.txt)
```

The snapshot produced cross-platform `0.2.1-next` archives and passed checksum
verification. PowerShell syntax was not run locally because `pwsh` is absent;
the Windows CI lane remains the authoritative check. Native macOS and Windows
execution was not available locally; the cross-build matrix only proves that
the six release targets compile. No live credentials, provider account, or
upstream model call was used.

Known gaps are deliberate: Godex queues quota usage snapshot writes for the
process lifetime, but does not mirror Prodex's full runtime state-save,
continuation-journal, or probe-refresh workers. Soft-affinity identity migration
is not modeled, and the five-scenario differential harness still reports
header, durable-state, diagnostic, and upstream-request metadata differences.
Success, retry, cancellation, and restart outcomes now agree on status, body,
and retry counts; the remaining observations keep this checkpoint at
IMPLEMENTED-NOT-VERIFIED.


## Release parity gate (candidate checkpoint)

The release workflow now requires an executable synthetic differential run
against the **exact Prodex 0.436.1 source** and the SHA-256-pinned official
Linux release executable. Godex must be built from the clean tagged release
commit, with VCS metadata matching the supplied source commit. The release
job fails closed on divergent observable outcomes, corrupt/missing evidence,
or uncommitted oracle/candidate trees. See
`scripts/verify-release-parity.sh` and `tools/differential/README.md`.

This four-scenario gate is necessary but **not sufficient** to certify full
feature-for-feature parity. Provider/auth, TUI/app-server, WebSocket/SSE
continuation, and durable restart behavior still require broader cross-binary
evidence. The DeepSeek loopback suite explicitly distinguishes Codex-client
retry from provider-owned retry, including an exact-source single-key 429
case. The remaining response-header, upstream-request, startup diagnostics,
and state-layout observations must be compared as semantic contracts rather
than silently normalized. Do not publish v0.2.1
until all material mismatches are resolved and the larger parity matrix is
verified.

## Differential evidence refinement

The five synthetic DeepSeek scenarios now exercise a terminal single-key 429
without Codex shim retries. Successful and 429 responses have matching status,
body, retry decisions and upstream attempt counts after the routing fixes.
Strict fixture-oracle guards verify provider Authorization, request model, user
text, response output and usage independently of cross-product equality.
Exact whitelisting documents the three nonsemantic transport fingerprints
(Date timestamps, Rust server identity, and duplicate identical default
User-Agent values), with mutation tests rejecting drift in every other header.
The remaining raw diagnostics and opaque durable-state layout divergences
are still visible as blockers, not automatically declared equivalent.


The diagnostic comparator now distinguishes the tagged Prodex launch banner
from actual runtime failure evidence. A single explicit local client timeout
is compared by failure class, validated loopback route and cancellation
state; all other diagnostics remain strict. Because Godex intentionally omits
Prodex's progress banner, this is a scoped semantic exception rather than an
assertion that raw CLI stderr matches.


The differential harness now also reads real synthetic-run state files after
exit and rejects invalid JSON, foreign rollouts, unexpected history,
world-writable state, leaked synthetic provider keys, and divergent routing
recovery snapshots. These integrity checks are necessary but do not prove
cross-implementation state persistence semantics: the raw durable-state
layout mismatch remains a release blocker.

The expanded six-scenario differential suite includes a real cross-process
429-to-healthy recovery on the same isolated state root. This verifies that
terminal provider failures do not poison subsequent process launches. Raw
internal file inventories remain visible and have not been treated as proof
of equal persistence semantics; corrupt journal, multi-profile state repair,
and goal-monitor restarts require independent behavioral tests.
