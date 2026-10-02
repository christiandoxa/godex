# OpenAI/Codex core parity audit

Reference: exact Prodex tag `0.435.0`, commit
`caaf2e1998c8cedccb0d1e527d42c39ef3bef83b`. The tag's `Cargo.toml` declares
`0.435.0`, and its compatibility audit names Codex `rust-v0.160.0` at commit
`a956835d020762cb2b570053af06f643a11c0ecc`. Comparison reads use tagged Prodex
Git objects rather than the mutable Prodex checkout. The audited Codex commit is
not present in this local checkout, so it is treated as release-audit evidence
rather than a locally re-executed source snapshot. Historical core-closure
evidence below that names `0.434.2`/`0.159.2` remains evidence for that earlier
checkpoint, not the current parity target.


## 1:1 parity expansion

The project target is now feature-for-feature parity with Prodex `0.435.0`, not
only the historical OpenAI/Codex core boundary. The core closure below remains a
verified baseline while additional surfaces are implemented. Current expansion
checkpoints add standalone/external profile registration and copy workflows,
`--profile` runtime selection, active standalone `CODEX_HOME` launch, and
persisted redacted runtime activity powering `info`, `status`, and `log`, plus
OpenAI profile bundle export/import compatible with Prodex plain v1, encrypted v2
Argon2id/AES-256-GCM-SIV, and legacy encrypted v1 PBKDF2-SHA256 envelopes.

A feature is counted as closed only when its observable behavior is implemented
and covered by local verification; a command-name stub does not count as parity.

## Final closure decision

Practical parity is reached for the declared OpenAI/Codex account, isolated
profile, session, and foreground managed HTTP/SSE runtime scope. The preserved core audit read the then-current tagged Prodex source alongside
Godex production code, callers, tests, and Codex 0.159.2. The active expansion
baseline is now Prodex 0.435.0 / Codex 0.160.0; the historical audit found and
closed these gaps:

- Import identity now comes from the same credential snapshot that is staged,
  so concurrent native credential replacement cannot associate another account's
  credentials with stale metadata.
- Transaction journals retain the original existence result. Recovery removes
  uncommitted new homes or restored auth when no original existed.
- Launches re-read eligibility after acquiring profile leases. A concurrent
  disable cannot leak stale enabled accounts into the child or routing pool.
- Native queue UUIDs/prefixes resolve the rollout home, preserve `--thread` and
  `--thread=VALUE` forms, and reject conflicting account selection. Native names
  remain profile-local and bypass fresh-work quota selection.
- Native debug app-server model diagnostics and app-server daemon/proxy modes
  fail before launch because they discard managed overrides or escape the
  foreground proxy and profile lease. App-server option values cannot hide those
  commands from the guard.
- Body-read errors during bounded non-stream inspection fail before commitment,
  rather than producing successful truncated responses.
- Fresh SSE startup metadata is inspected within a byte ceiling. Quota failures
  before output can rotate; output, unknown events, and the ceiling stop this
  guard. Failed startup attempts do not claim conversation ownership.
- SSE ownership parsing runs incrementally throughout forwarding, including
  late multiline data and CR/LF framing. Oversized events are skipped for metadata
  without changing forwarded bytes; later events still recover ownership.
- Doctor uses the same isolated version/configuration capability probe as launch
  and rejects unsupported Codex runtimes without submitting model work.
- Single-profile `quota --raw` now emits the bounded upstream usage JSON, matching
  Prodex's non-watch raw inspection path without adding live polling or provider filters.

Full native-home import remains a deliberate difference. Auth-only first import
leaves the source settings, rollouts, and databases intact; existing managed
state survives repeat import/login. Copying live Codex databases or sharing homes
is not required for isolated-profile correctness.

## Implemented practical parity

