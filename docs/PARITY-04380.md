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
| 0.438.0 pinned release differential | VERIFIED (finite gate only) | On clean Godex `c68ba481`, both independently pinned preflight partitions PASS (34 profile stages, 14 CLI probes), DeepSeek **22/22**, Gemini **13/13**, and WebSocket **58 attempts** PASS with active negative controls and zero reported comparison differences. Official Codex 0.163.0 app-server also PASS on this commit. The 15 compiled-source Gemini compound native fixtures PASS; native OAuth executable proof remains PARTIAL. Six Linux/Darwin/Windows cross-build and targeted cross-test compilation targets PASS through `make verify`. Every phase verifies tagged Prodex source, manifest, official binary SHA and the same clean Go source. Production-wide 1:1 remains PARTIAL. |
| Copilot logged-in candidate input bounds | VERIFIED for local source-derived import fixture | Tagged 0.438.0 `prodex_profile_export_copilot_candidate_users_v1` and Rust adapter cap raw `(host, login)` views at 256 including one preferred last user, **before deduplication**. Godex formerly accepted 257 duplicates and reached account HTTP; failing synthetic loopback tests now PASS after bounding `Source.Load`, with exact-pair priority/dedup preserved. Real multi-user Copilot/keychain import remains PARTIAL. |
| OpenAI retired-model catalog filter | VERIFIED for tagged predicate / local cache | Prodex 0.438.0 `catalog_openai_model_is_retired` trims Unicode whitespace then performs ASCII-only case-insensitive exact matches for `spark`, `gpt-5.3-codex-spark`, `gpt-5.3-spark`. Go previously used Unicode `strings.ToLower`, which incorrectly retired lookalikes such as `sparK`. Failing source-derived negative tests and a real synthetic `models_cache.json` picker regression now PASS using `asciiEqualFold` after `TrimSpace`; interactive PTY model-picker screenshot not yet directly compared. |
| Gemini Chat-compatible null `functionCall` output | VERIFIED for one exact synthetic wire fixture | Tagged Rust source has two distinct buffered entry points with different text policies. Direct two-executable Prodex 0.438.0/Godex candidate `83fa59d5` comparison established the *public* Chat-compatible path keeps adjacent text and emits deterministic fallback `call_1` for `functionCall:null`. The separate internal buffering helper suppresses text. Focused Go tests failed before correction; full pinned Gemini differential **PASS 13/13**, zero diffs, mutation sentinels; broader function-call permutations remain PARTIAL. |
| Native Gemini compound tool-history pipeline | VERIFIED for 15 compiled-source synthetic cases; native OAuth executable proof remains PARTIAL | The complete tagged order **0/1/2/4/3** now coalesces repeated model/user rows before ID-and-name pairing, stable ID ordering and role shaping. Every case failed before the fix and now matches outputs executed by the Mojo kernel archived from the canonical source commit. Fixtures preserve signatures, first-row metadata, existing errors, null-key presence and duplicate-ID order; missing results remain explicit errors. All five missing-pass negative controls fail closed. Regeneration is credential-free via `tools/gemini-content-oracle.py`; it is not a two-executable OAuth test. |
| DeepSeek explicit message-role validation | VERIFIED for source and exact two-executable rejection fixture | Tagged `deepseek_input_item_validation` accepts only user/assistant/system/developer on explicit message items. Godex previously converted critic to user and accepted explicit tool messages. Regressions now reject critic/tool/empty/uppercase roles while preserving untyped chat-role fallback and generated function-call outputs. Exact Prodex 0.438.0/Godex differential now matches terminal HTTP 502 `upstream request failed`, exit 2, zero upstream attempts and zero retries; mutation controls pass. Broader input-shape parity remains PARTIAL. |
| Gemini API-key invalid function-tool validation | VERIFIED for tagged malformed cases; PARTIAL for all tools | Full 0.438.0 tagged two-executable Gemini differential extended from 10 to **12** cases. Prodex rejects flat/wrapped invalid `tools[0].function.parameters` locally with HTTP 400, exit 2, indexed `invalid_request_error` JSON and zero upstream attempts; Godex initially forwarded the requests (false 200), then retried local 400s as transient, then emitted plaintext. Corrected adapter, Router and HTTP error shaping in local checkpoints. Clean exact-source/binary-pinned 12/12 differential PASS, `comparison_differences: []`; broader schema permutations remain untested. |
| WebSocket terminal-frame Mojo classification | VERIFIED for tested status-wire cases; PARTIAL overall | Source-derived regressions reproduced actual Godex mismatches in `status`/`status_code` precedence, strict u16 bounds and nested/top-level connection-limit matching. Fixed in `4aac2592`; real pinned Prodex/Godex `--websocket` comparison confirms continuation after invalid/fractional/overflow primary status and **terminal error** for valid `65535` (one upstream attempt per mode; total 50). Status-frame mutation tests PASS. A newer **clean, exact-tagged two-binary** differential on `ab219e07` additionally verified top-level and nested connection-limit precommit recovery on a fresh WebSocket: one same-owner retry per case, neither precommit error leaks, total **58** upstream attempts. Persistent limits and already-retried budgets are bounded in in-process TCP tests, with no postcommit replay; broader future field permutations remain OPEN. |
| Usage-limit native session candidate fallback after transient pool retry | PARTIAL | Tagged 0.438.0 compiled Mojo `usage_limit_recovery_candidate` allows the originally failed profile as the **last** candidate only after a transient wait and verified eligibility/quota. Godex formerly always excluded the original owner and could relaunch a previously exhausted backup. `TestProdex04380SessionRecoveryFailedProfileFallbackAfterPoolRetry` reproduced these failures and now passes with three profiles, model-aware ready claims, backup precedence, cancellation, owner/credential negative controls and prompt-no-replay. Legacy no-quota adapter backup retries remain unchanged. Independent exact-binary three-profile session recovery differential still OPEN. |
| Super CLI required-provider usage exit | VERIFIED for tested cases | The exact Prodex 0.438.0 CLI and clean Godex `27d850b6` both exit 2 for missing `--provider` under `super` and `s`; all 14 exact-source/published-SHA pinned CLI probes PASS with negative controls. Uncovered malformed-option combinations remain outside the finite fixture. |
| Super CLI positioned Gemini/DeepSeek provider alias | VERIFIED for tested cases | Prodex 0.438.0 `rewrite_positioned_super_alias` chooses a provider after leading options, but preserves literal `--`/explicit provider/local URL. Godex formerly treated `s --dry-run gemini` and `s --dry-run deepseek` as OpenAI Codex-input tails; `TestProdex04380SuperPositionedProviderAliasAfterOptions` failed before the fix, passed after. Clean `27d850b6` exact-binary CLI differential PASS 14/14 including four new dry-run semantic-provider probes. Source-size baseline reduced from 901 to 889 lines with helper extraction. More opaque-argument and platform-specific parsing is still OPEN. |
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


## Fresh-limit recovery complete finite gate — exact clean `154833a3`

The tagged 0.438.0 WebSocket first-connection limit repair (`ab219e07`) and
expanded 58-attempt executable oracle (`154833a3`) were validated against
the exact public Prodex 0.438.0 source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, published executable
SHA-256 `9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`,
release manifest, and the **same clean Godex source**
`154833a39021b0c906bf742209572d233ead3f0a` in every phase:

- `--phase=preflight-tests`: **PASS**, including bounded fresh/retained
  connection-limit retry, persistent limit budget, no postcommit replay,
  exact native Codex 0.163.0 handshake, and tagged regression sentinels.
