# Godex ↔ Prodex 0.438.0 parity checkpoints

**Target:** official Prodex tag `0.438.0`, source commit
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`.
Published Linux x86_64 SHA-256 for `prodex-x86_64-unknown-linux-gnu`:
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`.
Reference: tagged `docs/release-notes/0.438.0.md` and source migrations.

The previous pinned baseline is `docs/PARITY-043714.md`. Do not silently
transfer 0.437.14 differential PASS claims to the new 0.438.0 target.

## Behavioral delta matrix

| Surface | Status | Evidence / next proof |
| --- | --- | --- |
| Windows signed process exit and Unix cancellation | PARTIAL | A source-derived signed-i32 policy now detects Windows Ctrl-C NTSTATUS and real Unix SIGINT; original Go logic detected only exit 130. Tagged negative controls and native Linux child-signal tests PASS; Windows cross-compiled test binary, but Windows runtime execution has not been observed. |
| Local proxy under accept and accepted-socket resource pressure | PARTIAL (Linux cross-binary recovery VERIFIED) | Actual official Prodex 0.438.0 and clean Godex gateway both reached RLIMIT_NOFILE=80 under 120 partially written TCP connections; each retained the original port, held a newly queued GET while saturated, served it exactly once after release, served a subsequent independent GET, and exited cleanly. The new source/SHA-pinned `profileparity` stage and mutation controls cover 3 exact upstream GETs. Go's stdlib has no Rust tiny_http accepted-clone step, so an isolated cross-executable clone-specific failure and native Windows/macOS behavior remain unverified. |
| Official Codex 0.163.0 compatibility | PARTIAL | Official `rust-v0.163.0` asset metadata and SHA-256 verified; extracted binary `codex-cli 0.163.0` successfully completed real isolated app-server `initialize`, `thread/list` active and archived through the Godex protocol on Linux x86_64. `TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList` PASS. `response.incomplete`, compaction, native retry/interrupt and Windows MCP environments still need separate executable evidence. |
| 0.438.0 pinned release differential | VERIFIED (finite gate only) | All six independent binary harnesses use official 0.438.0 tagged source, manifest, binary version and SHA. On clean `c4cc8e0f` all four release phases PASS: preflight (including verified native Codex 0.163.0 app-server handshake, real EMFILE and shutdown), DeepSeek 22/22, Gemini 10/10, WebSocket 42 upstream attempts; negative controls retained, no reported material comparison differences. This is not a production-wide 1:1 certification. |
| Three-profile quota/auth restart | PARTIAL | Prior Godex 0.437.14 synthetic and multi-process tests PASS. Independent two-executable multi-profile restart differential is still needed. |

**Status is PARTIAL, not certified 1:1 production parity.** Only a fully
pinned oracle and adequate behavioral evidence can mark a surface VERIFIED.
User account secrets must never appear in tests, logs or persisted fixtures.
Push checkpoint commits normally to `main`, without force or waiting for CI;
never replace existing WIP in `/home/doxa/IdeaProjects/godex`.

## Official 0.438.0 oracle checkpoint

