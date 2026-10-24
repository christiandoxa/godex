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
| Exact tagged source, binary and manifest | Active oracles pinned to 0.439.0; older identities rejected | Release checksums, version, provenance guards | `tools/*parity`, `scripts/verify-*parity*` | PASS full finite release gate on frozen `1fb3dbe4f`; latest context matrix on clean `48f8a6c31` |
| Custom cache key independent of session header; omitted key stays absent | Raw forwarding exists; explicit new-release comparison pending | Tagged `http_backend_passthrough.rs`; HTTP oracle | `gateway/openai`, `usecase/routing`, HTTP delivery | PASS exact HTTP cases on clean `9a4c2654b`; finite fixture only |
| Merged turn metadata and unknown fields survive precommit rotation | Raw request forwarding and rotation exist | Tagged `http_followups/memory_metadata.rs` | `usecase/routing`, `gateway/openai` | PASS exact HTTP cases on clean `9a4c2654b`; finite fixture only |
| An omitted User-Agent stays omitted | Go's default agent reproduced on both HTTP and WebSocket; shared header preparation now suppresses it | `TestForwardedUserAgentPreservesClientOwnership`; HTTP native-metadata oracle | `gateway/openai` | PASS after fix on clean `9a4c2654b`; HTTP/WebSocket wire regressions and exact-binary HTTP check |
| Repeated root/exec/resume config overrides retain precedence | Shared scoping now covers direct and managed launches | Tagged `args_resume.rs`; context oracle | CLI runtime delivery, `gateway/codex` | PASS seven exact-argv cases on clean `a01bca13f`; native `--last` remains a separate FAIL |
| Native exec --last selects across thread source kinds and native SQLite | Native selection preserved; shared settings carry no owner identity | Verified Codex 0.164.0 native-created rollouts/SQLite, context argv oracle, owning regressions | Session/runtime use cases and runtime CLI | FAIL reproduced again on clean `0eff8135d`; implemented fix awaits clean executable qualification |
| Inline native commands remain prompt text | Native argument forwarding preserves inline commands | Tagged `args_resume.rs`; context oracle | CLI runtime delivery, `gateway/codex` | PASS explicit-session model, permissions and presets cases on clean `a01bca13f` |
| Codex 0.164.0 initialization and thread listing | Shared official fixture covers both 0.163.0 and 0.164.0 | `TestProdex04390OfficialCodex0164AppServerInitializeAndThreadList` | `gateway/codex` | PASS credential-free initialize and active/archived thread lists; no model turn |
| Native metadata-patch and model-instruction ownership | Delegated to official Codex; no wrapper merge | 0.164.0 schema / source qualification | `gateway/codex` | PASS credential-free schema and 82-file/101-group source guard; full model-turn behavior unverified |
| Failed compaction attempts stay isolated | Native compaction delegated; translated Compact is separate | Native lifecycle and Compact oracles | `gateway/codex`, `gateway/compact` | FAIL no comprehensive 0.439.0 evidence |
| Compressed rollout resume preserves invocation choices | Existing compressed-session support | Context oracle: native, Super and local Super legacy rollouts | Session domain, `gateway/codex` | PASS three finite dispatch/file-preservation cases per product on clean `293ccd49d`; no native model turn |
| Hard continuation ownership and no postcommit replay | Existing bounded routing and recovery | Continuation HTTP/WebSocket oracles | `usecase/routing` | PASS full finite release gate on frozen `1fb3dbe4f`; native --last remains a separate selection FAIL |
| Precommit quota/retry, weighted capacity and watchdogs | Existing admission and recovery | HTTP/WebSocket deadline/capacity phases | Routing use case, HTTP delivery | PASS complete finite phases in frozen `1fb3dbe4f` full gate; current production is newer |
| Concurrent durable state and recovery | Atomic stores, journals and conflict checks exist | Profile, restart, ownership oracles | Owning repositories/use cases | PASS pinned profile/restart/continuation projections on frozen `1fb3dbe4f`; formats differ and comprehensive concurrency is unverified |
| CLI parsing, diagnostics and exit status | Existing profile and CLI matrices | `profileparity`, `clierrorparity` | CLI domain delivery | PASS pinned 40-step profile and fourteen-error projections on frozen `1fb3dbe4f`; later quota errors and provider identity cases have named newer evidence |
| Terminal shutdown, protocols and saved-session guidance | Existing PTY differential | `sessionfooterparity` | `gateway/codex` | PASS five Linux PTY cases in frozen `1fb3dbe4f` full gate; broader native platform/TUI equality unverified |
| Native TUI semantics | Bubble Tea owns corresponding Godex surfaces | Owning delivery tests, TUI guard | CLI domain delivery | FAIL incomplete interactive differential coverage |
| DeepSeek request/response/stream behavior | Existing finite matrix; schema aliases now fixed | `tools/differential` | `gateway/deepseek` | PASS all 383 cases on frozen `1fb3dbe4f`; earlier `03b58873f` full383 reference-temporary FAIL remains recorded |
| Gemini request/response/stream behavior | Existing finite matrix | `tools/differential` | `gateway/gemini` | PASS all 974 cases in the completed frozen `1fb3dbe4f` full gate; current production is newer |
| Other providers, catalogs, quota and terminal migrations | Existing implementations; changed source needs comparison | Tagged migration baselines and owning tests | Owning gateways/use cases/delivery | FAIL migration inventory not fully qualified |
| Quota/reset-credit requests respect profile US residency | Codex config port supplies normalized residency | Forty-step profile oracle, gateway regressions | `gateway/codex`, `gateway/openai` | PASS on clean `1fb3dbe4f`; no model-path I/O added |
| Positive reset-credit counts show earliest available expiry | Optional display-only detail GET; raw/runtime unchanged | Pinned reset-credit executable oracle | `gateway/openai`, `usecase/quota`, CLI quota | PASS seven cases per product on clean `cea73bcdd`; finite display coverage |
| Single-profile one-shot quota fails on malformed config or probe errors | Delivery returns the report error before stdout | CLI regression, profile executable oracle | CLI quota delivery | PASS nine failure cases on clean `0209f13a1`; aggregate fixture isolates optional workspace lookup |
| Aggregate quota resolves workspace metadata for an account ID | Optional account-check GET and detail label | Exact tagged collector, quota display oracle and local regressions | Quota use case / OpenAI gateway / CLI quota | PASS 32 cases per product on clean `4f7ece56d`; doctor and raw/runtime isolation covered |
| No generated quota User-Agent when version probing fails | Suppressed Go's automatic agent on usage/credit/workspace requests | Missing-version display oracle and gateway HTTP regressions | `gateway/openai` | PASS 32 cases per product on clean `b1240683a`; known-version construction stays open |
| Codex auth originator override retains valid and empty values | Codex auth-header port supplies originator and residency | Expanded reset-credit oracle and gateway HTTP tests | `gateway/codex`, `model/auth`, `gateway/openai` | PASS seventeen cases per product on clean `a2e14460a`; refresh/consume have local wire coverage |
| Explicit profile can resume a legacy rollout absent from shared catalog | Profile-scoped fallback only for a missing shared target; saved settings remain native | Context executable oracle and session use-case regressions | Session use case, runtime CLI | PASS three context, ten native-argv and eight Super dispatch cases per product on clean `293ccd49d`; `--last` and implicit legacy selection remain open |
| Explicit profiles reject unsupported shared provider identities | Pre-child validation, registry aliases and ASCII-only case handling | Eight executable provider cases plus Kelvin lookalike comparison and runtime regressions | Runtime CLI delivery | PASS complete 29-case context matrix per product on clean `48f8a6c31`; broader external-provider dispatch remains separately scoped |
| Authentication and refresh against real services | Official clients retain authentication ownership | Synthetic auth and lifecycle fixtures | Auth/profile domains, native gateways | BLOCKED live OAuth unavailable; synthetic tests remain runnable |
| Native macOS/Windows runtime behavior | Six cross-build targets and native runners exist | `check-cross-build.sh`, native parity runner | Platform-specific adapters | BLOCKED native execution unavailable here; cross-build is weaker evidence |