- `--phase=preflight-oracles`: **PASS**, including Linux PTY, CLI and context
  checks and all **34** source/binary-pinned `profileparity` stages (synthetic
  quota 401 reload, physical Linux TCP FD saturation, and six native
  incomplete-result buffered/SSE frames).
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`, mutation
  negative controls PASS.
- `--phase=gemini`: **PASS 10/10**, `comparison_differences: []`, mutation
  negative controls PASS.
- `--phase=websocket`: **PASS 58 upstream attempts**, with top-level and
  nested fresh connection-limit recovery both completing after **exactly
  two same-owner attempts**, plus prior terminal-status, continuation-owner
  and no-replay assertions. The oracle rejects missing or excess attempts,
  leaked precommit error, ID/owner mutation and second committed output.

`scripts/check-cross-build.sh` **PASS** on exact `154833a3`, compiling
executables for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
windows/amd64, windows/arm64 and targeted Darwin/Windows tests. Local
`make verify` and the focused Go race tests were PASS for the same production
source before the final documentation commit. Native Windows/macOS execution,
real live OAuth and three-profile model-specific quota/relaunch, actual
Codex 0.163.0 compaction, and isolated tiny_http accepted-socket clone faults
are still **PARTIAL/OPEN**. The complete finite release gate is VERIFIED;
no unqualified production behavioral parity certificate is asserted.


## Super positioned provider alias — exact Prodex 0.438.0

Prodex 0.438.0 implements its provider shorthand in two stages: an immediate
compatibility rewrite at CLI entry, then a **positioned provider** selection
after parsing leading `super/s` options. A direct isolated official-binary
experiment showed `prodex s --dry-run gemini` and
`prodex s --dry-run deepseek` selected `prodex-gemini` and `prodex-deepseek`,
while the pre-fix Godex executable selected OpenAI and passed `gemini` or
`deepseek` as a native Codex argument. The new code promotes the first
leftover native argument only when it is an exact Gemini/DeepSeek alias,
there is no literal `--` boundary, and neither `--provider` nor `--url`
explicitly claims the provider. Prefix shorthand before options retains
its previous behavior. The typed provider, quota-preflight choice and
remaining native Codex argument order are source-derived and guarded by
`TestProdex04380SuperPositionedProviderAliasAfterOptions`, including
nonprovider first positional text, explicit overrides, literal boundaries,
and shorthand followed by genuine Codex input.

Clean candidate `27d850b6953700446d7e69032fbc047af3c79b6c` was compared
to the SHA/source-pinned Prodex 0.438.0 binary by the extended
`tools/clierrorparity` executable harness. All **14 CLI probes PASS**,
including four semantic provider-selection cases under `super`/`s` plus
the existing exit-2 required-provider checks. The fixture recognizes the
canonical Prodex `prodex-<provider>` internal dry-run name and the public
Godex `<provider>` name as equivalent **only** for these explicitly typed
providers; mismatched names, missing provider output, leaked native
argument markers and wrong exits still fail the independent negative
controls. No live provider login or model turn is involved.

The source-size regression detected `super.go` over its 901-line legacy
limit. The policy was extracted into a dedicated helper in the same package,
shrinking `super.go` to **889** lines and lowering its committed baseline
accordingly. `make verify`, source-size, format and focused race tests PASS.
This verifies the bounded named-option alias behavior; opaque OS arguments,
all future CLI parser versions and actual runtime provider startup are not
certified by this synthetic dry-run fixture.


## Complete pinned Prodex 0.438.0 synthetic gate after Super alias repair

The latest clean source `f54886eb1fb7c12afb34cd874723b42fc1dc4f86`
was tested against the **same** exact Prodex 0.438.0 source tag
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, release manifest, and
public Linux x86_64 SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
in every independently pinned phase:

- `--phase=preflight-tests` **PASS**: local regression/race sentinels for
  native cancellation, quota/auth recovery, first-connection WebSocket
  limit retry/no committed replay, real Linux descriptor saturation,
  positioned provider alias, and official Codex 0.163.0 app-server handshake.
- `--phase=preflight-oracles` **PASS**: exact binary PTY footer, context,
  34/34 profile lifecycle/startup/FD/incomplete-wire cases, and **14/14 CLI
  probes**, including the four new semantically normalized Super Gemini/
  DeepSeek alias dry-runs and original required-provider exit-2 checks.
- `--phase=deepseek` **PASS 22/22** with `comparison_differences: []` and
  all existing mutation negative controls PASS.
- `--phase=gemini` **PASS 10/10** with `comparison_differences: []` and
  all existing mutation negative controls PASS.
- `--phase=websocket` **PASS 58 synthetic upstream attempts**: top-level
  and nested connection-limit retry on a fresh socket, invalid/overflow/u16
  status terminality, hard owner continuation and no postcommit replay.
- `scripts/check-cross-build.sh` **PASS** on exact `f54886eb`, building
  linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64,
  windows/arm64 and compiling targeted Darwin/Windows gateway/runtime tests.

The source-equivalent checkpoint passed `make verify`, Go race, format and
source-size checks before the final documentation commit. A finite synthetic
release gate **cannot** prove live production-wide 1:1 parity. Independent
native Windows/macOS runtime tests, actual live OAuth/multi-profile
model-specific quota and relaunch, native Codex 0.163.0 compaction, and
isolated Rust accepted-socket clone pressure remain PARTIAL/OPEN. Neither
CI status nor an externally published tag is a prerequisite for these
ordinary non-force development pushes; release/tag publication is separate.


## Gemini API-key function tools — indexed 0.438.0 rejection wire

A new **failing** exact Prodex 0.438.0 vs Godex differential exposed an
uncovered request-hardening gap. Prodex's tagged request validation refuses a
wrapped function tool with non-object `parameters`, and a flat function tool
with missing `parameters`, before dispatch to Gemini. Both return HTTP 400,
child exit 2 and structured `error.code=invalid_request_error`, with the
canonical indexed `tools[0].function.parameters` message. Previously Godex
sent both malformed requests to the Chat-compatible provider mock, which
returned HTTP 200. After local validation was added, its Router incorrectly
retried that terminal HTTP 400 until a timeout. After Router classification
was fixed, its proxy still emitted plaintext rather than Prodex's JSON error
envelope. Three separate fail-before witnesses led to scoped fixes in the
Gemini API-key bridge, the local-validation error path in Router, and the
Responses HTTP error serializer. Native Gemini/OAuth paths, ordinary upstream
400 errors and transport retry classification are intentionally unchanged.

The full official-source/binary-pinned Gemini differential **PASS 12/12** on
clean Go source after all three corrections, with
`comparison_differences: []`. Both new cases show exact JSON errors, HTTP
400, exit 2 and **zero** upstream requests; old ten response/stream/quota/
rotation cases also PASS. Source-derived regression tests additionally
verify malformed name/description, successful valid flat/wrapped function
tools (not suppressed), no repeated account/routing attempts, and negative
controls that reject wrong status, exit, error field, missing provider check
or leaked upstream POST. This is a finite simulated request-validation oracle,
not a certification of live Gemini/OAuth or all possible tool schemas.


## Complete pinned 0.438.0 gate after Gemini invalid-tool correction

All five independently pinned phases exited 0 on the **same clean** Godex
candidate `49caa1c4d489d216b53addad30241e9a574b5bfb`, against Prodex
official tagged source `8cab9794b0e08b2cb8507df8321a8e38ebd6492f`,
published Linux x86_64 executable SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
and the verified release manifest:

- `--phase=preflight-tests`: **PASS**, including official Codex 0.163.0
  app-server handshake, local Router/Gemini/proxy, signed-cancellation,
  actual Linux descriptor saturation and no-replay regression guards.
- `--phase=preflight-oracles`: **PASS**, 34/34 managed-profile/FD/incomplete
  executable stages, 14/14 Super/CLI probes, Linux PTY and context oracles.
- `--phase=deepseek`: **PASS 22/22**, zero comparison differences, all
  mutation sentinels PASS.
- `--phase=gemini`: **PASS 12/12**, zero comparison differences, including
  two new indexed malformed function-tool cases with identical local HTTP
  400/JSON `invalid_request_error`/exit 2 and **zero** provider POSTs.
- `--phase=websocket`: **PASS 58 upstream attempts**, including fresh
  connection-limit precommit retry and no replay after committed output.

`make verify` passed on the candidate's exact production source before the
final documentation-only checkpoint, including Go shuffled race tests, vet,
format/source-size checks and build. `scripts/check-cross-build.sh` also
**PASS** on exact `49caa1c4`, compiling six production binaries across
Linux/macOS/Windows and test binaries for Darwin ARM64 and Windows AMD64.
The no-argument and unsplit preflight commands retain all original tests;
partitions exist only to fit independent synchronous execution budgets.

**Overall production 1:1 parity is still PARTIAL.** This finite oracle does
not certify real native macOS/Windows execution, Codex 0.163.0 live model
compaction, all Gemini/OAuth tool schema combinations, live multi-profile
quota and session recovery, or independently forced Rust accepted-socket
clone failures. Do not turn these bounded synthetic PASS results into a
production-wide parity claim.


## Gemini null tool-call public wire — independent tagged 0.438.0 evidence

The tagged Rust test `gemini_provider_core_buffered_response_suppresses_visible_text_across_tool_calls`
initially suggested a text-suppression rule for `functionCall:null`.
A direct *public API-key Chat-compatible executable* oracle revealed that
this was the **wrong entry point** to mirror: Prodex 0.438.0's real public
Responses payload retains the adjacent text (`hidden beforehidden after`)
as an assistant message, then emits a separate `function_call` item with
`name:tool_call`, `call_id:call_1` and `arguments:{}`. Both message and tool
call are client-visible. Godex originally ignored null `functionCall` and
lost the tool item; a preliminary fix incorrectly suppressed text and used
an unstable UUID. Neither incorrect state was pushed. The corrected Go
API-key translation preserves both output items, the exact call fallback and
metadata/usage, while the direct native/OAuth GenerateContent route retains
its independently tested ID/visible-text policy.

The candidate `83fa59d540edc7809a7bd57ecc93f7e6f7fa961d` completed the
source/SHA-pinned **Gemini 13/13 executable differential PASS** against the
official Prodex 0.438.0 binary, with `comparison_differences: []` and all
negative controls PASS. The new fixture compares strict output ordering,
text, function name, arguments, call ID and all nonvolatile response fields;
it normalizes only independently validated generated Responses IDs and
wall-clock fields. Go regression tests protect the native/OAuth route,
unknown malformed part types, and null tool-call key presence. The
`native_response.go` source-size baseline was lowered after moving shared
candidate/alias helpers into a cohesive 0.438.0 module. This establishes
only a bounded synthetic public-wire contract, not all live Gemini model
tool-call variants or foreground compaction behavior.


## Complete pinned release gate — Gemini 13/13 checkpoint `c6efe3c0`

The public Gemini Chat-compatible null tool-call fix is verified together
with **all** independently pinned 0.438.0 finite release phases on the exact
same clean Godex candidate
`c6efe3c00c25dfdf2bcc22c2c357a7047facf827`, against official tagged
Prodex source `8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, release
manifest and published Linux x86_64 binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`:

- `--phase=preflight-tests`: **PASS**, including official Codex 0.163.0
  native app-server handshake and tagged provider/transport/routing controls.
- `--phase=preflight-oracles`: **PASS**, 34/34 managed-profile/FD/incomplete
  physical/executable cases, 14/14 CLI probes, PTY and context oracles.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`, negative
  mutation controls PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`, negative
  controls PASS. In particular, public buffered `functionCall:null` retains
  adjacent visible text and its deterministic `call_1` item on *both*
  executables. The previous 12 provider/quota/error cases remain mandatory.
- `--phase=websocket`: **PASS 58 upstream attempts**, preserving precommit
  same-owner retry, continuation affinity, terminal status and no replay.

`make verify` passed on the exact production-equivalent source before the
final docs-only checkpoint (including shuffle/race tests, vet, source-size
and build). `scripts/check-cross-build.sh` passed on exact `c6efe3c0` for
Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 and targeted
Darwin ARM64/Windows AMD64 test compilation. The original monolithic
release gate and unsplit preflight still contain every test; independent
partitions are used only to fit synchronous execution bounds.

**Production 1:1 parity remains PARTIAL**, not certified by this bounded
synthetic suite. Remaining material unverified cases include actual native
Windows/macOS runtime execution, live Codex 0.163.0 foreground/background
compaction, live OAuth and multi-profile model-specific quota/relaunch,
provider schema permutations beyond fixtures, and independently forced
Rust accepted-stream-clone exhaustion. No live secret or real model turn
was used for these local oracle cases.


## OpenAI dynamic catalog aggregate alias/effort view ceilings

Tagged Prodex 0.438.0 `prodex_mojo_rich_catalog_choices_v2` separately bounds
the total number of alias and reasoning-effort views at **65,536** across
the entire dynamic-model input. The 1 MiB cache-file limit does not enforce
this property: 65,537 very short alias strings or one-character effort
entries can still fit. Before this correction, Godex loaded such caches
without checking the aggregate view count. Failing regressions now pass
with explicit typed-view totals, rejecting 65,537 aliases/efforts, preserving
an exactly-65,536-alias fixture and rejecting two individually valid models
whose aliases together exceed the limit. The validator skips non-string
aliases and non-object effort entries like the Rust source acquisition
stage, but cannot be bypassed by heterogeneous JSON arrays. These local
file-cache and race tests complement the earlier per-field/record bounds;
a direct PTY model-picker differential is still PARTIAL.

## OpenAI dynamic model-cache input bounds and fail-closed fallback

Prodex 0.438.0 `prodex_mojo_rich_catalog_choices_v2` rejects more than **1,024**
model records, identifiers longer than **4,096 UTF-8 bytes**, labels/efforts
longer than **65,536 bytes**, and aliases longer than **4,096 bytes** before
filtering or selecting eligible models. Godex previously accepted 1,025
choices, silently skipped an oversized ID/effort/alias while preserving
other entries, and discarded a valid 5,000-byte effort due to its old
4,096-byte threshold. Source-derived regressions failed first and now pass
using `pingCatalogInputValid04380` before any model selection. A malformed
single batch is rejected and bundled models remain available; a valid longer
(5,000-byte) effort is preserved. The validator examines **every typed**
entry in heterogeneous JSON arrays; invalid-type entries themselves are
skipped as in Rust but may not mask a later overlong string. The tagged
Rust parser also selects the *first string-valued* `slug`/`id` and
`display_name`/`displayName` key (even if that selected text is empty),
instead of falling back to a second key merely because the first string is
blank. Null and wrong-type keys still allow fallback to the next typed
key. `TestProdex04380DynamicModelCache*` and
`TestProdex04380DynamicCatalog*` exercise cardinality, ID/alias/label,
reasoning-effort bounds, heterogeneous types and first-key precedence.
All local ping package and race tests pass. This is a tagged-source/model-
cache regression, not a two-executable PTY interactive picker differential.

## OpenAI dynamic catalog identity — bytewise ASCII-only equality

The same tagged `rich_catalog.mojo` model-identity comparisons fold ASCII
A–Z *byte by byte*, but do not use Unicode case mappings or runewise
shortcuts. Godex's model cache previously used `strings.ToLower` to form
dedupe keys and its `asciiEqualFold` iterated only UTF-8 rune *starting*
indices. This incorrectly merged `book-model` with `booK-model`, and
`böok-model` with `BÖOK-MODEL`, despite the different code points. The
`TestProdex04380DynamicOpenAIModelIdentityUsesASCIIOnlyFold` cache fixture
reproduced those lost model choices, while
`TestProdex04380ASCIIModelComparisonChecksEveryUTF8Byte` checks ASCII
upper/lower success and distinct Kelvin, accented, canonical-composition and
length cases. The fix uses an ASCII-byte normalized cache key, with a
complete byte-index scan for equality, preserving original IDs and sorting.
The previous retired-model regression continues to pass and the full local
`internal/usecase/ping` package race suite passes. This is tagged-source
and locally cached picker behavior; direct interactive terminal comparison
has not been performed.

## OpenAI model catalog retired filter — ASCII-only 0.438.0

The tagged Mojo `prodex_mojo_rich_catalog_openai_retired_v1` trims Unicode
whitespace and uses ASCII-only casefold for the three exact retired model
names: `spark`, `gpt-5.3-codex-spark`, and `gpt-5.3-spark`. Godex's model
picker previously called Unicode-aware `strings.ToLower`, which maps the
Kelvin sign `K` to ASCII `k`. Source-derived regressions reproduced this
false retirement on four identifiers (e.g., `sparK`) plus the resulting
loss from a realistic synthetic `models_cache.json`. Replacing the broad
case mapping with the existing `asciiEqualFold` after `strings.TrimSpace`
retains distinct Unicode names while still hiding exactly the retired
ASCII aliases, including uppercase and Unicode whitespace-padding cases.
`TestProdex04380RetiredOpenAIModelOnlyFoldsASCII` and
`TestProdex04380ModelPickerRetainsNonASCIILookalikesButHidesRetired` failed
before the fix and now pass with the complete local ping package suite.
This is a directly tagged-source/local-model-cache proof; a physical
interactive model-picker comparison against both binaries is a separate
PARTIAL item, not silently inferred from the unit tests.


## Exact 0.438.0 finite-gate checkpoint after model catalog hardening

The four ordinary release surfaces and independently partitioned preflight
were **all verified on the exact same clean** Godex commit
`f63e1111ee6be86828e0457ae4b98401f8fe9c96`. Each release phase
confirmed Prodex tagged source `8cab9794b0e08b2cb8507df8321a8e38ebd6492f`,
published Linux x86_64 binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
and its release manifest:

- `--phase=preflight-tests`: **PASS**, including new source-derived ASCII
  catalog retirement/deduplication and bounded input/view tests, official
  Codex 0.163.0 app-server handshake, routing recovery and provider guards.
- `--phase=preflight-oracles`: **PASS**, 34/34 managed-profile/FD/incomplete
  response executable cases, 14/14 Super/CLI cases, Linux PTY and context.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`, all
  negative mutation controls PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`, all
  negative mutation controls PASS.
- `--phase=websocket`: **PASS 58 upstream attempts**, precommit same-owner
  retry, hard continuation affinity and no postcommit replay checks.
- `scripts/check-cross-build.sh`: **PASS** on the exact candidate for six
  Linux/Darwin/Windows executable targets plus Darwin ARM64/Windows AMD64
  runtime/superexpose/Codex cross-test compilation.

Full `make verify` (format, Go vet, shuffled `-race` package tests,
source-size and production build) also PASS on the exact production source
before this documentation-only checkpoint. The new catalog tests were
reproduced as failing prior to each source-aligned fix and remain part of
pinned preflight; they are **source-derived local cache** tests, not physical
two-executable TUI model-picker proof.

Overall parity is still **PARTIAL**, not an unqualified production 1:1
certificate. The finite synthetic gate does not exercise native Windows/
macOS sessions, live OAuth/three-profile quota and relaunch, actual Codex
0.163.0 foreground/background compaction, independently forced Rust
accepted-stream-clone errors or all provider schema permutations. No
personal auth secrets or external model turns were used in these fixtures.


## Copilot preferred-user candidate ABI limit — 0.438.0

The tagged `prodex_profile_export_copilot_candidate_users_v1` and its Rust
adapter reject **more than 256 raw candidate views** before duplicate
elimination. The preferred `lastLoggedInUser` consumes one view, so 256
logged users without a preferred user is valid, but the same 256 with a
preferred user exceeds the ABI and must fail. Godex's Copilot import
originally deduplicated first and accepted 257 identical logged entries,
then resolved a synthetic token and contacted the HTTP user-info service.
`TestProdex04380CopilotCandidateCountRejectsExcessiveLoggedUsersBeforeAuth`
reproduced that credential-probing side effect before the fix. The updated
`Source.Load` now rejects excessive raw counts before reading tokens,
calling keychain helpers or issuing HTTP requests; valid exact-bound
configurations retain the original successful import path. Additional
`TestProdex04380CopilotCandidateOrderUsesExactHostAndLogin` checks
last-user priority and strict bytewise host/login identity with duplicate
removal, distinguishing case, slash and whitespace variants without leaking
real identity or credentials. Local functional and race tests PASS. This is
not an end-to-end two-executable real Copilot/keychain import certificate.


## Anthropic Responses output block cardinality — 0.438.0

The tagged Prodex 0.438.0 source
`crates/prodex-mojo-core/src/rich/anthropic_messages.rs` checks
`ANTHROPIC_RESPONSE_PLAN_MAX_BLOCKS = 65_536` **before** classifying content
or planning output. Its Mojo `ResponseOutput` operation independently caps
JSON array materialization at 65,536 items. Godex previously accepted a
native Messages response containing **65,537** invisible thinking blocks,
even though the complete response body was smaller than the separate 4 MiB
read limit. That mismatch could create a false successful empty Responses
output instead of a deterministic invalid-plan error.

`TestProdex04380AnthropicResponsePlanRejectsTooManyContentBlocks` explicitly
reproduced acceptance before the fix and now verifies the maximum legal
65,536-block response is accepted while one additional block fails closed
with the tagged `Anthropic Messages response plan failed: InvalidInput`
classification. The quota and auth/profile selectors are unaffected, and no
real provider account is used. This is **source-derived local translator
parity**, not yet a full two-executable native Anthropic Messages
cardinality differential; the test is included in the release preflight.


## Complete pinned 0.438.0 gate — Copilot and Anthropic bounds

The source-equivalent clean Godex commit
`3f821649b9707628fa484b8767b8148e93aff70c` was verified against the
exact official Prodex 0.438.0 tagged source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, published Linux x86_64
binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`,
and its release manifest. Every phase checks these pins independently:

- `--phase=preflight-tests`: **PASS**, including new **256 raw candidate**
  Copilot pre-auth bound and **65,536 block** Anthropic response-output
  planner guard, official Codex 0.163.0 app-server handshake, and all earlier
  local auth/routing/provider and state-integrity sentinels.
- `--phase=preflight-oracles`: **PASS**, 34/34 profile/FD/incomplete-
  response executable stages, 14/14 CLI probes, Linux PTY and context.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`,
  negative mutation controls PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`,
  negative mutation controls PASS.
- `--phase=websocket`: **PASS 58 upstream attempts**, precommit same-owner
  connection-limit retry, hard owner and no-postcommit-replay guards.
- `scripts/check-cross-build.sh`: **PASS**, Linux AMD64/ARM64, Darwin
  AMD64/ARM64, Windows AMD64/ARM64 builds plus Darwin ARM64 and Windows AMD64
  runtime/superexpose/Codex test compilation on the **same** candidate.

`make verify` independently PASS on that clean production source before
this documentation-only checkpoint: formatting, source size, Go vet, shuffled
race tests and executable build. The Copilot and Anthropic additions are
**source-derived local regression coverage**, not proofs from two independent
native-provider account sessions. The finite synthetic release gate is
VERIFIED for those bounded scenarios; full production parity remains PARTIAL
because native Windows/macOS execution, live OAuth/three-profile quota and
relaunch, actual Codex 0.163.0 foreground/background compaction and isolated
Rust accepted-socket-clone failure have not been independently certified.