| Capability | Godex implementation and observable coverage |
| --- | --- |
| Isolated ChatGPT accounts | Official Codex interactive/device login, per-account homes, identity deduplication, deterministic and unambiguous selectors. Account/auth tests cover registration and selection. |
| Safe profile lifecycle | Repeat login/import replaces credentials while retaining native state. Metadata-only journals recover interrupted operations; owned OS locks and shared profile leases exclude concurrent credential mutation/removal. Account repository tests cover recovery, leases, and unsafe paths. |
| Account retention and native auth | Enable/disable retains the home. Managed status/logout bypass quota/rotation; logout uses an exclusive lease. Unsafe mutating auth passthrough is rejected with managed-command guidance. |
| Session discovery and launch | Bounded metadata catalog, list/current filters, text/JSON/ID/resume-command output, unique prefixes, and bare UUID resume. Native resume/fork, including nested exec forms and root options, resolve the rollout home; local deletion/archive stays local. Delivery/session/runtime tests cover argument preservation and selector conflicts. |
| Quota and fresh selection | One-shot compact/detailed usage windows, single-profile raw JSON, reset timestamps, fail-open probe uncertainty, deterministic bounded selection, and temporary exhaustion deadlines. Explicit selectors remain fixed. Quota/runtime tests cover exhaustion, uncertainty, and reset eligibility. |
| Managed Codex configuration | HTTP/SSE Responses provider keeps native account/bootstrap HTTPS. Managed config enters the innermost exec scope; user overrides retain precedence. Routing/auth-store overrides, quoted/equals forms, whole provider tables, OSS/local providers, and remote app-server routing cannot bypass it. The Codex delimiter preserves literal arguments. |
| Durable conversation ownership | Hashed, bounded, versioned bindings survive restart/cache expiry. Requested owners beyond cache capacity resolve correctly. Independent routers serialize first-owner selection under an OS guard. Native picker/name/last resumes keep the rollout home and the enabled owner pool; explicit account scope remains fixed. |
| Safe HTTP rotation and streaming | Selected bearer and ChatGPT routing ID replace caller credentials. Bounded retries occur only before commitment. Known continuations preserve their owner; unknown opaque continuations fail closed. Streams flush and preserve bytes/headers/trailers; committed failures abort downstream without replay. HTTP/routing tests cover chains, concurrency, restart, and real broken streams. |
| Native process and installation surfaces | Native Codex owns models, tools, sandbox/approval behavior, session replay, queue execution, agents, mcp-server, app-server, exec-server, and token refresh. Godex preserves safe foreground arguments and meaningful child exit status; routing escapes fail before launch. Existing checksum installers and release naming remain intact; no installation/release files changed in this audit. |

## Gaps closed by these checkpoints

- Strict capability checking previously used a command that rejected
  `--strict-config`. It now uses `exec-server --listen stdio`, closed stdin,
  a temporary home and working directory, and a ten-second deadline. Relative
  executable paths resolve before changing the probe directory.
- Exec config placement and routing/credential override detection now cover
  nested resume/fork/review, quoted keys, equals forms, and literal delimiters.
- Root option values and generated config previously hid native commands from
  dispatch. Session intent now crosses delivery into the use case through
  `model/session.Launch`, without reparsing CLI placement in the use case.
- Native picker resumes previously restricted the upstream pool to the rollout
  home and applied fresh-work quota selection. Home and pool are now independent.
- Loading more durable bindings than the cache could evict the requested owner.
  Only requested bindings are loaded into the bounded cache.
- Independent processes could execute one fresh conversation on different
  accounts before discovering a persistence conflict. First-owner lookup,
  upstream selection, and durable commitment now share a repository OS guard.
- Native Codex 0.159.2 rejects the older
  `reminder_interval_model_requests` field emitted by Prodex 0.434.2.
  Godex's interval flag now emits `reminder_interval_seconds`; its unit is
  documented as seconds. Out-of-range TOML integers and invalid token weights
  fail at delivery, and large valid rollout reminder percentages do not overflow.

Argument parsing stays private to its transport/integration owner. Routing
policy stays in `usecase/routing`, persistence/OS guards in
`repository/routing`, and native launch in `gateway/codex`. The existing
`helper/lockfile` supplies the shared technical primitive; no new shared helper,
dependency, daemon, or background worker was introduced by those checkpoints.
Final closure adds only `helper/sse`, a technical framing parser with real
consumers in routing and HTTP delivery. Quota classification stays in the routing
use case; HTTP delivery owns stream commitment and forwarding. Import decoding
stays private to the Codex gateway, journal recovery stays in account persistence,
and eligibility validation stays in the runtime use case.