## Current checkpoint

- Worktree: `$HOME/.cache/godex-parity-04390-20261024`
- Branch: `parity/04390-20261024`, based on `origin/main` at `0eff8135d`.
- Original checkout, its local-only quota commit and untracked files preserved.
- Active oracle identities migrated together; historical parity records kept.
- PASS: `rtk proxy go test -json ./internal/gateway/codex ./internal/delivery/cli/runtime ./internal/usecase/routing ./internal/delivery/http/proxy`
- PASS: `rtk proxy go test -json ./tools/...`
- PASS: `bash -n scripts/verify-release-parity.sh`; workspace oracle Python
  syntax and `git diff --check`.
- Latest pushed production: `48f8a6c31` (shared provider identities); prior
  gate/session/auth/quota fixes remain qualified. Original checkout is preserved.
- Clean `0209f13a1` PASSes the expanded 49-step profile oracle with the corrected
  failure fixtures. `make verify` PASSes on `30ddd406e`; later changes affect
  only fixtures and documentation. Clean `cea73bcdd` full tests PASS 11,361
  cases with 19 optional SKIP.
- Clean `a2e14460a` PASSes seventeen reset-credit scenarios per product,
  11,398 full tests with 19 optional SKIP, and `make verify`. Clean `1a8d44630`
  PASSes context/native-argv and make verify. Clean `196a27571` PASSes 26 expiry
  scenarios per product and make verify. Clean `e6b54bb23` PASSes make verify,
  HTTP profile-status and raw 429 phases. Clean `4f7ece56d` PASSes 32 quota
  display cases, 49 profile steps, 11,451 tests with 19 optional SKIP and make verify.
  Clean `b1240683a` PASSes the corrected missing-version 32-case oracle and
  make verify. Clean `47b22b95e` PASSes make verify and context/native-argv plus
  both Super dispatch cases. Clean `293ccd49d` PASSes make verify and the
  expanded context oracle with saved settings, provider identities, explicit
  native overrides and compressed rollouts. Clean `48f8a6c31` PASSes make verify
  and the 29-case context/provider matrix per product. Next: native --last
  ownership or known-version auth metadata.
- Frozen `c72085ef0` full gate exits 1 on the Prodex Gemini primitive-control
  invariant after DeepSeek369 PASS. Frozen `1fb3dbe4f` completes the full default
  gate with exit 0, including DeepSeek383 and Gemini974. This qualifies only that
  frozen candidate and finite matrix; full 1:1 and foreign-platform parity remain
  uncertified.
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

## Quota/reset-credit profile residency

