# Prodex 0.439.0 behavioral parity

Full 1:1 parity is **not certified**. PASS applies only to the evidence named
in a row. FAIL includes missing current-release evidence; BLOCKED identifies
verification requiring an unavailable external capability. Historical reports
do not qualify the current candidate.

## Exact reference

- Repository: <https://github.com/christiandoxa/prodex>
- Tag: `0.439.0`; commit: `433aad44bb249d5c25a231a6b06c2fa522a39a1f`
- Linux x86-64 executable SHA-256:
  `2dec9565fd94eb59aeb83ae5e7cc4b772db130cbd45258d86a2104c8f4ba186a`
- Release manifest SHA-256:
  `ba2f1f8d3430bd3fe64addd886ebee1207bf59b9db0d0e34b834b0dc7a0008f8`

The clean detached source resolves to the exact release commit. The downloaded
official executable and manifest match the published SHA256SUMS; the executable
reports `prodex 0.439.0`. All six native-platform pins come from those sums.
Only the Linux x86-64 executable has been executed in this campaign.

The previous finite gate targets 0.438.4, documented in
[PARITY-04384.md](PARITY-04384.md). The exact 0.438.4-to-0.439.0 source delta is
566 files, 45,098 insertions and 5,146 deletions. The release includes compiled
presentation, auth-header, provider-config, session and launch-policy changes;
internal migration alone proves no behavioral equivalence. Intermediate
0.438.0/1/2/3 contracts remain recorded in their versioned parity documents.
Their current executable regressions remain in the gate.

## Evidence matrix

| Reference behavior | Godex behavior / difference | Test or fixture | Owner | Result / limitation |
| --- | --- | --- | --- | --- |
| Exact tagged source, binary and manifest | Active oracles pinned to 0.439.0; older identities rejected | Release checksums, version, provenance guards | `tools/*parity`, `scripts/verify-*parity*` | PASS reference provenance; candidate differential pending |
| Custom cache key independent of session header; omitted key stays absent | Raw forwarding exists; explicit new-release comparison pending | Tagged `http_backend_passthrough.rs`; HTTP oracle | `gateway/openai`, `usecase/routing`, HTTP delivery | PASS exact HTTP cases on clean `9a4c2654b`; finite fixture only |
| Merged turn metadata and unknown fields survive precommit rotation | Raw request forwarding and rotation exist | Tagged `http_followups/memory_metadata.rs` | `usecase/routing`, `gateway/openai` | PASS exact HTTP cases on clean `9a4c2654b`; finite fixture only |
| An omitted User-Agent stays omitted | Go's default agent reproduced on both HTTP and WebSocket; shared header preparation now suppresses it | `TestForwardedUserAgentPreservesClientOwnership`; HTTP native-metadata oracle | `gateway/openai` | PASS after fix on clean `9a4c2654b`; HTTP/WebSocket wire regressions and exact-binary HTTP check |
| Repeated root/exec/resume config overrides retain precedence | Scope-aware Codex gateway exists | Tagged `args_resume.rs`; context oracle | CLI runtime delivery, `gateway/codex` | FAIL expanded scoped comparison needed |
| Inline native commands remain prompt text | Native argument forwarding exists | Tagged `args_resume.rs` | CLI runtime delivery, `gateway/codex` | FAIL current executable comparison needed |
| Codex 0.164.0 initialization and thread listing | Shared official fixture covers both 0.163.0 and 0.164.0 | `TestProdex04390OfficialCodex0164AppServerInitializeAndThreadList` | `gateway/codex` | PASS credential-free initialize and active/archived thread lists; no model turn |
| Native metadata-patch and model-instruction ownership | Delegated to official Codex; no wrapper merge | 0.164.0 schema / source qualification | `gateway/codex` | FAIL exact upstream contracts need qualification |
| Failed compaction attempts stay isolated | Native compaction delegated; translated Compact is separate | Native lifecycle and Compact oracles | `gateway/codex`, `gateway/compact` | FAIL no comprehensive 0.439.0 evidence |
| Compressed rollout resume preserves invocation choices | Existing compressed-session support | Session repository and resume tests | Session domain, `gateway/codex` | FAIL current executable qualification needed |
| Hard continuation ownership and no postcommit replay | Existing bounded routing and recovery | Continuation HTTP/WebSocket oracles | `usecase/routing` | FAIL current executable rerun needed |
| Precommit quota/retry, weighted capacity and watchdogs | Existing admission and recovery | HTTP/WebSocket deadline/capacity phases | Routing use case, HTTP delivery | FAIL current executable rerun needed |
| Concurrent durable state and recovery | Atomic stores, journals and conflict checks exist | Profile, restart, ownership oracles | Owning repositories/use cases | FAIL finite coverage must be rerun; formats differ |
| CLI parsing, diagnostics and exit status | Existing profile and CLI matrices | `profileparity`, `clierrorparity` | CLI domain delivery | FAIL current executable rerun needed |
| Terminal shutdown, protocols and saved-session guidance | Existing PTY differential | `sessionfooterparity` | `gateway/codex` | FAIL current executable rerun needed |
| Native TUI semantics | Bubble Tea owns corresponding Godex surfaces | Owning delivery tests, TUI guard | CLI domain delivery | FAIL incomplete interactive differential coverage |
| DeepSeek request/response/stream behavior | Existing finite matrix; schema aliases now fixed | `tools/differential` | `gateway/deepseek` | PASS fourteen alias cases; full383 matrix rerun pending |
| Gemini request/response/stream behavior | Existing finite matrix | `tools/differential` | `gateway/gemini` | FAIL current full-matrix rerun needed |
| Other providers, catalogs, quota and terminal migrations | Existing implementations; changed source needs comparison | Tagged migration baselines and owning tests | Owning gateways/use cases/delivery | FAIL migration inventory not fully qualified |
| Authentication and refresh against real services | Official clients retain authentication ownership | Synthetic auth and lifecycle fixtures | Auth/profile domains, native gateways | BLOCKED live OAuth unavailable; synthetic tests remain runnable |
| Native macOS/Windows runtime behavior | Six cross-build targets and native runners exist | `check-cross-build.sh`, native parity runner | Platform-specific adapters | BLOCKED native execution unavailable here; cross-build is weaker evidence |