## Remaining 1:1 parity gaps

- Prodex `0.435.0` is now the exact parity baseline. Its audited Codex target is
  `rust-v0.160.0`; the accepted minimum remains 0.153.2. The 0.435.0 audit reports
  no model-transport change, while adding upstream invariants around explicit
  provider catalogs, provider/history restoration, projectless defaults, and
  subagent environment inheritance. Godex must preserve those Codex-owned behaviors.
- The 0.435.0 provider-error delta is covered locally: `rate_limit_error` maps to
  rate limiting, `not_found_error` maps to model-not-found, and
  `overloaded_error` / `server_is_overloaded` map to transient overload with the
  same cooldown class. The broader 0.435.0 precommit change is policy relocation
  into Mojo; its observable retry/commit semantics remain the Godex routing target.
- All Godex equivalents of Prodex TUI surfaces must use Bubble Tea. Live status,
  quota watch, log stream/upstream, and redeem confirmation have been migrated;
  non-TTY fallbacks remain line-oriented.
- Multi-provider runtime/login parity remains incomplete for Gemini, Kiro,
  and AGY where present in Prodex. DeepSeek now has the 0.435.0 raw-key runtime
  plus its dedicated Codex model catalog and advanced request-side Responses
  adapter: exact key precedence/provider defaults/stable key rotation, launch-model
  catalog precedence, reasoning effort, primitive sampling/token controls,
  stop/logprobs/user normalization, JSON mode, message/tool replay, RTK tool
  arguments, strict-schema normalization with config.toml-over-env precedence,
  named tool choice, `pro/flash` model fallback, Chat/Messages passthrough,
  native DeepSeek Messages URL/auth, local Models emulation, and local Compact
  fallback are implemented. DeepSeek-specific response/SSE reasoning/tool shaping,
  web-search modes, and beta-base routing remain parity gaps. The Prodex local OpenAI-compatible
  `--url` runtime surface is implemented: Godex validates credential-free
  HTTP(S) endpoints, normalizes root URLs to `/v1`, generates the exact
  `prodex-local` Responses provider config/default model/context/compact
  settings, preserves later Codex argument precedence, and launches directly
  without quota/account/provider rotation. Anthropic now has both managed
  Claude OAuth and raw API-key runtime paths. The OAuth path has a foreground
  runtime bridge:
  the private .credentials.json token is resolved per profile, expired OAuth is
  refreshed through bounded claude auth status --json, default launches form a
  selected-first credential pool, unusable profiles are filtered, and explicit
  profile selection remains hard-affinity. Responses use the shared 0.435.0
  Responses-to-Chat compatibility contract with exact Anthropic alias/fallback
  ordering before credential rotation; auth failures and bare 429s do not advance
  models. Buffered JSON and live SSE are translated back to Responses, Chat and
  Messages stay passthrough, Models list/single are locally emulated from the
  verified 0.435.0 IDs/aliases/context/endpoint metadata, and Responses Compact
  uses the same bounded local-fallback summary and degraded headers as the
  reference without an upstream model call. Raw Anthropic API-key parity is also
  implemented: `--api-key` overrides `ANTHROPIC_API_KEYS`, which overrides
  `ANTHROPIC_API_KEY`; plural keys preserve reference separators and rotate via
  stable synthetic routing identities, custom `--base-url` is credential-free
  HTTP(S)-validated, provider secret env is scrubbed from the Codex child, and
  native Messages uses `x-api-key` while translated/chat routes use bearer auth.
  Copilot now
  has a foreground Responses bridge with external credential resolution,
  direct/legacy runtime auth, Prodex-compatible request/header policy, private
  model catalogs built from the exact 0.435.0 static provider data plus account
  `/models` metadata, and managed multi-profile credential rotation. Default or
  active-profile launches prefer the selected profile first, filter unusable
  credential profiles, rotate only before commitment, and retain durable
  continuation affinity through the existing routing layer; explicit profile
  selection remains single-profile hard affinity. Copilot model fallback now also
  matches the 0.435.0 pre-commit policy for the native Responses path: the exact
  alias chains are bounded, only quota/rate-limit/transient/not-found classes may
  advance models, bare 429 and auth failures do not, and buffered non-retryable
  error bodies are preserved. The reference resolves Copilot runtime auth at
  launch rather than refreshing it per request, so that is not a remaining gap.
  The reference custom-instruction merge helper is test-only for the native
  Responses flow and is likewise not a production parity gap. Copilot endpoint
  parity now matches the registry as well: Responses is native, compact/chat/
  messages are passthrough through the same bounded transport/fallback policy,
  and GET models list/single are locally emulated from the merged static/dynamic
  catalog with exact case-folded, no-trim identity lookup.