The exact 0.439.0 codex_openai_auth_headers_for_home sends the selected profile's
normalized US residency on quota, account-detail and reset-credit requests. A
five-case loopback CLI probe on clean `a01bca13f` proves Godex omitted the header
for both lowercase and padded-uppercase US. Other/absent controls agree. A
separate existing CLI difference remains: malformed config yields Prodex exit 1
and Godex's exit-0 config-error table, with no upstream request in either case.
That CLI difference is **FAIL**, not silently treated as equivalent output.

The integrated profile oracle independently reproduces the US header difference
with private imported homes, one authenticated GET, a valid durable usage
snapshot and successful child shutdown. Its matching-corruption guards reject
missing or fabricated residency in both products.

At `1fb3dbe4f`, QuotaClient consumes a mandatory narrow ReadQuotaResidency contract from
its Codex auth adapter. Codex's original bounded config reader is package-local
and reused by provider inspection and residency. The OpenAI quota and redeem
adapters apply the returned residency before each credential attempt. Invalid
config is terminal before dispatch. The shared model-request hot path acquires
no extra config I/O. The existing composition root still injects CodexProcess;
no new dependency, option, shared helper, worker or persistence format is added.

Six new quota/redeem regressions FAIL before the fix, with absent/other/Unicode
controls passing. PASS after the fix:
`rtk proxy go test -race ./internal/gateway/openai ./internal/gateway/codex ./internal/usecase/quota ./cmd/godex`
and `rtk proxy go test -race ./tools/profileparity`.
Exact clean-candidate runtime qualification follows before publication. Quota
User-Agent/originator and reset-credit expiry presentation remain separate
unverified differences; this header fix does not claim their parity.

## Expanded DeepSeek gate result and retained failure

The original `rtk proxy scripts/verify-release-parity.sh --phase=deepseek` on
clean `03b58873f` completes all 383 cases and exits 1. Every Godex invariant
PASSes. One reference case, custom-input-null, leaves two unrecognized atomic
state temporary files after the Prodex process exits. Its request/response wire,
authentication and exit status otherwise match; the durable-state guard correctly
fails. The full383 result remains **FAIL**. An unchanged isolated rerun of that
exact case PASSes on both products; it does not replace the failed full run.
No temporary-file exception or state-integrity assertion was weakened.
The earlier pin-only clean `c72085ef0` full run completed DeepSeek369 with PASS
and is still executing Gemini974. Its results qualify only that frozen commit.

## Reset-credit expiry: display workflow

Prodex 0.438.4 introduced optional reset-credit details; 0.439.0 retains them.
Godex parsed only the count and did not request or display expiry. The new
pinned two-executable oracle runs seven scenarios: finite offset-aware earliest
expiry, never-expiring credits, HTTP failure, incomplete/invalid details, zero
count and unchanged raw JSON. Every reference scenario PASSes. Against clean
`1fb3dbe4f`, the first Godex case FAILs the required second read-only GET.
An initial fixture with public intermediate directories was rejected by the
reference safety checks; the corrected fixture makes every owned directory
private and retains the safety guards.

The gateway's display-only method retains the accepted usage credential for
its optional 4 MiB detail GET. Parsing ignores unavailable credits, distinguishes
null from missing expiry, rejects incomplete available entries, and chooses the
earliest instant. Optional failures preserve count and quota. The use case
selects this port only for display reports; raw/runtime probes remain unchanged.
Display facts are excluded from serialized model JSON and snapshot formats.
Delivery places the summary beside the quota windows and wraps narrow terminal
rows using existing Lip Gloss; Bubble Tea retains refresh/resize ownership.

Focused race checks PASS for gateway/openai, usecase/quota and CLI quota. They
cover credential continuity, request count/routes, finite/never/unavailable
states, raw/runtime isolation, serialization and 30-column rendering. The shared
read-only quota sender also avoids retrying timeout errors while retaining one
transient retry, matching the tagged transport policy. No model, live account
or real credit consumption is involved. The release gate now runs the oracle
and its matching-corruption self-tests. Full current executable qualification
follows before publication; broader display-layout equality remains unverified.

Clean `cea73bcdd` now PASSes the pinned reset-credit oracle: seven scenarios
per product, with the expected request counts, routes, credentials, residency,
summaries and unchanged auth. Its driver SHA-256 is
`19ccf59a65d3c060262dd84349ceb96ca5e37c99985a84fa160d5c017bdad1fc`.
PASS: `rtk proxy go test -json ./...` (11,361 tests, 19 optional SKIP) and
`rtk proxy make verify`. The commit was pushed normally to `origin/main`.

## Single-profile one-shot quota errors

The exact handle_profile_quota propagates fetch errors, while aggregate reports
retain individual failures. Godex's CLI always rendered the report as success.
A new CLI test FAILs before the fix; the expanded profile executable oracle
independently rejects clean `cea73bcdd` for malformed-config exit 0 after
Prodex returns exit 1. Both products make no upstream request and keep the
profile files unchanged in that reproduced case.

The CLI delivery change returns a single-profile one-shot report error before
writing stdout. The existing use-case error model remains available to aggregate,
watch and doctor consumers. Nine new executable scenarios cover config, HTTP 503
and invalid JSON across selected, raw and aggregate modes, with independent
exit/stream/request/state/credential assertions and mutation guards. Existing
profile fixtures are reused locally; no production helper or dependency is added.
PASS: `rtk proxy go test -race ./internal/delivery/cli/quota ./internal/usecase/quota ./tools/profileparity`.
Clean executable qualification follows before publication.

