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
| 0.438.0 pinned release differential | VERIFIED (finite gate only) | Clean Godex `b0d0dc7f`: independently source/SHA-pinned `--phase=preflight-tests` and `--phase=preflight-oracles` both PASS (full preflight, including official Codex 0.163.0 handshake and 34/34 profile cases). DeepSeek 22/22, Gemini 10/10, WebSocket **50 upstream attempts** including status-field wire precedence PASS, negative controls retained; zero reported semantic differences. Six-target cross-build/test compilation PASS. Not a production-wide 1:1 certification. |
| WebSocket terminal-frame Mojo classification | VERIFIED for tested status-wire cases; PARTIAL overall | Source-derived regressions reproduced actual Godex mismatches in `status`/`status_code` precedence, strict u16 bounds and nested/top-level connection-limit matching. Fixed in `4aac2592`; real pinned Prodex/Godex `--websocket` comparison confirms continuation after invalid/fractional/overflow primary status and **terminal error** for valid `65535` (one upstream attempt per mode; total 50). Status-frame mutation tests PASS. A newer **clean, exact-tagged two-binary** differential on `ab219e07` additionally verified top-level and nested connection-limit precommit recovery on a fresh WebSocket: one same-owner retry per case, neither precommit error leaks, total **58** upstream attempts. Persistent limits and already-retried budgets are bounded in in-process TCP tests, with no postcommit replay; broader future field permutations remain OPEN. |
| Usage-limit native session candidate fallback after transient pool retry | PARTIAL | Tagged 0.438.0 compiled Mojo `usage_limit_recovery_candidate` allows the originally failed profile as the **last** candidate only after a transient wait and verified eligibility/quota. Godex formerly always excluded the original owner and could relaunch a previously exhausted backup. `TestProdex04380SessionRecoveryFailedProfileFallbackAfterPoolRetry` reproduced these failures and now passes with three profiles, model-aware ready claims, backup precedence, cancellation, owner/credential negative controls and prompt-no-replay. Legacy no-quota adapter backup retries remain unchanged. Independent exact-binary three-profile session recovery differential still OPEN. |
| Super CLI required-provider usage exit | PARTIAL | Exact Prodex 0.438.0 CLI reports exit 2 for `super --api-key <value> --dry-run` without required `--provider`, while prior Godex reported exit 1 despite correct error text. Updated Godex's syntax-error classification and added source/binary-pinned CLI differential for both `super` and alias `s`, with negative controls rejecting exit 0/1/130 and no runtime execution. Focused tests PASS; direct clean-commit two-executable comparison to be run before marking VERIFIED. |
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
same fixture exercises the three distinct Codex 0.163.0 incomplete reasons
(`max_output_tokens`, `content_filter`, `interrupted`), each in buffered JSON
and terminal SSE. The **six** responses must retain the exact original
JSON/SSE bytes, response ID, reason plus opaque unknown fields, detailed token
usage, HTTP 200 and correct native content types. The upstream must receive
**exactly six** well-authenticated requests with the correct model/input;
no extra attempt, invented completion, quota classification or escaped string
substitution can pass. Positive
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

### All three Codex 0.163.0 incomplete reasons

The same two-executable `gateway_native_incomplete_04380` fixture was expanded
from 2 to 6 upstream requests: `max_output_tokens`, `content_filter`, and
`interrupted` are each delivered as a buffered native Responses JSON result
and a streaming `response.incomplete` SSE terminal frame. For both official
Prodex 0.438.0 and the Godex candidate, every response must preserve exactly
the original status, reason, response identity, token usage, unknown fields,
content type and event bytes. A synthetic upstream counts each dispatch and
rejects mismatched model, input, account or bearer. Focused live-binary smoke
and mutation negative controls PASS. This is a **synthetic wire** oracle, not
evidence of actual interrupted live inference or native compaction behavior.

## Independently pinned preflight execution partitions

After expanding the profile oracle to 34 stages (including three distinct
`response.incomplete` reasons in buffered/SSE pairs), a single `--phase=preflight`
invocation on candidate `3b6d3c83` reached the synchronous 112-second tool
budget **after local focused checks passed but before the executable oracles
completed**. This is neither a semantic FAIL nor evidence of a completed
preflight on that commit. The script now supports `--phase=preflight-tests`
(local regression and race sentinels) and `--phase=preflight-oracles`
(exact-source and binary PTY/profile/CLI/context differentials) separately.
Each partition independently verifies tagged Prodex source, published binary
SHA and release manifest, then builds the same clean Godex source. Both must
PASS on an identical commit; either partition by itself is **not** the entire
preflight. The existing `--phase=preflight` and no-argument full release gate
retain **both** groups with no tests removed. This avoids hiding an uncompleted
run behind a timeout or relaxing any negative control.