## Anthropic response JSON recursion depth — tagged 0.438.0

The production Mojo `anthropic_request_value_end` bounds recursive JSON
parsing to `ANTHROPIC_REQUEST_KERNEL_MAX_DEPTH = 128`. Its
`ResponseOutput` operation validates the projected **content-block array**,
not arbitrary unprojected outer provider metadata. Godex originally accepted
a synthetically valid, under-4-MiB Anthropic Messages response with
`tool_use.input` nested 140 levels deep and returned a Responses
`function_call` instead of the tagged kernel's invalid-input error.
`TestProdex04380AnthropicResponseToolInputDepthMustStayBounded` demonstrated
the acceptance before the fix. The new bounded in-memory value-tree check
rejects a depth-129 leaf and depth-140 tool input while allowing a depth-128
leaf, shallow tool calls and literal escaped braces in text. A separate
negative control confirms a deeply nested **outer extension** does not
accidentally invoke the content validator. The request body and emitted
metadata are otherwise unchanged. Local tests and race checks PASS; this is
source-derived native-translation protection, not yet an independent
Prodex/Godex two-executable deep JSON Messages differential.


## Complete pinned 0.438.0 finite gate — bounded Anthropic JSON depth

The exact clean Godex commit
`cdc20d355f2dacd7fd3ee401dc1127353128b024` was compared with the
published Prodex 0.438.0 source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, release manifest, and
Linux executable SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
in the five independent source/binary-pinned release phases:

- `--phase=preflight-tests`: **PASS**, including local signed exit, real
  Linux FD acceptance/release/shutdown, official Codex 0.163.0 app-server
  handshake, Copilot pre-auth candidate bound, Anthropic 65,536-block limit
  and **exact JSON depth 128** with metadata/escaped-text negative controls.
- `--phase=preflight-oracles`: **PASS**, 34/34 managed-profile/
  descriptor-pressure/incomplete Responses executable stages, 14/14 CLI
  probes, and PTY/context oracles.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`, all
  configured negative mutation controls PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`, all
  configured negative mutation controls PASS.
- `--phase=websocket`: **PASS 58 upstream attempts**, same-owner bounded
  precommit retries, hard continuation affinity, no postcommit replay.

`scripts/check-cross-build.sh` additionally PASS on that **same** SHA for
Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 and Darwin ARM64/
Windows AMD64 test compilation. `make verify` (including shuffled Go race
suite, vet, source-size, formatting, build) passed on the identical clean
production source before this documentation-only checkpoint.

**Interpretation:** finite synthetic gate VERIFIED; production 1:1 parity
still PARTIAL. The Copilot/Anthropic bounds remain source-derived local
regressions rather than independently executed two-product live account
flows. Native macOS/Windows runtime, Codex 0.163.0 real compaction, OAuth/
multi-profile quota reset, and isolated Rust accepted-socket-clone failure
remain material unverified surfaces.


## Native Gemini plain-content role normalization — bounded 0.438.0 parity

The tagged Prodex 0.438.0 `prodex_mojo_gemini_content_hardening_v1`
operation 0 coalesces consecutive equal-role contents while preserving their
original JSON part order and first retained row's metadata. Operation 3
repairs the user/model conversation boundaries: a plain model-led transcript
receives `[Continuing from previous Codex context.]`, and a model-ended
transcript receives a final `Please continue.` user prompt. Godex's direct
`geminiGenerateContentRequest` initially emitted separate adjacent user/model
contents and lacked both boundary prompts. Source-derived
`TestProdex04380GeminiNativeContentCoalescesAdjacentRoles` and
`TestProdex04380GeminiNativeContentRepairsModelOnlyBoundaries` failed before
the fix and now pass with full native Gemini regression coverage, race tests,
metadata/primitive-part order checks and non-mutating normalization.

**Scope constraint:** Prodex's complete native pipeline also invokes separate
Mojo operations for missing tool-result pairing, orphan responses, and
response order. Those operations have not yet been qualified in a native
OAuth two-executable fixture. The Go fix therefore applies role coalescing
and boundary shaping to *tool-free* contents only; tool-bearing transcripts
retain the existing established tool order. This intentionally closes a
bounded, source-derived plain-content gap without pretending the entire
native Gemini/OAuth pipeline has been certified. All live native tool-pair
permutations remain PARTIAL/OPEN.


## Complete pinned gate after native Gemini role normalization

All finite release phases exited 0 on the **same clean Godex candidate**
`0fa5adae19bdd520305707f0f4f06bf255b12796`, against the exact official
Prodex 0.438.0 source
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`, release manifest and
published Linux x86_64 binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`:

- `--phase=preflight-tests`: **PASS**, including source-derived Gemini
  tool-free native coalescing and model-boundary regressions, existing
  state/credential controls, real Linux FD recovery and official Codex
  0.163.0 native app-server handshake.
- `--phase=preflight-oracles`: **PASS**, 34/34 executable profile/FD and
  incomplete Responses stages, 14/14 CLI probes, Linux PTY and context.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`,
  negative mutation controls PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`,
  negative controls PASS. These are public API-key Chat-compatible
  executable scenarios; they do not certify native OAuth content pairing.
- `--phase=websocket`: **PASS 58 upstream attempts**, bounded precommit
  same-owner retry and no replay after committed output.

`scripts/check-cross-build.sh`: **PASS**, Linux AMD64/ARM64, Darwin
AMD64/ARM64, Windows AMD64/ARM64 executable builds, plus targeted Darwin
ARM64/Windows AMD64 test compilation. `make verify` passed on the exact
production source before this documentation-only commit: format, source-size,
vet, shuffled Go race suite and production build.

The finite synthetic suite is VERIFIED, but **overall 1:1 production parity
is still PARTIAL**. In particular, native Gemini OAuth tool-response pairing
and reordering, real Codex foreground/background compaction, native
macOS/Windows runtime execution, live multi-profile quota/relaunch and an
isolated Rust accepted-stream clone failure are not proven by these tests.


## Gemini native raw-boundary precedence — tagged source regression

In the 0.438.0 Mojo `gch_coalesce(..., shape=true)` operation, the decision
to create a leading/trailing synthetic user prompt depends on the **raw**
first/last content row before invalid rows or empty `parts` arrays are
filtered. Godex previously decided from the first/last *retained* content,
causing two source-proven semantic differences: a raw `null` followed by
`model` incorrectly gained a leading user prompt, and an invalid lone
`model` content with `parts:false` produced no prompt at all rather than a
single user content combining both synthetic lead and tail parts. Its tool
part-inspection helper also panicked on a null content row.

`TestProdex04380GeminiNativeRawBoundaryPrecedesInvalidRowFiltering`
reproduced the panic before the fix and exercises four exact tagged source
cases, including metadata loss when a synthetic leading prompt merges with a
later valid user content. The updated content normalizer evaluates raw role
boundaries, drops invalid rows only during the retained-content pass,
coalesces synthetic boundary text with the adjacent user group and preserves
primitive part ordering without mutating caller-owned slices. A separate
regression checks empty/custom/Unicode roles and their distinct bytewise
identities. Full Gemini local package and race tests and the finite release
preflight protect these behaviors. Native tool-pairing still uses a separately
preserved adapter path until a two-executable OAuth/native protocol oracle
qualifies the complete operations 1, 2 and 4; this is a bounded source-
derived FIX, **not** a production-wide 1:1 certificate.


## Native Gemini tool result pairing — isolated operations 1/2