The initial expanded run passed malformed-config selected/raw/aggregate cases,
then rejected Prodex's ordinary HTTP error text as synthetic private material.
That oracle assertion was too broad: the reference intentionally renders its
upstream error message. The corrected fixture uses ordinary synthetic error text
and continues rejecting credential exposure. The failed run is retained; no
successful fixed-candidate qualification is inferred from it. Exact diagnostic
wording/body equality is outside this finite failure projection. Godex continues
to use its existing body-free diagnostics.

The next run independently exposed an aggregate workspace lookup difference:
Prodex sends an account-check GET after usage when account_id is present, while
Godex does not. That run remains FAIL and workspace lookup remains open. The
failure-only fixtures now omit the optional account ID; both products must send
the same bearer-only usage request with no account header. Existing startup and
residency fixtures still require their account ID. This isolates error handling
without accepting an extra or missing request in either product.

Clean `0209f13a1` now PASSes the 49-step profile workflow using the corrected
driver, including all nine failure cases and the original lifecycle, startup,
residency, gateway and restart checks. Driver executable SHA-256:
`50ea436284d1713456f32469c2d20b4f820776194ad62df7ff54b6b24238d570`.
The corrected driver was built from the working fixture changes; the candidate
source and binary provenance remained pinned and clean. Its seven reset-credit
display cases also PASS. `rtk proxy make verify` PASSes on production commit
`30ddd406e`; `rtk proxy go test -race ./tools/profileparity -count=1` PASSes on
the corrected fixtures. No full release-gate PASS is inferred.

## Strict reset-credit detail fields

The expanded reset-credit oracle adds six malformed known-field shapes and an
unknown-duplicate control. All fourteen reference scenarios PASS. Clean
`0209f13a1` fails uppercase_credits after passing the original scenarios; six
new Go cases independently FAIL before the fix because encoding/json accepts
case-insensitive names and overwrites duplicate fields.

The OpenAI gateway now selects exact known names from validated raw JSON and
rejects a repeated credits/status/expires_at field, including escaped names.
Unknown fields, including repeated unknown names, remain ignored. Malformed
optional details preserve the quota and credit count with unavailable expiry.
The small parser stays private to the owning gateway; no shared helper, exported
type, interface, dependency, persistence or request-count change is needed.

PASS: `rtk proxy go test -race ./internal/gateway/openai ./internal/usecase/quota ./internal/delivery/cli/quota`,
`rtk proxy go test -race ./internal/gateway/openai -run '^TestCredit' -count=1`,
and `rtk proxy python3 tools/reset-credit-oracle.py --self-test`.
Clean fourteen-case executable qualification follows before publication.

Clean `1978eced1` now PASSes all fourteen executable reset-credit scenarios per
product, with driver SHA-256
`3fec17f586b7b59598f87a376a18b3a4b91ff3f1a6f3943bd130ec446180ab56`.
`rtk proxy make verify` also PASSes on that exact clean source. The commit was
pushed normally to `origin/main`.

## Auth request originator

The tagged auth-header builder honors CODEX_INTERNAL_ORIGINATOR_OVERRIDE and
retains valid empty values; invalid HTTP field values fall back to codex_cli_rs.
Godex hardcoded the default on usage, credit-detail, consume and auth refresh.
Six new usage/display/consume HTTP tests FAIL before the fix. All seventeen
expanded reference scenarios PASS, while clean `1978eced1` fails the custom
override after passing the fourteen existing reset-credit scenarios.

The existing mandatory config port now returns originator and residency through
a two-field auth request model. The Codex gateway owns native environment and
HTTP field validation; OpenAI consumes those facts on every credential attempt.
The existing Codex refresh sender reuses the private originator resolver.
There is no cross-gateway concrete dependency, shared helper, new configuration
option, worker or durable format. Empty, custom, invalid/control/UTF-8 values,
credential continuity and the refresh adapter have focused regressions.

PASS: `rtk proxy go test -race ./internal/gateway/openai ./internal/gateway/codex ./cmd/godex`
and the focused Codex header tests, plus reset-credit oracle self-tests.
Clean seventeen-case executable qualification follows before publication.
Quota User-Agent construction remains a separate unimplemented parity gap.

Clean `a2e14460a` now PASSes seventeen reset-credit/originator scenarios per
product. Driver SHA-256:
`94d5f8ac5c4061168a1e4ac7777a160d2ba4b80b627f7d05ee7aaa25d007f8ea`.
PASS: `rtk proxy go test -json ./...` (11,398 tests, 19 optional SKIP) and
`rtk proxy make verify`. Pushed normally to `origin/main`.
Current remote CI runs remain in progress; no green result is inferred.

## Current continuation and compaction checks

Clean `a2e14460a` PASSes these complete pinned executable phases, including
their original credential, wire, retry-count, durable-owner and mutation guards:

- `rtk proxy scripts/verify-release-parity.sh --phase=http-compact`
- `rtk proxy scripts/verify-release-parity.sh --phase=http-session-conflict`
- `rtk proxy scripts/verify-release-parity.sh --phase=websocket-compact-lineage`

These are finite transport/ownership checks, not proof of every native Codex
compaction lifecycle. Frozen full gates on older commits remain running.
Reviewing official Codex 0.164.0 exec --last confirms provider filtering, all
thread-source kinds and latest turn cwd inspection differ from the current
wrapper catalog. The retained --last argument/selection FAIL stays open.

## Explicit-profile legacy rollout resume