- Super mode and its hidden expose/broker/MCP bridge/sub-agent execution stack,
  including optional Presidio integration.
- The standalone gateway surface, remaining live TUI parity, process/resource
  metrics, audit-log backend, and richer runtime-policy diagnostics. Doctor now
  supports install checks, bounded runtime tails, quota summaries, runtime JSON,
  and redacted private bundles; import-journal repair, full session-index repair,
  and policy suggestions remain. Existing status/quota/log, doctor panels, and
  redeem-confirmation, human session-list, and profile bundle password TUIs use
  Bubble Tea; the login/provider menu remains to be implemented with Bubble Tea
  once multi-provider login bridges land.
- Explicit self-update, best-effort cached update notices on eligible commands,
  manual reset-credit redemption, and cost-bearing `ping openai` diagnostics are
  implemented.
- Built-in Claude import is implemented with `CLAUDE_CONFIG_DIR`/`~/.claude`
  source resolution, bounded regular-file checks, Anthropic identity deduplication,
  Prodex-compatible unique naming, private managed `.credentials.json`, and
  create/update/activate semantics. Anthropic bundle export/import now also
  matches the 0.435.0 wire contract: empty `auth_json`, provider metadata, and a
  validated `.credentials.json` provider secret file survive plain/encrypted
  round trips; update rollback restores prior provider metadata and credentials.
  Kiro bundle export/import also preserves the full provider identity fields,
  required `kiro_auth.json`, optional validated `kiro_model_catalog.json`, and
  absent-file rollback semantics. Built-in Kiro import is implemented against the
  current read-only Kiro/Amazon Q SQLite state, with Prodex auth-key priority,
  profile identity matching, bounded `whoami`/model-list metadata calls,
  normalized model snapshots, and non-fatal catalog-refresh warnings. Copilot
  bundle metadata is also implemented with Prodex's empty `auth_json` / no-secret
  wire contract while preserving host/login/API/SKU/plan metadata and profile
  email. Built-in Copilot import is implemented with commented-config parsing,
  config/keytar/libsecret/SDK credential fallback, bounded user-info enrichment,
  trimmed host+config-login identity matching, Prodex-compatible naming, and
  tokenless profile persistence. Process-crash lifecycle-journal recovery for
  multi-profile imports also remains
  to match Prodex exactly. OpenAI plain/encrypted bundle wire formats, Bubble Tea
  protection/password prompts, and identity-safe runtime rollback are implemented.
- HTTP/SSE model transport is explicit; Godex does not implement Prodex's
  WebSocket/Realtime forwarding. Unexpected upgrades fail before upstream work.
- Import-current is auth-only, not full native-home migration. Existing native
  configuration, rollouts,
  history, and databases remain owned by Codex; homes are not symlink-shared.
- Remove keeps its existing destructive contract. Disable and logout provide
  retained deactivation; no second archive tree or changed removal default.
- Native names/pickers/last remain profile-local rather than a shared-session UI.
  Explicit UUIDs/prefixes provide the cross-profile workflow.