The tagged Prodex 0.438.0 compiled Gemini content-hardening kernel performs
operation 1 (`gemini_provider_core_pair_tool_calls`) and operation 2
(`gemini_provider_core_repair_orphan_tool_responses`) after content coalescing.
Its matching key is **both** function `id` and `name`; it does **not**
deduplicate identical missing calls. A call without a matching result
receives an explicit synthetic `functionResponse` saying that tool execution
output is unavailable. A response with no matching call receives a synthetic
`functionCall` with `args:{}` and
`thoughtSignature:skip_thought_signature_validator`. These are metadata
repairs only: the adapter does **not** execute the missing tool or invent a
successful tool result. Null/malformed function objects use tagged fallback
identity `id:""`, `name:"unknown"`.

The four source-transcribed regressions in `native_tool_pairs_04380_test.go`
check missing results with repeated identical calls, wrong names with the
same IDs, a malformed intermediate user row, null functionCall and
functionResponse entries, synthetic orphan calls and repeated standalone
orphans. `native_tool_pair_integration_04380_test.go` also exercises the
actual native request builder for a lone missing result, standalone orphan
and existing matched pair. These tests demonstrate **source-derived local
behavior only**. The integration intentionally avoids applying isolated
operations 1/2 to adjacent same-role tool-bearing entries: tagged Prodex
coalesces those rows before pairing and subsequently performs operation 4
response sorting. Reproducing only part of that compound pipeline would
silently reorder existing client tool histories, so it remains explicitly
**PARTIAL**, pending a proper native two-executable OAuth/provider oracle.


## Native Gemini immediate response ordering — compiled operation 4

The tagged 0.438.0 provider applies
`gemini_provider_core_refine_tool_response_order` (Mojo operation 4) **after**
missing/orphan pairing, but only when an adjacent `model` row contains one or
more `functionCall` parts and is followed by a `user` row. In that eligible
user row, all `functionResponse` parts are sorted ahead of text/media; those
whose IDs match preceding calls use the **first** call index as their rank,
unmatched responses follow, and stable arrival order is preserved within
equal ranks and among ordinary parts. This sorting key is the ID alone;
missing-result matching in operations 1 and 2 uses ID **and** name.

`TestProdex04380GeminiNativeResponseOrderMatchesTaggedSorter` uses the
source's full reversed/duplicate/unmatched/media example. Additional exact
source regressions exercise `functionCall:null`, `functionResponse:null`,
custom non-user rows, same-ID/different-name stability, idempotence and
unchanged rows with zero preceding tool IDs. The actual Go
`geminiGenerateContentRequest` now orders two consecutive tool results
received as `call-b`, `call-a` into their original `call-a`, `call-b` request
order without changing names, arguments or output. Tests pass for complete
matched pairs, synthetic missing outputs and isolated orphan inputs; the
fully sourced local Gemini package and race suite pass.

**Scope:** The tool-bearing adapter applies compiled operations **1 → 2 → 4**
only when there are no adjacent equal-role rows needing prior coalescing.
The tagged complete native pipeline additionally coalesces those compound
rows before matching and then enforces final role shape. Since native OAuth
response-order permutation across all compound histories has not yet been
checked against an official executable, the compound branch stays on its
existing safe ordering path. This checkpoint closes source-derived **isolated
native pair/order behavior**, not all native OAuth tool-history parity.


## Complete finite gate — native Gemini isolated pairing and order