A private profile-only rollout and an empty separate shared home reproduce an
exit mismatch on clean `a2e14460a`: Prodex launches exact exec/resume UUID/prompt
in the selected original profile, without copying a rollout to shared storage;
Godex exits 1 before child launch. The integrated context oracle independently
reproduces that FAIL with exact arguments/home and profile integrity checks.

Runtime delivery now carries its explicit profile selection in model/session.
The session use case falls back only after a typed missing-target result,
reusing its existing bounded profile reader, identity resolver and durable-owner
lookup. Shared matches and ambiguity still win, account conflicts still fail,
and public shared listing is unchanged. Local private code owns the fallback;
no helper, gateway, dependency, file-format change or migration is introduced.
The dead private runSessionArgument wrapper was removed while passing selection
through its two real runtime callers. Super is not given an uncertified fallback.

PASS focused race tests for session use case, runtime delivery/use case and the
context oracle; clean executable qualification follows before publication.
Native --last and unselected/implicit legacy resume remain separately unverified.

Clean `7c29b6e4a` PASSes the context executable oracle: three original context
cases and eight exact native-argument cases, including legacy profile resume.
Its make verify correctly FAILs because removing dead code shrank an inherited
oversized run.go without reducing its source-size exception. Session dispatch
now lives with the existing resume delivery in resume_session.go; run.go falls
below 400 lines and its exception is removed. No guard limit is raised. The
clean follow-up must rerun make verify before this session fix is pushed.

Clean `1a8d44630` PASSes the same three context/eight native-argv executable
cases and `rtk proxy make verify`. The fallback and its size cleanup are pushed
normally to `origin/main`. Original failed size validation remains recorded.

## Reset-credit timestamp boundaries

An adversarial nine-timestamp comparison reproduces seven differences from the
tagged Chrono 0.4.45 boundary: lowercase/space separators, leap-second Unix
projection, single-digit hours, comma fractions and invalid offset components.
Year zero and long fractions already agree. Seven Go regressions FAIL before
the fix. All 26 expanded reference scenarios PASS; clean `1a8d44630` fails the
first lowercase-timestamp case after passing the existing seventeen cases.

The private expiry parser now checks Chrono's exact lexical form, normalizes its
accepted separators and maps second 60 onto second 59 before Go's existing
calendar parser. Time selection, count preservation, requests and optional-error
handling remain unchanged. No general time helper or dependency is introduced.
PASS: `rtk proxy go test -race ./internal/gateway/openai ./internal/delivery/cli/quota ./internal/usecase/quota`
and `rtk proxy python3 tools/reset-credit-oracle.py --self-test`.
Clean 26-case executable qualification follows before publication.

Clean `196a27571` now PASSes all 26 reset-credit scenarios per product. Its
`rtk proxy make verify` PASSes. Driver SHA-256:
`b870434e0b50e5de31abd25ca2492394029f196b485e2768700e21f15f16a98f`.
The commit is pushed normally to `origin/main`.

## Release-gate completion reporting

The release script printed its PASS line before the appended HTTP profile-status
and 429-policy stages. Its actual exit code stayed nonzero on failure, but the
message could claim success too early. A shell execution regression FAILs before
the fix for each late-phase failure. The success line now follows every selected
stage. The test executes the actual script tail with deterministic stand-ins,
checking success and failure for all three final HTTP phases without network.

PASS: `rtk proxy go test -race ./tools/continuationparity -run '^TestReleaseGateSuccessFollowsEverySelectedHTTPPhase$' -count=1`
and `bash -n scripts/verify-release-parity.sh`. No oracle assertion, exit policy
or production behavior is weakened. Earlier complete-phase results still use
their collected terminal exit status, not a bare PASS line.

Clean `e6b54bb23` PASSes `rtk proxy make verify` and both complete executable
phases `--phase=http-profile-status` and `--phase=http-429-policy`, with the
success line now last. It is pushed normally to `origin/main`.
Remote CI on `1978eced1` passes Linux and both security jobs; macOS fails in
the official Prodex realtime process with connection reset during session update.
Its diagnostic again records an accepted nonblocking socket returning would-block.
That native reference failure remains FAIL; no timing workaround is introduced.
Windows was still running when inspected. The complete failed macOS log is saved
locally with SHA-256 `4b98477ce6f62790ef1fec380005e255d95cfd4e9e0c6acca7a9f600f7ad56e8`.

## Aggregate quota workspace metadata

Six additional pinned display scenarios cover list/map account shapes, trimmed
names, Personal fallback, explicit map ordering, HTTP failure and invalid JSON.
All 32 reference scenarios PASS; clean `196a27571` fails workspace_list because
it never makes the authenticated account-check GET. Prior failure-only fixtures
still omit account ID and keep their exact request invariants.

The OpenAI adapter adds one bounded optional metadata GET for aggregate reports,
using the profile's current account identity, originator and residency without
inventing the usage-only reserve header. Parsed details match the auth account;
HTTP/parse failures retain its ID. The use case owns explicitly requested
aggregate enrichment, while doctor keeps its existing network behavior,
and CLI delivery owns detail labels, Unicode shortening and safe terminal text.
Raw/single-profile/runtime paths and quota state remain unchanged. Model facts
are excluded from persistence. The existing exact-field parser stays private to
the OpenAI gateway and is reused by the two quota response boundaries.

PASS: `rtk proxy go test -race ./internal/gateway/openai ./internal/usecase/quota ./internal/delivery/cli/quota`
and `rtk proxy python3 tools/reset-credit-oracle.py --self-test`, plus Python
syntax and diff checks. Clean 32-case executable qualification follows.