- Profile bundle protection/password interaction now matches the Prodex terminal
  split with Bubble Tea: export protection defaults to protected on Enter, masked
  password + confirmation are required when no export-password env value exists,
  and encrypted import prompts only after the bundle requires a password. Non-TTY
  mode still requires explicit flags/environment and never silently writes plain
  reusable credentials.
- Human `session list/current` now follows the Prodex TUI split using Bubble Tea:
  short terminal lists render inline and return; longer terminal lists use a
  scrollable alternate screen with j/k, arrows, PgUp/PgDn, Home/End, and
  q/Esc/Enter exit controls. JSON, ID-only, resume-command, and non-TTY outputs
  remain unchanged.
- Doctor expansion now covers the observable 0.435.0 diagnostics that have real
  Godex data sources: `--install`, `--runtime`, `--quota`, 128 KiB default bounded
  `--tail-bytes`, `--runtime --json`, and `--bundle [PATH] --redacted`. Bundle
  runtime events omit account IDs, quota diagnostics omit identity/email and raw
  gateway errors, and file output is private/atomic. Unsupported repair/policy
  actions fail explicitly until their owning subsystems land.
- `update` now matches the standalone Prodex self-update contract: five-minute
  latest-release cache, short GitHub redirect probe, semver/no-downgrade decision,
  exclusive install lock with actual-binary re-probe, embedded installer execution,
  and bounded secret-redacted installer output. The embedded Godex installer keeps
  its release archive/checksum verification path. Eligible commands also use the
  same five-minute cache for best-effort update notices; info/log/ping/update,
  raw quota, and JSON/bundle doctor surfaces suppress them like Prodex.
- `ping openai` now matches Prodex's explicit cost-bearing diagnostic surface:
  profile/model/base-URL/no-proxy/JSON options, 45-second timeout, four-worker
  cap, completion-order human rows, stable nullable JSON fields, failure taxonomy,
  private diagnostic CWD, provider-secret environment stripping, and bounded
  redacted failure detail. It is never invoked implicitly. Large-model context
  enrichment remains part of the wider provider/runtime parity work rather than
  a ping-specific duplicate implementation.
- Quota now matches Prodex's default five-second watch cadence, `--once`, raw,
  detail, profile selection, command-scoped base-URL override, aggregate
  `--auth`/`--provider` filtering, and the 0.435.0 provider-filter aliases.
  The all-profile Bubble Tea watch also matches the 0.435.0 interactive state:
  `j/k` or arrows scroll, `s` cycles current/remaining/profile/auth/account/plan
  sorts, `f` cycles all/openai/gemini/anthropic/copilot/kiro/deepseek/local/agy
  when not locked by an explicit provider, and `u` refreshes. Single-profile
  quota watch remains quit-only. The catalog includes standalone profiles and
  reports non-OpenAI profiles as unsupported until provider-specific quota
  adapters land. Manual `redeem PROFILE` now matches the usage preflight, one-hour
  reset confirmation guard, idempotent consume endpoint, base-URL override, and
  no-proxy controls. Runtime `--auto-redeem` policy and provider-specific quota
  adapters remain 1:1 gaps.
- Godex reloads Codex-owned auth on an authentication retry; it does not implement
  OAuth/token refresh, aggressive history rewrites, or silent model relaunch.
- Native tools, models, approval/sandbox behavior, foreground command servers,
  agents, queue execution, and rollout replay remain Codex-owned passthroughs.
  Godex resolves profile/session ownership and rejects routing escapes rather
  than duplicating those commands or introducing a daemon.

## Reference evidence

Tagged Prodex sources inspected include:

- `crates/prodex-runtime-launch/src/args.rs` for governed HTTP provider config and
  exec config scope, and `crates/prodex-cli/src/runtime_features.rs` for flags.
- `crates/prodex-cli/src/session_context.rs` and `profile.rs` for session/profile
  contracts, and profile login lifecycle/removal implementations in `prodex-app`.
- `crates/prodex-app/src/runtime_proxy/upstream.rs`, `selection/affinity.rs`,
  `responses/affinity_state.rs`, `standard/attempts/precommit.rs`, and
  `response_forwarding/streaming_writer.rs` for auth, ownership, and commitment.