## Current checkpoint

- Worktree: `$HOME/.cache/godex-parity-04390-20261023`
- Branch: `parity/04390-20261023`, based on `origin/main` at `cea78671d`.
- Original checkout, its local-only quota commit and untracked files preserved.
- Active oracle identities migrated together; historical parity records kept.
- PASS: `rtk proxy go test -json ./internal/gateway/codex ./internal/delivery/cli/runtime ./internal/usecase/routing ./internal/delivery/http/proxy`
- PASS: `rtk proxy go test -json ./tools/...`
- PASS: `bash -n scripts/verify-release-parity.sh`; workspace oracle Python
  syntax and `git diff --check`.
- Next: commit the pin migration, build the clean candidate, execute 0.439.0
  differentials, and close verified production gaps. No current full-gate or
  native foreign-platform PASS is claimed.
- No release version, tag or package publication is authorized by this work.

## Codex 0.164.0 qualification and HTTP fixture expansion

The official Codex and standalone app-server compressed Linux assets match
both the tagged audit and GitHub release API digests. Decompressed hashes:
`b93f6811d3f653f0fb83be4050d857fb9abbb111dfa2ce160563327daff7f306`
(Codex) and `27e9426359a5b34b8de3c10efd60765924929b6a61ea8d2b8b4e05c00abad6bc`
(app-server). Both report 0.164.0. No installed executable was replaced.

PASS: `GODEX_TEST_CODEX_0164_APP_SERVER_BIN=$ARTIFACTS/codex-0164 rtk proxy go test -race ./internal/gateway/codex -run '^TestProdex04390OfficialCodex0164AppServerInitializeAndThreadList$' -count=1 -timeout=40s`.
The test uses a temporary isolated home, no credentials and no model request.
Its existing 0.163.0 qualification remains available through its original opt-in.