Clean `4f7ece56d` now PASSes all 32 quota-display scenarios per product and the
49-step profile workflow. PASS: `rtk proxy go test -json ./...` (11,451 tests,
19 optional SKIP) and `rtk proxy make verify`. Display driver SHA-256:
`db0d395ee7b970c757f90476f6a0c4cf8774ceaff92e1c9e160642303d08cdcf`.
The initial profile-driver start raced the candidate build and correctly failed
provenance; a subsequent run after the build PASSes. Both logs are retained.
The commit is pushed normally to `origin/main`.

## Generated quota agent with unavailable native version

The reference omits User-Agent when Codex version probing fails. Godex still
manufactured Go-http-client/1.1 on its own usage, credit-detail, consume and
workspace requests. Nine focused HTTP regressions FAIL before suppression.
The first executable fixture incorrectly assumed /bin/true had no version;
GNU true --version reports a candidate accepted by the tagged version lexer.
That reference-side fixture failure is retained. The corrected failing-version
fixture uses /bin/false: all 32 reference cases PASS, while clean `4f7ece56d`
fails the first agent assertion. Original wire/count/auth/residency checks remain.

Each owning OpenAI request builder now suppresses the fabricated Go default.
No new helper, port, option, process probe or persistence is added. The existing
quota flow has no native User-Agent metadata yet, documented by a shortcut
comment. Full known-version Codex/OS/terminal construction and auth-refresh
User-Agent parity remain separate FAILs; no complete auth-header parity is claimed.

PASS focused gateway/Codex/quota/composition race tests and oracle self-tests.
Clean missing-version executable qualification follows before publication.

Clean `b1240683a` now PASSes all 32 missing-version quota display scenarios per
product and `rtk proxy make verify`. Driver SHA-256:
`a29bbeba6d6f537a7997cfa7906fbcd1aad0676a6733edcf9ff26468d712c55a`.
The commit is pushed normally to `origin/main`. Full known-version agent
construction remains FAIL. The frozen old full gates are making progress through
Gemini974; observed scenario directories reached 885 and 591 respectively.
Neither full run has a collected terminal success result.

## Super explicit-profile legacy resume

A pinned synthetic preflight and recording child reproduce the next gap on
clean `b1240683a`: Prodex reaches exec/resume UUID/prompt in both native and
local Super, with a temporary overlay and unchanged original auth; Godex exits
before child launch because the rollout exists only in the selected profile.
The initial probe lacked the required hooks-list handshake and failed on both
products for different reasons; that exploratory run is retained. The corrected
probe and integrated context oracle reject the original candidate independently.

Runtime delivery now passes its explicit profile through both Super resolution
steps. Its existing launchers consume the resolved report and dispatch standalone
profiles through the existing Super target/lease paths, keeping full access,
Smart Context, synthetic local auth and durable managed-account ownership.
Shared resolution and the bounded session use-case fallback are reused without
changes. Local target dispatch remains a private runtime-delivery function with
three real callers; no helper root, dependency, worker or persistent format is
added. Local Super code moves into super_local.go, reducing super_launch.go to
382 lines and removing its source-size exception.

Both new native/local Go regressions FAIL before the fix. PASS:
`rtk proxy go test -race ./internal/delivery/cli/runtime ./internal/usecase/session ./internal/usecase/runtime ./tools/contextparity`
and the focused prior Super resume/owner suite. Clean candidate verification
follows before pushing. The executable projection covers dispatch, arguments,
credential isolation and overlay cleanup; it does not run a native resume turn
or claim complete generated-argument equality. Native --last and implicit legacy
selection remain open.

Clean `47b22b95e` now PASSes the context oracle: three context, eight exact
native-argument and two Super dispatch cases per product. The strengthened
Super check reads the exact UUID metadata through the overlay's actual sessions
directory; mutation tests reject an inaccessible rollout as well. Driver SHA-256:
`1dc86599ece984fdee0947f87716a89503009786d50f7aac284bb41973cf9192`.
PASS: `rtk proxy make verify` on that clean source and
`rtk proxy go test -race ./tools/contextparity -count=1` on the expanded driver.
The production fix is pushed normally to origin/main. The new driver is wired
into the already-required context stage; no release-gate assertion is weakened.

Frozen `c72085ef0` completes all 974 Gemini cases but exits 1: the reference
numeric-unicode case leaves two atomic state temporary files. Godex's state
invariants PASS, and no other full-run comparison difference is reported.
Full log SHA-256:
`524296a14ab8638ee683e38a514f0e6bbb55c710c2ac45c4a6dc607717a41c04`.
The unchanged isolated numeric-unicode retry PASSes both products with the
original wire, credentials, state and mutation guards. Full974 remains FAIL;
that retry does not replace it. Frozen `1fb3dbe4f` is still running.

PASS: `rtk proxy make verify` on the expanded context driver and mutation tests.
The next adversarial probe keeps the same legacy path but adds a saved turn
model and effort. Both products launch, but clean `47b22b95e` injects those
settings while the reference adds neither. A legacy Kiro or unknown provider
also launches on the reference; Godex instead selects Kiro or rejects the
provider before launch. These saved-setting/provider counterexamples remain
FAIL pending a source-aware fallback fix. The earlier minimal-metadata dispatch
PASS does not qualify them.

## Profile-only session settings stay native