The tagged source checkout is clean at
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`. The official Linux
x86_64 executable was downloaded from the published release, measured at
39,355,488 bytes, verified against the published SHA256SUMS, and reported
`prodex 0.438.0`. Its release-manifest version and source commit match the
same tag. The release gate and all six independently source-verified Go
binary differentials have been pinned to these exact values, with their
negative controls retained. Existing `TestProdex043714...` names remain
historical regression cases; their names are not evidence that 0.438.0
cross-binary comparisons passed. Re-run the gate on a **clean Godex commit**
before promoting any parity surface to VERIFIED.

## Real Linux TCP descriptor-pressure checkpoint

The tagged Prodex 0.438.0 `tiny_http` patch protects the owned loopback
listener when accepting a socket or cloning an accepted socket encounters
EMFILE. Godex uses the Go standard-library HTTP server rather than the Rust
socket-cloning transport; Go's `net/http.Server.Serve` has its own bounded
retry for temporary `net.Listener.Accept` errors. The actual Godex
`Proxy.Start()` path was tested in a dedicated subprocess that sets
`RLIMIT_NOFILE=96` and opens descriptors until Linux reports EMFILE. It
sends one HTTP response before pressure, opens a real queued TCP connection
under pressure, verifies that it does not complete prematurely, then releases
the file descriptors. The queued request must produce the second response
exactly once; an independent fresh connection must produce the third response
on the identical listening endpoint. The captured native HTTP server log must
contain the actual accept EMFILE error; empty synthetic transient-error mocks
do not qualify. The success path requires all these facts.

This provides **verified real Linux accept-pressure recovery** on Godex.
It does not certify the Rust accepted-socket-clone path (which Go does not
use), Windows listener behavior or indefinitely persistent resource exhaustion.
The next checkpoint below adds the independent two-executable Linux fixture.

## Official Codex 0.163.0 native app-server handshake

The release `rust-v0.163.0` was verified through the official OpenAI/Codex
GitHub release API, with tagged commit
`83086188ae0614bd882818ff1aa23f3194962012`. The published Linux x86_64
musl tarball SHA-256 is
`beb339e8ecd9ba057a6bb2dc81ad8ca474812edfbf0fc5056ca818797648dde4`;
its independently extracted 296,865,480-byte `codex` executable has SHA-256
`399ff69c431c066e40de318fba45c2b07f45a76b19939d247ee4828fc70c4215`
and reports `codex-cli 0.163.0`. The opt-in Go regression runs its actual
`app-server --listen stdio://` with a private empty configuration and state
home, then performs real JSON-RPC `initialize` and active/archived `thread/list`
handshakes using Godex's existing bounded protocol, without starting a model
turn or loading an installed Codex login. All requests completed successfully.

Use `GODEX_TEST_CODEX_0163_APP_SERVER_BIN=/path/to/sha-verified/codex` to run
the optional release-gate check. The ordinary release gate does not download
or replace the user's Codex executable. This is VERIFIED for the narrow native
app-server handshake on Linux x86_64 only; the other native Codex 0.163.0
changes documented by Prodex remain PARTIAL/OPEN pending targeted tests.

## Codex 0.163.0 incomplete-response wire checkpoint

`TestProdex04380NativeResponseIncompletePreservesReasonAndUsageWithoutRotation`
exercises Godex's real HTTP forwarding stack with a mock upstream returning a
native Responses result with `status: incomplete`. The buffered and streamed
SSE variants contain a nonstandard future field, an opaque incomplete reason
subfield, a stable upstream response ID and nested token usage details. Both
forms returned the exact original body/framing and content type with HTTP 200.
Two consecutive client requests generated **exactly two** upstream calls,
without a hidden retry, quota classification or credential rotation.

This is local deterministic HTTP evidence for 0.163.0 native passthrough and
no-replay; it is **not** a live Codex inference run or an independent two-
executable source-pinned incomplete-response differential at the time of this
local-only checkpoint. A subsequent cross-binary fixture is recorded below. Other foreground/
background Responses-v2 compaction and native interruption features remain
open pending scenario-specific proof.

## Shutdown during unresolved Linux descriptor exhaustion

`TestProdex04380ProxyShutdownStopsAcceptRetryDuringEMFILE` starts the real
Godex TCP proxy in a separate low-RLIMIT process, verifies the listener
reports an actual EMFILE after its descriptors are exhausted, and queues a
request from the parent. Instead of releasing descriptor pressure, the parent
orders the proxy to shut down. The HTTP accept retry must terminate within a
bounded interval; the queued, **unaccepted** request must generate no upstream
model turn, and the original endpoint must stop accepting new connections.
The integration regression passed without any permanent process-local
resource limit change. Combined with the queued-request recovery test, this
locally verifies both the temporary EMFILE recovery and intentional shutdown
paths on Linux. The separate Rust accepted-socket clone path is not present
in Go's standard HTTP server; an independent Linux two-executable fixture
covers shared TCP recovery semantics, not the internal clone-failure phase
or Windows/macOS listener behavior.

## Complete pinned finite synthetic gate — clean candidate c4cc8e0f

