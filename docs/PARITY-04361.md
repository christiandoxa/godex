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
is not modeled, and the current fourteen-scenario differential harness still reports raw
durable-state layout differences.
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

This fourteen-scenario synthetic gate is necessary but **not sufficient** to certify full
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

The fourteen synthetic DeepSeek scenarios include a terminal single-key 429
without Codex shim retries. Successful and 429 responses have matching status,
body, retry decisions and upstream attempt counts after the routing fixes.
Strict fixture-oracle guards verify provider Authorization, request model, user
text, response output and usage independently of cross-product equality.
Exact whitelisting documents the three nonsemantic transport fingerprints
(Date timestamps, Rust server identity, and duplicate identical default
User-Agent values), with mutation tests rejecting drift in every other header.
The remaining opaque durable-state layout divergence is still
visible as a blocker, not automatically declared equivalent.


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

The expanded fourteen-scenario differential suite includes a real cross-process
429-to-healthy recovery on the same isolated state root. This verifies that
terminal provider failures do not poison subsequent process launches. Raw
internal file inventories remain visible and have not been treated as proof
of equal persistence semantics; corrupt journal, multi-profile state repair,
and goal-monitor restarts require independent behavioral tests.


The tagged Prodex runtime likewise returns a sole-credential upstream
service-unavailable (503) response when no alternate model/credential is
eligible. A router fix prevents Godex from silently waiting for the same
credential until Codex times out; independent gateway, routing and executable
comparisons verify both terminal 429 and 503 paths.


The added single-key HTTP 503 and recover-after-503 workflows check the
outage response and a later healthy process restart against Prodex 0.436.1.
Prodex's runtime profile score sidecar may be written following an overload;
this material state transition must be evaluated explicitly, not omitted
through filename-level normalization.


Prodex's 503 provider-health sidecar and narrowly formatted atomic-write
temporary files are audited and distinguished from unexpected persisted
state. The fourteen-scenario differential suite, including the
503-to-healthy cross-process case, verifies user-visible behavior for
one synthetic key, **not multi-profile health ranking equivalence**.


A source-audited sidecar check decodes Prodex runtime health-score snapshots
(including bounded atomic-save temp files) and requires empty score maps for
the single-key synthetic fixture; Godex route-memory and backoff state must
also be empty. Nonempty provider scores fail independent negative-control
tests. The multi-profile scoring and continuation-journal contracts remain
outside this narrow proof.


An additional synthetic two-key DeepSeek differential scenario checks
the provider credential-rotation contract: the primary key is always
rate-limited, and success requires selecting the distinct secondary key
without the Codex client retrying. The accepted key is represented only
by an opaque slot label in test output. This still does not establish
all provider/auth combinations or multi-profile health ranking parity.


**Closed: API-key pool rotation persistence.** Godex now marks only
launch-local API-key pool accounts as ephemeral and confines their retry,
route-health, and circuit decisions to the process lifetime. The same
credential hash can be used again by a new process without inheriting an
old backoff. Managed profiles still durably persist their own retry backoff
and health scores, as verified by independent regression tests. With the
canonical 0.436.1 Prodex binary and the exact-source Godex candidate,
both key-rotation runs select primary→secondary on **both launches**,
and all prior ten synthetic scenarios agree on exit code, translated response,
ordered upstream requests and credentials. The raw durable-file layout
still differs, and full parity beyond these fixtures is not certified.


## Exact DeepSeek stream and provider-error parity evidence

Full binary-to-binary differential execution was extended beyond buffered
DeepSeek messages to include a tool call, terminal authentication failures
(401/403), and an actual SSE stream. The SSE contract identified and fixed
three client-observable divergences: Godex previously emitted an additional
response.output_text.done frame, included buffered-only object/created_at
fields in the completed streaming response, and exposed upstream headers
instead of the canonical UTF-8 SSE Content-Type projection. Ordinary
buffered-response Date and Content-Length handling is unchanged.

The SSE comparator validates event count, ordering, sequence, body and usage
against an independent fixed reference before normalizing only the opaque
numeric output-item ID, which must match between added/done events. These
additions improve verified provider wire coverage. Full parity remains
unproven where other providers, state contracts or transports have not
been differentially exercised; the raw durable-file mismatch remains a
fail-closed release blocker.