## Complete pinned release-gate checkpoint on `37246b8b`

The single-worktree/one-process preflight at `3b6d3c83` exceeded the
synchronous tool budget after completing local tests; the independent pinned
partitions added in `37246b8b` resolved that execution constraint **without
removing or weakening any tests**. All of the following returned exit 0 on
the **same exact clean Godex commit**
`37246b8bf80025cce524943b1a567dfd70f9c7fb`:

- `--phase=preflight-tests`: **PASS** (all local pinned regressions,
  verified official Codex 0.163.0 native app-server, HTTP payload safety,
  real Linux low-RLIMIT EMFILE acceptance and shutdown, auth recovery and
  mutation controls).
- `--phase=preflight-oracles`: **PASS** (published Prodex 0.438.0 source,
  binary/version SHA and manifest checked; Linux PTY, profile lifecycle,
  CLI errors and context; **34/34** profile stages, including cross-executable
  Linux FD saturation with a queued/retained-port request and **six** native
  buffered/SSE incomplete results for three distinct incomplete reasons).
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`, all
  configured negative mutation controls PASS.
- `--phase=gemini`: **PASS 10/10**, `comparison_differences: []`, all
  configured negative mutation controls PASS.
- `--phase=websocket`: **PASS**, 42 loopback upstream attempts, precommit
  retry, owner-affinity and strict no-postcommit-replay checks.

Every invocation independently pinned Prodex source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, published Linux binary
SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
and manifest, then built/checksummed the same clean Godex VCS candidate.
`make verify` PASS on the preceding production-equivalent commit `3b6d3c83`;
`scripts/check-cross-build.sh` independently PASS on exact `37246b8b` across
Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 plus Darwin ARM64
and Windows AMD64 cross-test compilation. The default no-option script and
unsplit `--phase=preflight` still run the complete original groups and
cannot report success for a partial group.

The finite synthetic oracle is therefore **VERIFIED** for these checked
contracts. Production-wide behavioral parity remains **PARTIAL**: genuine
native Windows/macOS runtime and TUI, Rust accepted-stream-clone failure
isolated from general FD exhaustion, real live multi-profile OAuth/quota
restart behavior, and native Codex 0.163.0 foreground/background model
compaction are outside the verified fixture coverage. Never claim
unqualified 1:1 on the basis of finite synthetic runs alone.

## WebSocket terminal status and code precedence — tagged 0.438.0

The tagged `crates/prodex-runtime-proxy/src/websocket_message.rs` and Mojo
`prodex_runtime_websocket_terminal_frame_plan_v1` require `status` to take
precedence over `status_code` based on **presence**, not whether Go can parse
it: an invalid, fractional, null, negative or out-of-u16 primary status may
not fall back to a secondary `400`/`503` and invent a terminal frame,
overload retry, account rotation or request replay. A valid unsigned u16
status of `1000` or `65535` is terminal for `type:error`, unlike the previous
Go HTTP-only 999 cap. Connection-limit detection checks nested `error.code`
**and** top-level `code` independently, folds only ASCII A–Z, and does not
trim whitespace or accept Unicode lookalikes. A malformed nested field cannot
mask a valid top-level code.

`TestProdex04380WebSocketTerminalFrameStatusPriorityAndBounds`,
`TestProdex04380WebSocketTerminalFrameConnectionLimitFromEitherCodeField`,
`TestProdex04380WebSocketDecodedFrameRetainsTaggedTerminality`, and
`TestProdex04380WebSocketStatusPrecedenceCannotInventTransportRetry` initially
reproduced multiple classification failures and now PASS after aligning the
parse/selection policy. A local unmasked server WebSocket frame fixture
verifies byte-identical forwarding and the resulting terminal/retry decision;
negative controls cover `status:399,status_code:400`, invalid primary with
secondary 503, top-level connection-limit with unrelated nested error,
malformed nested JSON types, Unicode/whitespace false positives and u16
bounds. The focused race test and complete OpenAI package suite PASS. This
is a tagged-source/local-wire checkpoint, **not** yet a cross-binary
adversarial-frame certification of the full runtime WebSocket retry behavior.

## Fully pinned 0.438.0 WebSocket terminal-state gate — clean `b0d0dc7f`

The official Prodex 0.438.0 executable and clean Godex commit
`b0d0dc7f17b9d5db89e61bd9b808e75cc98caa83` matched on **four** exact
adversarial WebSocket streams newly added to the continuous pinned
`tools/continuationparity --websocket` release oracle:

- `status:"invalid"` with `status_code:400`: **nonterminal**, followed by
  the exact next `response.completed` event.
- `status:400.5` with `status_code:400`: **nonterminal**; fractional numeric
  input must not be truncated to an integer.
- `status:65536` with `status_code:400`: **nonterminal**, because the
  explicitly present primary exceeds the Mojo u16 limit and masks fallback.
- `status:65535`: **terminal**; the deliberately delayed second completion
  frame must never leak to the client.

Each case must use precisely one same-owner upstream attempt, preserve exact
client frame bodies/order and resist mutation of the status or stream closure.
The strict differential observed **50 total upstream attempts**, passing the
full earlier continuation/opaque-tool/no-postcommit-replay suite alongside
these new cases. Targeted `TestProdex04380WebSocket...` fixtures additionally
check top-level/nested connection-limit codes, malformed nested JSON, ASCII
casefolding and status-to-overload retry safety. These failed before the
production Go correction and pass after it. This is an independently pinned
**finite synthetic WebSocket proof**, not a live upstream bearer/realtime
session test nor universal future-field compatibility guarantee.

On the same exact clean `b0d0dc7f` source, all pinned release phases exited
0: `--phase=preflight-tests`, `--phase=preflight-oracles` (34/34 profile
stages), `--phase=deepseek` (22/22, no differences), `--phase=gemini`
(10/10, no differences), and `--phase=websocket` (50 upstream attempts).
Every phase independently checked source tag, binary SHA-256, release manifest
and clean Godex VCS build metadata; every applicable mutation sentinel
remained enabled. `make verify` PASS at the preceding production-equivalent
commit containing the same runtime fix; the broader source checks and the
six-platform `scripts/check-cross-build.sh` matrix also PASS on `b0d0dc7f`.
The split preflight partitions are mandatory together, never substitutes
for each other. Full production behavioral parity remains **PARTIAL** for
native platform, live OAuth and compaction differences outside these tests.

## Native failed-profile fallback after a completed retry round

Source derived from tagged Prodex 0.438.0
`crates/prodex-app/src/app_commands/runtime_launch/usage_limit_recovery/plan.rs`,
`run_command_strategy.rs` and compiled Mojo
`prodex_launch_usage_limit_recovery_candidate_v1`. The bounded policy first
chooses an unattempted, authenticated/eligible, quota-ready backup. Only if no
backup qualifies and `allow_failed_profile` is enabled **after a successful
transient pool wait**, the original failed profile may become the last-choice
candidate, provided its own eligible authenticated identity is intact and
its quota is ready (or the user explicitly opted out of quota checks).

Godex previously permanently excluded original A after it failed, even after
B and C were attempted and a new quota observation found only A ready. It
also could retry a backup previously exhausted in the next round without
checking readiness. The new recovery planner preserves the old one-round
backup selection, marks the fallback gate only after a successful five-second
transient retry wait, re-evaluates verified candidate readiness for the
requested session model and attempts A only as the **last** authenticated
quota-ready candidate. A previously committed prompt/tool result is never
replayed; the original session affinity is released only immediately before
a newly verified relaunch checkpoint. Missing quota adapters retain legacy
backup-only transient retry, **never** an unverified A fallback. A rejected
quota observation, disabled/unknown account, non-ChatGPT credential, foreign
upstream owner, attempted profile or cancelled context cannot release A.

`TestProdex04380SessionRecoveryFailedProfileFallbackAfterPoolRetry`,
`TestProdex04380FailedProfileFallbackRequiresExactOwnerAndFreshEligibility`
and `TestProdex04380NativeRecoveryRequiresModelSpecificQuotaEvidence`
include fail-before witnesses, explicitly source-provided candidate order,
model-specific Responses route eligibility, local negative controls and
cancellation. Existing `TestProdex04360TransientPoolRecyclesOnlyWithFreshEvidence`
remains mandatory and passes without weakening its assertions. These are
local Go behavioral tests; **cross-executable real native three-profile
fallback equivalence** and every variant of provider-specific quota/compaction
remain PARTIAL/OPEN until independently executed against both binaries.

## Complete pinned 0.438.0 finite gate after failed-profile recovery fix

On exact clean Godex source commit
`83b8c987a7c1fe356bfcdf1c7704363ee54d16bb`, all mandatory independently
pinned release phases exited successfully. Each independently rechecked the
official Prodex 0.438.0 tagged source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, public release manifest,
Linux binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`,
and the clean Godex candidate's embedded build revision:

- `--phase=preflight-tests` **PASS**, including source-derived native A→B→C
  exhausted-pool recovery with A last-resort ready fallback, model-aware
  quota checks, cancellation/owner/credential/no-replay negative controls,
  existing five-second transient backup recycling regressions and the actual
  official Codex 0.163.0 app-server handshake fixture.
- `--phase=preflight-oracles` **PASS**, Linux PTY, CLI argument/context and
  **34/34 profileparity stages**, including the real two-executable FD
  saturation test and six terminal-incomplete variants.
- `--phase=deepseek` **PASS 22/22**, `comparison_differences: []`, negative
  controls PASS.
- `--phase=gemini` **PASS 10/10**, `comparison_differences: []`, negative
  controls PASS.
- `--phase=websocket` **PASS 50 upstream attempts**, including the four
  adversarial status precedence streams, opaque continuation affinity and
  strict no-postcommit-replay checks.

`make verify` PASS on the production source at `83b8c987`; the source-level
focused race regressions and cross-compilation of runtime test binaries for
Windows AMD64 and Darwin ARM64 also PASS. Full
`scripts/check-cross-build.sh` matrix PASS on the same exact clean source
for Linux AMD64/ARM64, macOS AMD64/ARM64, Windows AMD64/ARM64 plus the
Windows AMD64/macOS ARM64 test-compile matrix. Native execution on
Windows/macOS was not performed.