All four independent phases of `scripts/verify-release-parity.sh` exited
0 on the exact **same clean Godex commit**
`c4cc8e0f04cced202e6ebc9e9a8491b98dacab5f`. Each phase verifies tagged
Prodex 0.438.0 source commit
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, the published release
manifest and official Linux binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`,
plus the clean candidate's embedded Go VCS revision. Results:

- `--phase=preflight`: **PASS**. Actual official Codex 0.163.0
  initialize/thread-list handshake was enabled through the optional,
  SHA-pinned executable fixture. Linux low-RLIMIT EMFILE accept recovery and
  shutdown tests PASS, together with native output safety, signed cancellation,
  Linux PTY footer, CLI, profile lifecycle, routing/auth and context fixtures.
- `--phase=deepseek`: **PASS**, 22/22 cases in one bounded concurrent run,
  `comparison_differences: []`, negative controls PASS.
- `--phase=gemini`: **PASS**, 10/10 cases including native wrapped responses,
  streaming, multi-turn tool adjacency, credential rotation and errors,
  `comparison_differences: []`, mutation negative controls PASS.
- `--phase=websocket`: **PASS**, 42 loopback upstream attempts, precommit
  retry and hard-owner/no-postcommit-replay assertions.

`make verify` PASS on the prior production-equivalent checkpoint; the
`0.438.0` release gate above reverified the subsequent clean candidate and
all added focused tests. `scripts/check-cross-build.sh` additionally PASS on
this **exact** clean candidate for six CGO-disabled executable targets
(Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64) and the
Darwin ARM64/Windows AMD64 test-compile matrix. Cross-compilation is not
native Windows or macOS runtime execution.

**Current overall parity status: PARTIAL.** The finite synthetic release
oracle is verified, but full behavioral 1:1 parity still requires
additional evidence for native Windows/macOS process and TUI behavior,
the Rust-specific accepted-stream-clone fault and separate native
0.163.0 compaction and live incomplete-result error cases, and a complete
live multi-profile OAuth/quota restart matrix. Never upgrade this finite
fixture proof into a production-wide claim without executing those cases.

## Exact Prodex/Godex native incomplete-response wire differential

The new `gateway_native_incomplete_04380` profileparity stage runs official
Prodex 0.438.0 and a Godex candidate in independent synthetic managed homes,
pointing each at a private loopback Responses mock. For each executable the
same fixture issues a buffered native `response.incomplete` and a streamed
terminal `response.incomplete` and requires the **exact** original JSON/SSE
payloads, original response ID, `max_output_tokens` reason plus opaque
unknown details, original nested token usage, HTTP 200 and correct JSON/SSE
content types. The upstream must receive **exactly two** well-authenticated
requests with the correct model/input; no third attempt, invented completion,
quota classification or escaped string substitution can pass. Positive
cross-binary and mutation-negative-control runs PASS. Neither real accounts
nor model inference are used. This closes the two-executable synthetic
incomplete **wire** parity gap, not Codex-native inference, actual model
compaction, or all possible incomplete termination reasons.

## Independent official-Prodex and Godex Linux FD-pressure differential

`tools/profileparity` now runs a **third startup scenario** named
`gateway_fd_pressure_recovery` in addition to its managed-profile lifecycle
and quota HTTP 401 reload cases. It verifies both source-pinned Linux
executables with separately imported synthetic managed credentials, a
loopback-only upstream, and shell-inherited `RLIMIT_NOFILE=80`. The harness:

1. Issues one baseline `GET /backend-api/codex/models`, checking the exact
   synthetic JSON response and authenticated upstream identity.
2. Opens 120 incomplete TCP-header connections and directly inspects each
   gateway subprocess's `/proc/PID/fd` to establish **80 of 80 FDs in use**.
3. Confirms the gateway PID survives, and a separately connected complete
   GET remains queued for at least 220 ms with zero premature upstream
   dispatch or status response.
4. Closes the partial sockets and requires the original queued GET to
   complete exactly once, with a third fresh GET succeeding on the **same**
   port. It then stops the process and validates the exact three-request
   upstream count and clean child exit.

The real HTTP pressure fixture passed against the SHA-verified Prodex 0.438.0
binary and the matching Godex candidate, in separate synthetic homes.
Mutation sentinels independently reject wrong request count, absence of
actual FD saturation, listener loss, lost queued request, failed next
connection, or failed shutdown. The fixture uses a short private temporary
path because Prodex's own capability-secret store rejects excessively long
runtime paths; it does not modify either product's real installation or
user login. This closes the **shared Linux observable recovery** gap, but
does not prove a separately forced accepted-socket clone failure in the Rust
transport or runtime behavior on macOS/Windows. Full 1:1 parity remains
PARTIAL until those and the other stated native cases are tested.