Nine focused delivery cases FAIL before the fix: run/Super/local Super across
saved OpenAI, Kiro and unknown provider identities. The expanded executable
context oracle independently rejects clean `47b22b95e` for injecting saved
model/effort into a legacy run. Synthetic Super probes separately reproduce the
incorrect Kiro selection and unknown-provider rejection, while Prodex launches.
The tagged resume_provider.rs reconstructs settings only from shared reports;
a missing shared identity returns no settings before native profile dispatch.

The session use case now marks a successful explicit-profile fallback in its
existing report, preserving all metadata and durable ownership. This fact is
excluded from JSON and persistence. Runtime delivery skips only wrapper
model/effort/provider reconstruction for that source. Shared reports retain
their existing restoration, unsupported-provider rejection, explicit override
and recovery contracts. No entity, persistence format, helper, dependency or
worker is added. The new marker is a boundary fact, not a filesystem decision
inside delivery.

PASS: `rtk proxy go test -race ./internal/usecase/session ./internal/delivery/cli/runtime ./internal/usecase/runtime ./tools/contextparity`.
The expanded fixture checks nine exact native argument cases and six Super
provider/mode cases, with saved settings, explicit native overrides and mutation
controls. Clean executable and make verify qualification follow before push.

Clean `293ccd49d` PASSes the saved-settings context oracle: three context,
nine exact native-argument and six Super cases per product. Driver SHA-256:
`28fab232a889edc1adc829e25ea14bc08d37fc4438bcdf4796f55a42d45491e6`.
Its clean `rtk proxy make verify` PASSes, including six cross-build targets,
vet, shuffled race tests and build. The fix is pushed normally to origin/main.

## Compressed legacy launch qualification

The context oracle adds a compressed-only legacy native case plus native/local
Super cases. Its existing Zstandard dependency creates deterministic fixture
frames; the child reads the exact original bytes through the overlay link and
checks their SHA-256. All native fixture rollouts now require unchanged bytes,
and both Super storage formats keep the existing credential, shared-copy,
arguments, provider, saved-setting and cleanup assertions.

The first stronger plain-file check correctly rejected the reference: old
synthetic metadata lacked session_id, originator and cli_version, so Prodex
repaired it. The diagnostic confirms only those fields were added. Those failed
runs remain recorded. The corrected fixtures seed complete native metadata;
no unchanged-byte, request, credential or state assertion was relaxed.

Clean `293ccd49d` PASSes all three context, ten native-argument and eight Super
cases per product using the corrected driver. This qualifies three compressed
launch cases, not a native Codex resume/model turn. Driver SHA-256:
`b681a896a68ef0739c8ce0967f69b66f681b7a674607b46fafeda8e51aade81c`.
PASS: `rtk proxy go test -race ./tools/contextparity -count=1`,
`git diff --check`, and `rtk proxy make verify` on the expanded driver.
No production code, dependency, state format or helper root changes are needed.
Frozen `1fb3dbe4f` is still executing Gemini974, observed at 877 scenario
directories; no terminal PASS has been collected. Remote CI for the new pushes
is queued/running; the earlier macOS reference failure remains unresolved.

The final driver also rejects compressed rollout copies in shared storage.
It PASSes the same complete 21-case-per-product context matrix and the focused
race tests. Final driver SHA-256:
`80fe298b18f31060bf56b3693371249ab7d4aabf581892ff154a2336c1bc3a30`.
`rtk proxy make verify` PASSes again after that stricter compressed-copy guard.
Earlier corrected-driver evidence remains valid for its narrower assertion set.

An exploratory six-case quota metadata probe on clean `293ccd49d` confirms
five existing generated-agent differences and the leading-v rejection control.
The native version lexer accepts 0.alpha.1, uses the rightmost digit/dot token
(2.7! becomes 2.7), and omits an agent for v0.164.0. Terminal padding is preserved
as underscores, and each Unicode scalar projects to one underscore. The
reference's standard local prefix includes Zorin OS 18.0.0 and x86_64.
Godex still omits the agent when a usable native version exists. The copied
exploratory driver retains its other request/count/auth/residency/profile checks
while recording this one header projection; it is not a release-gate PASS.
No production metadata implementation has been added. Native known-version
User-Agent construction and auth-refresh parity remain open.

## Native --last counterexample with official Codex

The seventh exploratory fixture uses the SHA-256-verified Codex 0.164.0
executable to create two native rollouts and their SQLite state against an
owned loopback backend. Every model attempt stops on a synthetic HTTP 400;
there is no live service or paid inference. The older rollout's source becomes
CLI while its original timestamps are preserved. The newer one remains exec.
Both products share the normal profile-to-shared sessions directory link.

Prodex forwards --last and the native request's prompt-cache key identifies the
newer exec thread. Godex instead resolves the older CLI thread and its native
request carries that older key. Both use the correct synthetic credential and
keep auth.json unchanged. This is a selection/continuation FAIL, not merely an
argument-format difference. Result SHA-256:
`34c713371cf58a0680a1a81d0accec6dab6f66631b5caa45a88a6d47f27c3a27`.
The probe's status remains EXPLORATORY: the backend intentionally rejects turns,
and this is not a complete ownership or recovery qualification.

Earlier fixture attempts remain unqualified: native 0.164 rejects overriding
the built-in provider table; OPENAI_BASE_URL is not its inference override;
hand-written histories did not establish the intended native thread-list state;
and date-scoped seed discovery missed Codex's local-date directory. The corrected
probe uses top-level openai_base_url, native-written state and full-tree seed
discovery. A timed-out earlier fixture required terminating its owned Python
process and orphan Codex helper; the cleanup was restricted to that exact
synthetic CODEX_HOME. No owned native probe processes remain. A loopback server
connection-reset diagnostic after the corrected run does not change its captured
request projection; no successful model turn is claimed.