The existing HTTP prompt-cache oracle now includes omitted/custom-key and
merged-metadata rotation cases. Mutation checks PASS, including large-integer
rounding, fabricated cache/session/User-Agent, missing fields, incorrect owners
and extra attempts. Cross-binary execution against the new clean commit is the
next gate; this fixture addition alone is not a runtime parity PASS.

## User-Agent mismatch and root fix

The new exact-binary comparison rejected an invalid initial multi-profile
custom-cache fixture: cache identity may affect placement, so its transport
check now follows the tagged single-profile reference. A separate managed
rotation case retains two independently authenticated Ready profiles. The
single-profile gateway can use cached quota, so that transport check does not
claim to prove an asynchronous two-profile quota refresh.

With those reference conditions corrected, Prodex passes all new cases, while
Godex fails the omitted-agent control with exact body, session and metadata
preserved. The shared OpenAI header preparer let net/http manufacture
`Go-http-client/1.1` after an absent or hop-removed agent. Four wire regressions
FAIL before the fix (HTTP and WebSocket, omitted and hop-removed); explicit and
empty controls PASS. The common preparer now suppresses automatic generation
while retaining explicit client agents and leaving input header maps untouched.

The change stays private to gateway/openai. No new helper, interface, option,
dependency, worker or durable format is needed. The native patch/schema and
base-instruction policies remain owned by Codex. Credential-free schema export
also confirms `responsesapiClientMetadataPatch` on both TurnStartParams and
TurnSteerParams. Live authentication and model inference were not exercised.

The first pin-only commit `c72085ef0` is pushed. Its `rtk proxy make verify`
PASSes, including source/format/TUI guards, cross-builds, installers, vet, shuffled
race tests and the build. Its original full differential gate remains separate
from newer production and cannot qualify this header fix.

## Verified header fix checkpoint

Clean `9a4c2654b` passes the complete HTTP prompt-cache mode against the exact
0.439.0 source, official binary and manifest: eleven cached-token shapes plus
three native metadata cases on each product. All original request, header,
response, authentication, attempt-count and durable-owner assertions remain
active. Single-profile omitted/custom keys and two-Ready-profile metadata
rotation PASS. The driver also rejects a present empty fabricated User-Agent.
Earlier invalid fixture runs and the correctly rejected mistyped candidate SHA
are retained as failures; they are not qualification evidence.

PASS: `rtk proxy go test -race ./internal/gateway/openai ./internal/delivery/http/proxy ./internal/usecase/routing ./tools/continuationparity`
and `rtk proxy go test -race ./internal/gateway/openai ./tools/continuationparity -count=1`.
The clean candidate and driver were built with `go build -trimpath` and the
existing `continuationparity --http-prompt-cache` mode verified source cleanliness
and candidate VCS metadata before execution. Current full tests and make verify
run independently; no complete release-gate PASS is inferred from this mode.

Latest production checkpoint: `9a4c2654b`, pushed normally to `origin/main`.
Next independent gap: source-ordered DeepSeek function schema aliases and the
simple nested-tool wire view; add executable fixtures before fixing it.

## DeepSeek schema aliases: reproduced gap and fix

Fourteen new exact-binary cases compare all three schema aliases in flat,
simple nested and normal reconstructed forms, canonical-field precedence,
selected-null rejection, missing schema and an explicit false strict flag.
Against clean `9a4c2654b`, all fourteen reference invariants PASS; twelve
Godex cases FAIL and two precedence/admission controls PASS. The executable
reports retain original wire, response, authentication, state and exit checks.
Independent mutation tests reject matching corruption in both product runs.

The existing private request-value selector now chooses the first present key
in parameters/parametersJsonSchema/input_schema/schema order. Present invalid
values remain terminal errors. The existing stateless body override also keeps
its original nested tools, matching the tagged compiled request writer; normal
validation and pending-history acquisition still happen first. This preserves
large schema integers and the different normal/simple wire views with two
production-line changes. Both stay in gateway/deepseek and reuse its existing
presence selector; no shared helper, dependency or exported API is added.