This verifies the bounded synthetic gate, **not** comprehensive parity in
live user accounts or the physical native terminal on every platform.
Native three-profile relaunch using authenticated quota after a real Prodex
and Godex process remains **PARTIAL**: source-derived Go planner tests show
correct candidate precedence, but the model-aware quota adapter may use a
valid recent snapshot under its source-specific grace period; live credentials
and model-specific quota transitions have not been matched across two
independent native session executables. Never infer a production-wide 1:1
certificate from the finite suite alone.

## Super `--api-key` required-provider usage category

A direct isolated Prodex 0.438.0 CLI experiment found that
`prodex super --api-key synthetic-cli-key --dry-run` without a `--provider`
returns **exit 2**, the canonical command-line syntax/required-argument
status. Godex previously raised the same textual error but returned exit 1,
incorrectly treating malformed command input as a runtime startup failure.
Godex now uses its existing typed `cliArgumentError` for that narrow
required-provider validation, retaining the same redacted diagnostic text
and leaving unrelated runtime-error exit codes unchanged. Source-derived
`TestProdex04380SuperAPIKeyWithoutProviderIsCLIUsageError` fails before
and passes after the fix; the executable parity tool adds both the full
`super` and short `s` paths, with mutation controls that reject exit 0, 1
and 130 in place of the required exit 2. All fixtures use fake credentials
in isolated temporary homes; actual provider keys are never used.


## Fresh WebSocket connection-limit retry (Prodex 0.438.0)

A two-executable differential exposed a previously unverified behavioral gap:
Prodex 0.438.0 receives a top-level
`code: WEBSOCKET_CONNECTION_LIMIT_REACHED` even when nested `error.code` has
an unrelated value, drops the uncommitted error, reconnects to the **same**
profile, and completes on its **second** upstream attempt. Godex previously
returned the raw terminal error after a single attempt because its transport
retry handler only retried recycled WebSocket sessions, not fresh ones. The
same mismatch was reproduced with an uppercase nested limit and unrelated
top-level code. A source-pinned executable run initially FAILED on Godex
while Prodex recovered. The production fix in `ab219e0744a287903533325456e6329536adb713`
permits **one fresh precommit same-owner retry**, reusing the exact opaque
request and existing `FirstEventRetryUsed` retry guard. It never authorizes
retry after the first committed output frame.

The exact official Prodex 0.438.0 source and SHA-verified binary were compared
again to the clean `ab219e07` Godex build in the expanded WebSocket oracle:
**PASS 58 upstream attempts**, including both fresh-connection limit modes
with exactly two attempts and one final `response.completed` each. The
in-process TCP regression verified top-level/nested code priority, persistent
limit bounded at one retry, already-consumed budget, byte-identical request,
and visible output followed by connection limit with **zero** replay.
Additional oracle negative controls reject extra/missing attempts, owner
rotation, wrong final response ID, leaked first error, or extra completion.
`make verify` PASS for the fix. This is a verified bounded fixture, not proof
of every live native WebSocket session state or all future upstream fields.