Do not globally remove --last rewriting without qualifying native provider and
SQLite selection, the latest turn's cwd, profile leases, durable ownership and
recovery. Official exec uses all thread-source kinds, filters the configured
provider even with --all, and treats a usable SQLite candidate as authoritative.
An explicit standalone API-key launch can be investigated separately from
managed-account routing, but optional wrapper settings and empty catalogs still
need the tagged reference behavior.

The frozen `1fb3dbe4f` full gate now emits a Gemini PASS report for all 974 cases
with no comparison differences, then advances through its WebSocket checks.
Its extracted Gemini report SHA-256 is
`c57d82b8e406c121ed6f24323b117e9f6f6bbdec8b52210f4e718ca93492fbed`.
The full gate remains running; no overall terminal success has been collected.

## Shared-session provider validation

An eight-case pinned recording-child probe reproduces three explicit-profile
mismatches on clean `293ccd49d`: Prodex rejects prodex-openai,
prodex-openai-compatible and a synthetic unknown identity before child launch;
Godex launches all three. Five accepted OpenAI aliases launch on both products.
The tagged provider_registry.mojo accepts the general provider aliases plus its
specific native IDs; the two plain Prodex OpenAI IDs are absent.

New delivery regressions FAIL before the fix for that explicit-profile bypass,
missing registry aliases, retired identities and a Unicode Kelvin-sign lookalike.
Runtime delivery now validates a shared report's identity before explicit profile
launch unless the user supplied a native model_provider override. Profile-only
fallbacks keep the previous stage's native configuration policy. The existing
private mapper now follows ASCII-only registry aliases and deliberately preserves
Godex-written native IDs for existing sessions. Its parser and small plan type
move to resume_provider.go; no model, persistence, dependency or helper root is
added. Unsupported errors stay generic and do not expose the metadata value.

PASS: `rtk proxy go test -race ./internal/delivery/cli/runtime ./internal/usecase/runtime ./internal/usecase/session`.
The focused tests require pre-child rejection and no profile lease on failure,
explicit-override precedence, known aliases, Unicode trim/ASCII case behavior and
unchanged Godex identity compatibility. Clean executable and make verify checks
follow before pushing; broader external-provider dispatch stays independently
qualified rather than inferred from alias recognition.

Clean `48f8a6c31` PASSes `rtk proxy make verify` and the expanded context oracle:
three context, ten exact native arguments, eight Super and eight shared-provider
cases per product. The same driver rejects clean `293ccd49d` at the first retired
Prodex identity after the existing cases pass. Driver SHA-256:
`057124eb92c925b844a72f133cb26fc2bceba3c373e747727334d2a31ea279ea`.
A separate two-product wire-to-child probe verifies the Kelvin-sign identity:
before the fix only Godex launches; after it both reject before child launch.
The production fix is pushed normally to origin/main. PASS:
`rtk proxy go test -race ./tools/contextparity -count=1` and
`rtk proxy make verify` on the expanded driver and its mutation controls.
The complete native metadata builder stays private to the developer fixture
package and has three real consumers; no production helper is introduced.

## Collected full release-gate result

The long-running frozen `1fb3dbe4f` default gate now exits 0 after every selected
phase, including the late HTTP profile-status and raw 429 stages. Full log
SHA-256: `45042f990831807a328a41783d68b04111c002ff47b63bf4f406b9e3244147d8`.
Its structured reports include DeepSeek383 and Gemini974 with no differences,
five terminal cases, forty profile steps and fourteen CLI error probes. Original
wire, credential, state, retry, ownership and mutation guards remain active.
This is a complete finite gate PASS for that old frozen source. It does not
replace the earlier failed runs or certify newer production, native --last,
known-version auth metadata, other providers, native foreign platforms or all TUI
behavior. A new frozen current-candidate gate can now run independently of
continued development; no failed invariant was waived to collect this result.

## Native last delegation

The previous current-candidate gate is terminal: its log ends with PASS for all
selected phases on clean `0eff8135d`. No matching gate process remains. This
qualifies that frozen candidate's finite matrix; it does not certify native last
selection or newer production.

The official Codex 0.164.0 loopback fixture reproduces the last-selection failure
again on clean `0eff8135d`: Prodex selects the newer exec thread; Godex rewrites
the selector to the older CLI thread. Both retain the correct synthetic key and
unchanged auth file. Three focused delivery regressions also fail before the fix,
covering shared settings, an empty native catalog and explicit setting overrides.

Native resume last selection now stays with Codex, including flags following
prompt text. The session use case restores only shared provider/model/effort
settings and returns no session ID, home or upstream owner. Its metadata view
includes exec threads and excludes archived/subagent reports. Delivery preserves
explicit overrides and provider validation. Managed launches use the active or
selected home, preserve launch options and retain their existing routing pool;
actual request affinity remains routing-owned. Super retains its overlay, leases
and local transport. Fork last remains a separate catalog-owned path.

The changes stay in CLI runtime delivery and session/runtime use cases. Private
parsers remain local; no helper root, gateway, dependency, durable format or
worker is added. One unused standalone-profile wrapper is removed.
Validation and exact executable qualification follow before push. Native
cross-platform execution and comprehensive last-selection recovery remain
unverified; this change alone does not establish full parity.