PASS: `rtk proxy go test -race ./internal/gateway/deepseek ./tools/differential`.
New clean-candidate executable comparison follows before publication. The
registry expands from DeepSeek369 to DeepSeek383; Gemini974 is unchanged.
These finite matrices do not prove complete provider or authentication parity.

## Native source qualification

The exact official Codex tag resolves to
`6aaac694803004a63c6f393d153c96d22521a6b9`. The tagged Prodex source guard and
its negative self-tests PASS against that clean source: 82 critical files and
101 semantic groups, including the eleven newly required groups. Commands:
`rtk proxy node scripts/compat/check-upstream-baseline.mjs --source "$CODEX_SOURCE" --report "$ARTIFACTS/codex-0164-source-report.json"`
and `rtk proxy node scripts/compat/check-upstream-baseline.mjs --self-test`.
This qualifies the native owners; it is not end-to-end wrapper proof for every
compaction/resume path. The live-account and foreign-platform limits remain.

Clean `9a4c2654b` also PASSes `rtk proxy go test -json ./...` (11,286 passed,
19 optional skipped) and `rtk proxy make verify`. A newer production change
must run its own gates. CI for the pushed checkpoints was observed in progress;
no remote success is inferred. The prior baseline CI failure is being inspected
without waiting for asynchronous runs before pushing completed fixes.

## Native config-scope fix

The exact release applies scope_codex_exec_config_args at every child plan,
not only proxy-configured launches. A recording-child executable comparison
with shared synthetic rollout ownership proves Prodex moves root and exec
config overrides into exec/resume in original override order. Godex's direct
launch left them in the outer scopes. Inline native prompt text already PASSes.
A profile-only rollout was rejected by the shared-session catalog; the corrected
fixture seeds the shared store and verifies the selected original profile home.
That fixture correction is not a production session-ownership change.

The existing gateway/codex scopeModelArguments parser now serves both runtime
child entry points. The low-level Run primitive remains unchanged. Six focused
child-argv regressions FAIL before the fix and PASS after; inline controls PASS
throughout. The existing direct context test still requires exact argv and
unchanged model/context values under the corrected native scope contract.

The context executable oracle adds seven exact-argv cases and mutation guards,
using a fake Codex that does not record maintenance app-server invocations as
model child launches. PASS: `rtk proxy go test -race ./internal/gateway/codex ./internal/usecase/runtime ./internal/delivery/cli/runtime`
and `rtk proxy go test ./tools/contextparity -count=1`. Exact clean executable
qualification follows before publication.

Latest pushed production: `03b58873f`. All fourteen alias scenarios now PASS
on that clean commit against the exact official 0.439.0 executable, with driver
and candidate hashes retained in the local audit. Its full tests and make verify
PASS independently. The full383 DeepSeek phase is still running; no full-phase
PASS is inferred from those fourteen scenarios.

Observed baseline CI failures predate this work: macOS realtime fails in the
reference Prodex process with EOF during session update; Windows session-conflict
fails in the reference durable escaped-surrogate response-owner assertion.
Both native lanes pass ordinary tests/builds. Original job logs were retrieved
through the GitHub API; these reference failures are not waived or reclassified
as Godex successes. Current pushed CI runs remain asynchronous.

The first clean scope executable run additionally identified a separate native
selector difference: `exec resume --last` reaches Prodex's child unchanged,
while Godex resolves it to a UUID before launch. The config-form fixture now
uses an explicit UUID to isolate override scoping. The original --last failure
is retained and remains **FAIL**, requiring its own ownership/concurrency review;
no selector check was weakened or production selector policy changed here.

The scoped override fix on clean `a01bca13f` now PASSes the exact executable
context oracle: all three original context cases and all seven explicit-session
argv cases match on both binaries, including literal tails and native inline
commands. Driver mutation tests retain their expected scoped arrays. The
additional native --last selector difference remains open as documented above.