All of the following exited 0 on the **same exact clean Godex revision**
`798ce2bf41212c85a7cd5fc302690ee619c9c003` against Prodex official
0.438.0 tagged source `8cab9794b0e08b2cb8507df8321a8e38ebd6492f`,
published Linux binary SHA-256
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`
and release manifest:

- `--phase=preflight-tests`: **PASS**, tagged native Gemini raw boundary,
  ID/name-based missing/orphan tool pairing, null functionCall/Response,
  immediate tool-result ordering with stable duplicate IDs, negative controls,
  and verified official Codex 0.163.0 app-server handshake.
- `--phase=preflight-oracles`: **PASS**, 34/34 two-executable managed-profile
  cases (including HTTP 401 credential rotation, physical Linux TCP FD
  exhaustion and incomplete response wire), 14/14 CLI probes, PTY and context.
- `--phase=deepseek`: **PASS 22/22**, `comparison_differences: []`,
  mutation sentinels PASS.
- `--phase=gemini`: **PASS 13/13**, `comparison_differences: []`,
  mutation sentinels PASS. These are public API-key provider tests, *not* a
  complete native OAuth/GenerateContent tool-history differential.
- `--phase=websocket`: **PASS 58 upstream attempts**, strict owner affinity,
  bounded precommit retry and no committed replay.

`scripts/check-cross-build.sh` **PASS** on the identical clean source for
Linux AMD64/ARM64, Darwin AMD64/ARM64, Windows AMD64/ARM64 production binaries
and targeted Darwin ARM64/Windows AMD64 test compilation. `make verify`
(including shuffled Go race tests, vet, source-size, format and build) PASS
on the identical production source before this documentation-only checkpoint.

**Overall parity remains PARTIAL, not unqualified 1:1.** The bounded finite
synthetic gate is VERIFIED. Native tool-bearing multi-model conversation
coalescing plus operation-4 order, live OAuth/multi-profile quota and relaunch,
actual Codex foreground/background compaction, macOS/Windows process/TUI
execution and isolated Rust accepted-socket clone pressure remain outside
the demonstrated two-executable coverage.


## Native Gemini compound history — compiled 0.438.0 source oracle

This checkpoint supersedes the earlier same-role tool-history bypass. The
canonical source `crates/prodex-provider-core/src/gemini_bridge/hardening/contents.rs`
requires `Coalesce`, `PairToolCalls`, `RepairOrphanToolResponses`,
`RefineToolResponseOrder`, then `EnforceContentRoleShape`. The corresponding
`mojo/prodex_core/gemini_content_hardening.mojo` operations are **0/1/2/4/3**.

`python3 tools/gemini-content-oracle.py /path/to/prodex-git-repository` archives
only that pinned commit's Mojo sources, compiles its actual kernel as a shared
library, and executes both measured and writing ABI calls for all five passes.
The reference checkout's current HEAD is never used. Mojo 1.1.0 (8189361e)
produced 15 golden cases in `internal/gateway/gemini/testdata/native_compound_04380.json`.
Removing any of the five passes is detected by at least one fixture; all five
negative controls PASS. No credentials, tools or provider requests are involved.

`go test ./internal/gateway/gemini -run '^TestProdex04380GeminiNativeCompoundHistoryMatchesCompiledSource$' -count=1`
failed in **14/14** original subtests before the fix. The complete pipeline now matches
all 15 outputs, leaves caller-owned history untouched and remains idempotent.
Coverage includes consecutive model rows, consecutive user responses, reversed
results, duplicate calls/results, equal IDs with different names, missing and
orphan results, `functionCall:null`/`functionResponse:null`, invalid rows,
custom roles, and mixed calls/results/text/media. Exact equality checks retained
metadata, arguments, thought signatures and existing error payloads.

The gateway applies final role shaping after repair: model-led histories get
the tagged leading context prompt, and synthesized missing-result user rows
already satisfy the final boundary. They do not gain an extra `Please continue.`
or model turn. Signature hardening still runs afterward and injects only the
first function-call part in a coalesced Gemini 3 model row; existing nested
signatures survive. Earlier tests were updated to assert those exact tagged
boundaries and retained following-user text, rather than obsolete row indices.

Production ownership remains in `internal/gateway/gemini`; no shared helper,
new dependency, account policy or retry/replay path was added. The generator
is a development tool, with no production imports. No profile/state format,
credential permissions, CLI flag or provider availability changed.

Validation at this checkpoint:

- `rtk test go test ./internal/gateway/gemini ./tools/differential`: **PASS**.
- `rtk test go test -race ./internal/gateway/gemini ./tools/differential`: **PASS**.
- Source regeneration with the canonical repository: **PASS**, 15 fixtures and
  all five missing-pass negative controls.
- `rtk test make verify`: **PASS**, including formatting, source size,
  Bubble Tea guard, cross-build, installer checks, vet, full race tests and build.
- The five clean-commit release phases are recorded in the following checkpoint
  after they execute.

**Scope:** VERIFIED for these compiled-source native content transforms.
A shared library compiled from exact tagged source is not the official released
Prodex executable. Native OAuth two-executable proof remains **PARTIAL**; managed
Gemini OAuth runtime remains disabled. The public API-key 13-case differential
checks a separate Chat-compatible entry point. Overall production parity stays
**PARTIAL**, including live quotas, full compaction/cancellation and native
Windows/macOS execution.


## Complete pinned finite gate — compound native Gemini `c68ba481`

All five independent phases exited 0 on the same clean Godex commit
`c68ba481e028d07188ca252798b4be8a3ae995b7`, accepted by remote `main`.
Each phase rechecked the canonical Prodex 0.438.0 source commit, release
manifest, official Linux SHA-256 and clean Go build provenance.

- `./scripts/verify-release-parity.sh --phase=preflight-tests`: **PASS**.
- `./scripts/verify-release-parity.sh --phase=preflight-oracles`: **PASS**,
  including 34 profile stages, 14 CLI probes, Linux PTY and context checks.
- `./scripts/verify-release-parity.sh --phase=deepseek`: **PASS**, 22/22 cases,
  zero comparison differences and all mutation controls.
- `./scripts/verify-release-parity.sh --phase=gemini`: **PASS**, 13/13 cases,
  zero comparison differences and the new native compiled-source local cases.
- `./scripts/verify-release-parity.sh --phase=websocket`: **PASS**, 58 upstream
  attempts with strict precommit retry, owner and no-replay checks.

The separately SHA-checked official Codex 0.163.0 Linux binary also passed
`TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList` on this
commit through the opt-in environment. `rtk test make verify` passed before
commit, including all cross-build targets; the independently executed five
phases then verified the committed source. Native Windows/macOS runtime,
live OAuth/quotas and full compaction/cancellation remain PARTIAL/OPEN.


## DeepSeek explicit message roles — typed validation and untyped fallback

Parallel source review found that Godex silently accepted an explicit
`{"type":"message","role":"critic"}` by changing its role to user. Canonical
`crates/prodex-provider-core/src/deepseek_bridge/input_items/validation.rs`
and `mojo/prodex_core/deepseek.mojo` (`deepseek_input_item_validation`) instead
reject it. The same tagged validator rejects tool, empty and uppercase roles
on typed messages; only user/assistant/system/developer are valid there.

The source deliberately returns no role issue when an item has no explicit
`type`. Its separate raw chat-role conversion still defaults an unknown role
to user. Integration review caught both edge cases in the initial worker
patch (`9c249f4beb02f936e81c62d65e4de01eecda802a`): it retained typed tool roles
and also rejected the valid untyped fallback. Additional fail-before cases
proved both differences before the final local correction.

`TestProdex04380DeepSeekResponsesRejectsUnsupportedMessageRole` failed before
the fix with a nil error. Its four role cases now assert the exact rejection
text; `TestProdex04380DeepSeekResponsesUntypedRoleRetainsChatFallback` protects
the separate untyped contract. Existing tool-history coverage now supplies
a proper function_call_output and still verifies the same generated tool
message, call ID, tool name and arguments. No tool result is fabricated.

The correction stays local to `gateway/deepseek` at the existing Responses
input boundary. It adds no exported API, dependency, generic helper, routing
policy, persistence change or credential handling. Public CLI flags and the
separate Chat Completions route retain their contracts.

Focused package and race validation covers DeepSeek and `tools/differential`.
The independent two-executable unsupported-role rejection scenario is a
separate pending checkpoint. These source-based request cases are VERIFIED;
full malformed-input and public status/error-wire parity remain PARTIAL.


## Parallel review follow-up — falsy Gemini tool payloads

Independent review of `c68ba481` found that the source-oracle fixture builder
used Python truthiness to choose a default response payload. The original
14 fixtures contain no falsy payload and remain valid; the builder could not
express additional false/zero/empty/null payloads faithfully. An assert-based
self-check failed before correction. An explicit UNSET sentinel now separates
an omitted fixture argument from `false`, `0`, empty string/object/array, and
`null`. The new fifteenth compound fixture passes all six payloads through the
actual compiled canonical kernel, including response reordering. Exact Go
comparison and all five omission controls PASS. Production Gemini code did
not change in this follow-up; no broader native OAuth proof is claimed.


## Authenticated quota observation — reject rotation before applying state

Canonical `crates/prodex-app/src/runtime_background/probe_refresh/apply.rs`
checks the observation's credential identity before changing the probe cache,
quota snapshots, backoffs or persistence. Godex previously rechecked the
identity only when consuming a recovery receipt, allowing an obsolete success
to enter quota state first. Worker checkpoint `e660c5cf` added the existing
credential-digest port check immediately before applying a successful probe.
The comparison remains constant-time; no credential or digest is printed.

`go test ./internal/usecase/quota -run '^TestProdex043714CredentialRotationBeforeApplyDoesNotStoreStaleQuota$' -count=1`
failed before integration with zero pre-apply identity checks. After correction,
`go test -race ./internal/usecase/quota ./internal/repository/quota` PASS.
The regression rejects stale persistence, revision advancement and recovery
receipts. Startup inherited-profile tests also retain the correct profile
home for both identity checks. This stays in `usecase/quota`, consumes its
existing gateway contract, and adds no helper or persisted format. The source
check closes this observed pre-apply rotation gap; atomic filesystem rotation
through the entire apply interval and independent three-profile executable
proof remain PARTIAL.


## Proxy capture and listener lifetime — deterministic failure recovery

The first full `rtk test make verify` after the DeepSeek checkpoint failed in
`TestProxyForwardsCapturedBodyAfterClientHalfClose`: a false HTTP 200 with an
empty body replaced the upstream 207 payload. The old one-millisecond grace
classified a later read EOF as cancellation even after a closing request's
body had been captured. A 100-run isolated test and the same shuffle seed
could pass, so rerunning alone was insufficient proof of correction.

`TestProdex04380CapturedClosingRequestIgnoresDelayedReadEOF` uses Go's virtual
clock and a controlled transport to reproduce the failure deterministically,
without a wall-clock sleep. It failed before the fix with `context canceled`,
status 200 and an empty body. The fixed delivery finishes a captured closing
server request independently of delayed read EOF, matching canonical
`local_rewrite_request.rs` capture and `local_rewrite_pipeline.rs` dispatch.
The heuristic, timer and per-request goroutine were deleted. Ordinary
non-closing cancellation, incomplete-body rejection, exact status/body and
one upstream attempt remain negative controls. The new test plus all three
existing contracts PASS for 20 repeated race runs.

Worker `38de1ff3` also closed a separate accept-loop gap: canonical
`vendor/tiny_http/src/accept_retry.rs` retries every listener failure until
shutdown, while Go's HTTP server stops on a non-temporary error. A private
listener adapter now marks such errors retryable and retains net/http's bounded
backoff and shutdown ownership. Its scripted regression failed before the fix
and PASS after; shutdown still returns `http.ErrServerClosed`. Existing real
Linux EMFILE tests remain in place. No accepted request is resent.

These protocol changes stay in `delivery/http/proxy`; they add no business
policy, shared helper, configuration, persistence or background owner. The
Go standard library does not perform Rust's accepted-socket clone operation,
so clone-specific fault injection and native Windows/macOS behavior remain
PARTIAL. Complete closing requests intentionally finish after read EOF;
this must not be described as ordinary downstream cancellation behavior.

## Codex interactive interruption — preserve the native first SIGINT

Canonical Codex runtime launches keep an interactive child in the terminal
foreground group. The parent `signal.NotifyContext` consumes the first SIGINT;
that interrupt belongs to the native Codex TUI and must not immediately tear
down the managed session. A later SIGINT or SIGTERM is the bounded launcher
shutdown request. Godex previously treated the first canceled context as an
immediate child interruption.

`TestWaitForSessionChildPreservesInteractiveChildAfterSIGINT` failed before the
fix because the child stopped as soon as the parent received SIGINT. The Unix
session gateway now recognizes the signal context cause only for an interactive
child, waits for the child to handle that first interrupt, then arms a second
signal path and preserves the existing two-second interrupt/terminate bound.
Private process-group children retain the original cancellation path. The test
uses a synthetic child, no Codex account and no model turn; package and race
suites both PASS. Worker commit `db25bf8d` is integrated as `cc0e5e2c`.

The change stays in `gateway/codex`, adds no public option or process wrapper,
and preserves child reaping and socket cleanup. Native Windows signed exit,
macOS terminal behavior and real Codex TUI execution remain PARTIAL.


## Current clean checkpoint after parallel parity fixes

The current candidate contains the Gemini compound pipeline (`c68ba481`),
DeepSeek typed-role validation, pre-apply quota credential checks, proxy
half-close and accept recovery, and Codex interactive interrupt handling
(`cc0e5e2c`). `rtk test make verify` passes on this clean candidate after
formatting, source-size, Bubble Tea, cross-build, installer, vet, full race and
build checks. Focused package and race commands also pass for Gemini, DeepSeek,
Codex, quota, repository/quota, proxy and differential tooling.

The five independent release phases were rerun on clean `221c7d17` and all
passed: preflight-tests, preflight-oracles (34 stages), DeepSeek 22/22, Gemini
13/13 and WebSocket 58 attempts. DeepSeek and Gemini reports contain zero
comparison differences and all mutation controls pass. The parallel
unsupported-role executable scenario remains pending: its source fixture is
valid, but the existing child-exchange harness times out before materializing a
local 400 response, so it was not integrated or claimed as PASS. Native OAuth,
live quota restart, Windows/macOS execution and full production 1:1 parity
remain PARTIAL/OPEN.

## DeepSeek unsupported typed role — exact executable terminal error

The canonical source validates explicit DeepSeek Responses message items before
provider dispatch, but Prodex's runtime local-rewrite boundary presents this
translation failure as a generic terminal `502 upstream request failed`. It
must not expose a retryable 400 translator error or wait for another profile.

Before the correction, the source-derived request regression rejected
`critic`, but the exact executable scenario timed out: Godex made zero upstream
requests and emitted no child exchange because routing did not classify its
400 preparation error as terminal. The gateway now carries this specific local
validation failure through an existing typed error and maps it to the same
terminal 502. Routing recognizes that existing preparation shape and performs
no retry or rotation. Direct request validation remains explicit and safe.

Exact source/SHA-pinned executable evidence on candidate `05f98bbe`:

- official Prodex source `8cab9794b0e08b2cb8507df8321a8e38ebd6492f`;
- official binary SHA-256 `9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`;
- both products exit 2 and return HTTP 502;
- both body `upstream request failed` with text/plain content type;
- both make zero upstream attempts and zero retries;
- differential comparison is empty and mutation negative controls pass.

The fixture is now part of `scripts/verify-release-parity.sh --phase=preflight-tests`.
It uses synthetic credentials and loopback only; no provider request or live
account is involved. The local source and exact executable behavior are
VERIFIED for this typed-role case. Other malformed DeepSeek shapes and live
provider behavior remain PARTIAL.

## Exact DeepSeek local-error gate on `bd8e2861`

`make verify` passes after the DeepSeek terminal-error correction, including
cross-build/test-compile, installers, vet and full race tests. The independent
`--phase=preflight-tests` release partition also passes on the clean commit
`bd8e28617d0db333b771fd8b4261dd274c9c1465`; its new exact Prodex/Godex
unsupported-role differential reports `comparison_differences: []` and
negative controls PASS. This is the first executable proof for this local
translation error; no live DeepSeek request is made.

## Final five-phase gate after DeepSeek terminal-error correction

All five independent release phases pass on clean commit
`d59343ccbdf89e4d1a1d104258f4da8ba5179a21`, which is accepted by remote
`main`:

- preflight-tests: **PASS**, including the exact DeepSeek unsupported-role
  executable fixture;
- preflight-oracles: **PASS**, 34 stages;
- DeepSeek: **PASS**, 22/22;
- Gemini: **PASS**, 13/13;
- WebSocket: **PASS**, 58 upstream attempts.

DeepSeek and Gemini differential reports contain no comparison differences and
mutation controls pass. `make verify` also passes on this candidate. Overall
parity remains **PARTIAL** because native OAuth, live quota/restart matrices,
full compaction/inference behavior, and native Windows/macOS execution are not
proved by this finite gate.

## Codex 0.163.0 incomplete usage marker

The tagged Prodex runtime treats `response.incomplete` as a terminal Responses
event and keeps its token usage for runtime activity reporting. Godex already
forwarded the event bytes unchanged, but its terminal usage classifier omitted
that event type, so native incomplete turns silently lost their usage marker.

`TestProdex04380ObserveRecordsNativeIncompleteTokenUsage` failed before the
fix because `Observe` recorded no token event for a source-shaped
`response.incomplete` payload. The classifier now accepts that terminal type
alongside `response.completed`, `response.done`, and `response.failed`, while
non-terminal progress events remain ignored. Focused routing tests and the
release preflight check pass; the fix does not alter response wire bytes,
retry, quota, or profile selection behavior.

This is VERIFIED for the synthetic source-derived usage marker contract. Live
Codex inference and the wider native compaction matrix remain PARTIAL.

## Codex compaction v2 trigger boundary

The exact Prodex 0.438.0 executable keeps a `compaction_trigger` request in the
context-dependent fallback shape, but does not classify it as reconstructable
full history for invalid `previous_response_id` recovery. With a synthetic
upstream 400, Prodex makes one attempt and returns the original 400 with the
previous response ID intact. The earlier Godex recovery branch made a second
same-owner attempt after deleting that ID, which could turn the same request
into a false 200.

The correction restores the tagged Mojo predicate: only the older
`compaction` item establishes reconstructable history. The new negative
regression `TestResponsesInvalidPreviousIDDoesNotReplayCompactionTriggerHistory`
asserts one attempt, the original 400 body, and the retained previous ID;
`TestReconstructableFullHistoryMatchesCodexRequestShapes` covers the
context-dependent negative shape. A manual exact source/binary probe matched
after the correction. This is VERIFIED for the bounded synthetic recovery
boundary; live Codex compaction remains PARTIAL.

## Complete five-phase gate on `6f4c544e`

After the `response.incomplete` usage fix and the `compaction_trigger`
recovery correction, all five independent release phases passed on the same
clean Godex commit `6f4c544ec146e950c2ced9670d24c27d3a363458`, with remote
`main` at that SHA:

- `preflight-tests`: **PASS**, including the exact DeepSeek typed-role
  terminal-error differential and the two new routing regressions;
- `preflight-oracles`: **PASS**, 34 profile/startup/pressure/incomplete stages;
- `deepseek`: **PASS**, 22/22 with no comparison differences;
- `gemini`: **PASS**, 13/13 with no comparison differences;
- `websocket`: **PASS**, 58 upstream attempts with no postcommit replay.

`make verify` also passes on this commit. The gate is finite synthetic and
loopback evidence; native OAuth, live quota/restart behavior, live Codex
compaction, and native Windows/macOS execution remain PARTIAL.