- Profile import/manage/login source and
  `crates/prodex-app/tests/support/profile_commands_body/login/lifecycle.rs`
  for staged imports and rollback of uncommitted new homes.
- `crates/prodex-app/src/codex_binary.rs` for the `0.153.2` minimum, and
  `crates/prodex-cli/tests/src/lib.rs` for native queue/command-server passthrough.
- `crates/prodex-app/src/runtime_proxy/response_forwarding.rs`,
  `buffered_response.rs`, and
  `crates/prodex-runtime-proxy/src/response_forwarding.rs` for precommit SSE
  inspection, body failures, and incremental SSE ownership tracking.

Local Codex source at `a04940cb` supplied queue grammar and the native debug,
app-server proxy, and daemon branches that discard CLI overrides or detach.

Historical local parser validation used an ephemeral official npm package
`@openai/codex@0.159.3` without replacing the developer's global Codex installation.
It reported `codex-cli 0.159.3`; managed provider strict-config and runtime-feature
smoke tests passed, and unknown strict-config fields stopped exec/resume/fork/review
before model work. That remains useful regression evidence, but it is not presented
as a 0.160.0 execution result. The current Prodex 0.435.0 audit identifies Codex
0.160.0 as the compatibility target and reports no required model-transport change.
No live login, quota endpoint, refresh exchange, or model request is used for this
baseline migration.

## Verification and limits

The completion gate is `make verify`: formatting, source size, vet, shuffled
race tests, and build. Focused tests and both opt-in native parser smokes run
before it. Windows amd64 and macOS arm64 are cross-built locally; native Windows/macOS execution
and live OpenAI behavior are not established by that build. Prodex's Rust/Mojo
suite and release snapshots are outside this source/test audit.

Passed commands:

```sh
rtk go test ./internal/gateway/codex ./internal/delivery/cli/runtime
rtk go test ./internal/usecase/runtime ./internal/delivery/cli ./internal/delivery/cli/runtime ./internal/gateway/codex
rtk go test -race ./internal/repository/account ./internal/gateway/codex ./internal/usecase/auth
rtk go test -race ./internal/usecase/runtime ./internal/repository/account
rtk go test -race ./internal/delivery/cli/runtime ./internal/usecase/session ./internal/usecase/runtime
rtk go test -race ./internal/usecase/routing ./internal/delivery/http/proxy ./internal/gateway/openai
rtk go test -race ./internal/helper/sse ./internal/usecase/routing ./internal/repository/routing ./internal/delivery/http/proxy ./internal/gateway/openai
GODEX_TEST_CODEX_BIN="$(command -v codex)" rtk go test ./internal/gateway/codex ./internal/delivery/cli/runtime -run 'TestInstalledCodex.*Smoke' -v
rtk make verify
GOOS=windows GOARCH=amd64 go build -trimpath -o /tmp/godex-closure-windows-amd64.exe ./cmd/godex
GOOS=darwin GOARCH=arm64 go build -trimpath -o /tmp/godex-closure-darwin-arm64 ./cmd/godex
git diff --check
```

Stable bindings remain protected up to 8,192; opaque bindings expire after
30 days, and the in-memory cache remains at 4,096. Unknown/removed owners fail
closed. Affinity inspection remains bounded, including SSE metadata. The global
first-owner guard can serialize unrelated new conversations; use bounded lock
shards only if measured contention warrants it. No further small correctness
gap was found in the reviewed declared core after final closure. Startup SSE
inspection can wait for output and shares the global first-owner guard; there is
no background polling or new reader goroutine. SSE metadata remains bounded to
64 KiB per event, and compressed stream bytes are preserved without inspection.
These limits fail conservatively and never permit replay after commitment.

An initial closure test run failed because the new test composition passed a
typed nil repository through an interface. The fixture was corrected, and focused
race tests and the final verification gate passed afterward. No checks were
weakened. No live authentication, quota, refresh exchange, or model turn was used
to establish this result.
