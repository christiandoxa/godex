# Prodex 0.435.2 parity audit

Reference: exact Prodex tag `0.435.3`, commit
`31a1dbaedb300201a9e050ed3060fc5381dea62b`. The tag's `Cargo.toml` declares
`0.435.3`, and its compatibility audit names Codex `rust-v0.160.0` at commit
`a956835d020762cb2b570053af06f643a11c0ecc`. Comparison reads use tagged Prodex
Git objects rather than the mutable Prodex checkout. The audited Codex commit is
not present in this local checkout, so it is treated as release-audit evidence
rather than a locally re-executed source snapshot. Historical core-closure
evidence below that names `0.434.2`/`0.159.2` remains evidence for that earlier
checkpoint, not the current parity target.


## 1:1 parity expansion

The project target is now feature-for-feature parity with Prodex `0.435.3`, not
only the historical OpenAI/Codex core boundary. The core closure below remains a
verified baseline while additional surfaces are implemented. Current expansion
checkpoints add standalone/external profile registration and copy workflows,
`--profile` runtime selection, active standalone `CODEX_HOME` launch, and
persisted redacted runtime activity powering `info`, `status`, and `log`, plus
OpenAI profile bundle export/import compatible with Prodex plain v1, encrypted v2
Argon2id/AES-256-GCM-SIV, and legacy encrypted v1 PBKDF2-SHA256 envelopes.

A feature is counted as closed only when its observable behavior is implemented
and covered by local verification; a command-name stub does not count as parity.
Full parity remains open while any gap below remains.

## Historical core closure decision

The earlier core checkpoint closed practical parity for the declared
OpenAI/Codex account, isolated profile, session, and foreground managed HTTP/SSE
scope against its then-current baseline. It does not close feature-for-feature
parity with Prodex 0.435.3. The preserved audit read tagged Prodex source
alongside Godex production code, callers, tests, and Codex 0.159.2. The active
expansion baseline is Prodex 0.435.3 / Codex 0.160.0; the earlier audit found
and closed these gaps:

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

## Implemented practical parity

| Capability | Godex implementation and observable coverage |
| --- | --- |
| Isolated ChatGPT accounts | Official Codex interactive/device login, per-account homes, identity deduplication, deterministic and unambiguous selectors. Account/auth tests cover registration and selection. |
| Safe profile lifecycle | Repeat login/import replaces credentials while retaining native state. Metadata-only single-auth and multi-profile lifecycle journals recover interrupted operations, infer a fully persisted commit before cleanup, or roll partial actions back in reverse while restoring profile/account selection. Owned OS locks and shared profile leases exclude concurrent credential mutation/removal. |
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

- Prodex `0.435.3` is the exact parity baseline. Its audited Codex target remains
  `rust-v0.160.0`; the accepted minimum remains 0.153.2. The inherited 0.435.2
  auto-rotation reliability behavior is now covered on Godex HTTP production
  paths: 1% quota remains usable, authoritative all-zero pools do not dispatch,
  structured rate limits, overloads, and precommit transport failures recover
  across repeated sweeps, recovery sweeps are not a terminal attempt cap,
  cancellation remains terminal, and committed streams are never replayed.
  Individual recovery waits remain bounded while the overall retry epoch may
  continue as long as a retryable profile remains. Prodex 0.435.3 additionally
  removes user-visible local-capacity deadlines in favor of wait-and-resume
  backpressure with eligibility re-evaluation. Godex now applies that contract to
  global active-request admission: the default 64-request ceiling queues excess
  callers instead of rejecting them, cancellation stops queued work without
  upstream dispatch, and a production-path 32-caller/limit-1 regression verifies
  serialization with zero local saturation failures. Per-profile hard admission
  now follows the tagged default hard limit of 8 with Responses/WebSocket weight 2
  and Compact/Standard weight 1; permits live through response-body/duplex close,
  hard affinity bypasses the cap while remaining counted, capacity release wakes
  waiting work, and each wait epoch reloads profile eligibility so an unavailable
  saturated profile can yield to another profile. Lane/queue admission and the
  full per-message WebSocket recovery path remain active parity work; WebSocket
  must not be considered closed until those semantics are production-proven.
- Full native-home import remains unsupported. Godex imports auth only, leaves
  source settings, rollouts, and databases intact, and keeps managed homes isolated.
- The `0.435.1` hotfix adds no user-facing command surface; its material Godex
  delta is provider-model policy. Anthropic/Copilot embedded catalogs now come
  directly from the tagged canonical provider catalog, Anthropic defaults are
  `claude-sonnet-5-5` / 1,000,000 / 950,000, Copilot defaults are
  `gpt-6-astra` / 1,050,000 / 997,500, and both providers use the exact updated
  alias/fallback chains. Duplicate 0.434.3 external catalog tables were removed.
- The provider-error/precommit behavior introduced in Prodex `0.435.0` and
  inherited by `0.435.1` is covered locally: `rate_limit_error` maps to rate
  limiting, `not_found_error` maps to model-not-found, and `overloaded_error` /
  `server_is_overloaded` map to transient overload with the same cooldown class.
  Its observable retry/commit semantics remain the Godex routing target.
- All Godex equivalents of Prodex TUI surfaces must use Bubble Tea. Live status,
  quota watch, log stream/upstream, and redeem confirmation have been migrated;
  non-TTY fallbacks remain line-oriented.
- Gemini now has the 0.435.1 API-key runtime: tagged key/environment precedence,
  Gemini defaults and model fallback chains, Responses-to-Chat JSON/SSE
  translation, metadata and thought-signature preservation, function/namespace/
  MCP/custom/tool-search/web-search conversion, and semantic Compact with the
  bounded local fallback contract. For Responses, only structured Gemini
  quota/rate 429s advance the model chain; other 429 responses preserve their
  original status and body. Chat Completions uses the OpenAI-compatible
  endpoint; Messages and Embeddings pass through with Gemini API-key headers.
  GET Models list/single requests serve the exact Prodex 0.435.1 Gemini catalog
  locally, including tagged alias/case matching and model-not-found 404 behavior;
  non-GET Models requests still pass upstream. Gemini OAuth runtime/login remains
  disabled as in the tag. Legacy Gemini OAuth profile quota returns the exact
  disabled-auth guidance without credential reads or network access. OpenAI
  profiles with a non-OpenAI Codex `model_provider` expose the tagged configured
  provider/auth metadata and skip OpenAI quota probing. Gemini OAuth bundle
  migration is implemented independently of the disabled runtime: plain/encrypted
  bundles preserve empty `auth_json`, tagged provider email/project metadata, and
  one validated `gemini_oauth.json`; same-name updates replace metadata and the
  private secret without creating `auth.json`. Native Antigravity launch and
  global login now use
  `PRODEX_AGY_BIN` (default `agy`) with the shared Codex home and tagged child
  arguments/environment. Native launch skips profile startup and update lookup,
  prepares the shared home during dry-run without spawning `agy`, rejects resume
  and unsupported provider options, and preserves child exit status. Child
  launches hold the shared Codex session lock through process exit. Godex accepts
  the tagged `s`/`super` Gemini syntax plus its existing `run` spelling. The
  dry-run TTY panel uses Bubble Tea with the tagged panel fields.
- DeepSeek now has the 0.435.1 raw-key runtime
  plus its dedicated Codex model catalog and advanced request-side Responses
  adapter: exact key precedence/provider defaults/stable key rotation, launch-model
  catalog precedence, reasoning effort, primitive sampling/token controls,
  stop/logprobs/user normalization, JSON mode, message/tool replay, RTK tool
  arguments, strict-schema normalization with config.toml-over-env precedence,
  named tool choice, `pro/flash` model fallback, Chat/Messages passthrough,
  native DeepSeek Messages URL/auth, local Models emulation, and local Compact
  fallback are implemented. Buffered Responses and live SSE match the tagged sparse
  defaults, reasoning/tool shaping, raw function-argument deltas, empty-delta
  events, and completion semantics. Search-option mapping, off-mode rejection,
  config-over-environment `auto`/`openai_chat`/`anthropic` selection, strict-tools
  beta-base routing, and native DeepSeek Anthropic Messages request/response/SSE
  translation are implemented. Native streams inspect the first event before
  commitment, preserving the tagged bounded model-fallback and credential-rotation
  rules without replay after commitment. The Prodex local OpenAI-compatible
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
  profile selection remains hard-affinity. Responses use the shared 0.435.1
  Responses-to-Chat compatibility contract with exact Anthropic alias/fallback
  ordering before credential rotation; auth failures and bare 429s do not advance
  models. Buffered JSON and live SSE are translated back to Responses, Chat and
  Messages stay passthrough, Models list/single are locally emulated from the
  verified 0.435.1 IDs/aliases/context/endpoint metadata, and Responses Compact
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
  model catalogs built from the exact 0.435.1 static provider data plus account
  `/models` metadata, and managed multi-profile credential rotation. Default or
  active-profile launches prefer the selected profile first, filter unusable
  credential profiles, rotate only before commitment, and retain durable
  continuation affinity through the existing routing layer; explicit profile
  selection remains single-profile hard affinity. Copilot model fallback now also
  matches the 0.435.1 pre-commit policy for the native Responses path: the exact
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
  Kiro now also has the 0.435.1 foreground ACP runtime bridge: managed profiles
  run against isolated private Kiro runtime databases, selected-first profile
  pools reuse the generic precommit router, and explicit profile selection remains
  hard-affinity. Responses, Compact, Chat, Messages, and Models routes match the
  provider registry; the exact tagged canonical catalog is merged with bounded
  per-profile model metadata. Compact uses semantic ACP compaction with the same
  local fallback contract. Responses/Chat stream live ACP session/update deltas
  with bounded 16-chunk backpressure, reader-close cancellation, redacted
  128-event tool activity, and the 300-second/env-overridable stream-idle policy;
  Messages intentionally buffers the completed turn before emitting Messages
  SSE. Bounded conversation replay covers previous_response_id and tool-output
  call-id recovery. Kiro auth/runtime DB changes remain profile-local and are
  restored into managed snapshots only under the reference freshness rule.
- Super mode and its hidden expose/broker/MCP bridge/sub-agent execution stack,
  including optional Presidio integration.
- The standalone gateway surface, remaining process/resource metrics,
  audit-log backend, and richer runtime-policy diagnostics. Doctor now supports
  install checks, bounded runtime tails, quota summaries, runtime JSON, and
  redacted private bundles; `--repair-import-auth-journals` now recovers
  profile-store auth-replacement journals and reports the tagged orphan/repaired
  status shape. `--repair-session-index` now resolves the active/default Codex
  home, runs full shared-session maintenance, then reconciles active and archived
  threads through Codex app-server before reporting completion. Policy suggestions
  remain. Existing
  status/quota/log, doctor panels, redeem-confirmation, human session-list, profile
  bundle password, and
  login/provider-menu TUIs use Bubble Tea. The login menu now mirrors the 0.435.1
  nine-entry ordering/navigation and preserves the reference TTY-only trigger.
  Persisted OpenAI/API-compatible API-key login is implemented end-to-end:
  `--with-api-key`, `--base-url`/`--openai-base-url`, masked Bubble Tea input,
  Prodex-compatible `api_key[_host]` profile naming, private `auth.json`,
  `.prodex-profile.toml` endpoint persistence, repeat-login update/preserve/clear
  semantics, and direct `prodex-openai-compatible` Codex provider injection with
  user `model_provider` precedence. Antigravity runtime and login now execute
  through the native CLI; Gemini API-key entries remain guidance-only.
- Explicit self-update, best-effort cached update notices on eligible commands,
  manual reset-credit redemption, and cost-bearing `ping openai` diagnostics are
  implemented.
- Built-in Claude import is implemented with `CLAUDE_CONFIG_DIR`/`~/.claude`
  source resolution, bounded regular-file checks, Anthropic identity deduplication,
  Prodex-compatible unique naming, private managed `.credentials.json`, and
  create/update/activate semantics. Anthropic bundle export/import now also
  matches the 0.435.1 wire contract: empty `auth_json`, provider metadata, and a
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
  tokenless profile persistence. Multi-profile bundle imports now use a private,
  credential-free lifecycle journal with crash recovery across create/update
  actions, account-backed auth replacements, active-selection restoration, orphan
  staging cleanup, and committed-state inference when the final phase marker was
  not persisted. OpenAI plain/encrypted bundle wire formats, Bubble Tea
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
- Doctor expansion now covers the observable 0.435.1 diagnostics that have real
  Godex data sources: `--install`, `--runtime`, `--quota`, 128 KiB default bounded
  `--tail-bytes`, `--runtime --json`, and `--bundle [PATH] --redacted`.
  `--repair-import-auth-journals` recovers profile-store auth replacements, counts
  remaining orphan journals without mutating them, and exposes the tagged human,
  runtime-JSON, and bundle status fields. Account-store imports remain outside this
  journal path. External provider quota diagnostics now use the tagged `Quota`,
  `Main`, and optional `Reset` human fields plus the nested `{profile, provider,
  quota}` JSON shape. OpenAI diagnostics now derive ready/blocked state from tagged
  admission + 5h/weekly window semantics, preserve missing-vs-empty rate-limit
  shape, and emit the same nested success/error JSON without leaking raw errors.
  Bundle runtime events omit account IDs, quota diagnostics omit identity/email and raw
  gateway errors, and file output is private/atomic. Full session-index repair now
  matches the tagged maintenance-before-reconciliation ordering, including stable
  attachment paths, metadata-prefix repair, modified-time restoration, goal-DB path
  persistence, versioned maintenance cache, app-server active/archived pagination,
  and the optional runtime timing marker. Runtime-policy suggestions remain
  unsupported.
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
  redacted failure detail. Large-context OpenAI models now use the tagged launch
  precedence before spawn: explicit root config, configured/effective
  `models_cache.json`, then the exact 0.435.1 OpenAI catalog, with max-context
  preference for the tagged model families and the 90% auto-compact default only
  when not explicitly configured. It is never invoked implicitly.
- Quota now matches Prodex's default five-second watch cadence, `--once`, raw,
  detail, profile selection, command-scoped base-URL override, aggregate
  `--auth`/`--provider` filtering, and the 0.435.1 provider-filter aliases.
  The CLI rewrite also matches 0.435.1: invocations without `--profile`/`--raw`
  default to the detailed aggregate pool. Virtual provider quota is implemented
  for DeepSeek (plural/single key precedence + `/user/balance`), local
  OpenAI-compatible servers (bounded models reachability), and Anti-Gravity
  (bounded detailed all-account CLI probe); these virtual reports are collected
  only when their provider filter is selected, not for `all`. External account,
  plan, status, main/reset summary, readiness, and sort keys are observable in
  the Godex quota view. The all-profile Bubble Tea watch now also matches the
  0.435.1 `Quota Overview` aggregate: available-profile count, last-update time,
  ready and total OpenAI 5h/weekly remaining pools with earliest resets, and the
  generic main remaining pool used by Copilot-style snapshots when OpenAI window
  data is absent. Its interactive state matches 0.435.1: `j/k` or arrows scroll,
  `s` cycles
  current/remaining/profile/auth/account/plan sorts, `f` cycles
  all/openai/gemini/anthropic/copilot/kiro/deepseek/local/agy when not locked by
  an explicit provider, and `u` refreshes. Single-profile quota watch remains
  quit-only. Imported Kiro profiles now match the 0.435.1 external snapshot
  contract from managed `kiro_auth.json` plus optional model catalog: account
  fallback, auth plan, profile/region details, imported model count, readiness,
  and missing-catalog fallback are implemented without network work. Managed
  Anthropic profiles also match the external quota contract: existing OAuth
  refresh, account/auth-method/expiry details, optional
  `ANTHROPIC_ADMIN_KEY`/`ANTHROPIC_ADMIN_API_KEY` organization rate-limit
  summaries, and safe OAuth-only degradation on admin API failure are
  implemented. Managed AGY profiles also match the preferred-account external
  quota contract: profile account metadata suppresses `--all-accounts`, matching
  rows are selected from object/array output, and missing preferred rows fall back
  to the first account. Managed Copilot profiles now match the 0.435.1 user-quota
  policy: exact host/login token resolution, plan/access precedence,
  chat/completions remaining and monthly totals, blocked/readiness semantics,
  monthly reset summary, and minimum remaining percentage are implemented without
  persisting the token. Profile-backed raw quota now follows the tagged provider
  dispatch: Copilot returns the bounded original user-info JSON, Anthropic/Kiro/AGY
  serialize their external snapshot, and legacy Gemini OAuth fails before network
  access with the 0.435.1 migration guidance. OpenAI profiles whose `config.toml`
  selects a non-OpenAI `model_provider` report `model-provider:<id>` as auth, are
  non-quota-compatible, and expose the tagged configured-provider snapshot/raw JSON.
  Manual `redeem PROFILE` now matches the usage
  preflight, one-hour
  reset confirmation guard, idempotent consume endpoint, base-URL override, and
  no-proxy controls. Runtime `--auto-redeem` now matches the managed OpenAI
  foreground HTTP/SSE policy: normal ready/fallback selection precedes redemption;
  quota-blocked profiles get one same-profile redeem/retry before rotation; a
  whole-pool redeem requires complete OpenAI quota evidence with no weekly-usable
  profile; the 0.435.1 plan/reset/order planner, Spark exclusion, five-minute
  natural-reset guard, UUIDv7 idempotency key, post-redeem quota refresh, and
  hard-affinity owner preservation are implemented. Failed/missing quota probes,
  non-quota failures, and non-OpenAI providers never spend a credit. WebSocket
  auto-redeem remains absent with the wider WebSocket/Realtime transport.
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
- `crates/prodex-provider-core/src/translators/anthropic/messages.rs`,
  `crates/prodex-provider-core/src/translators/anthropic/messages/stream.rs`, and
  `crates/prodex-app/src/runtime_launch/proxy_startup/local_rewrite_deepseek_send.rs`
  for DeepSeek native Anthropic Messages request, response, SSE, and precommit
  fallback behavior.

Local Codex source at `a04940cb` supplied queue grammar and the native debug,
app-server proxy, and daemon branches that discard CLI overrides or detach.

Historical local parser validation used an ephemeral official npm package
`@openai/codex@0.159.3` without replacing the developer's global Codex installation.
It reported `codex-cli 0.159.3`; managed provider strict-config and runtime-feature
smoke tests passed, and unknown strict-config fields stopped exec/resume/fork/review
before model work. That remains useful regression evidence, but it is not presented
as a 0.160.0 execution result. The current Prodex 0.435.1 audit identifies Codex
0.160.0 as the compatibility target and reports no required Codex model-transport
change; the release itself is a provider-catalog/default/fallback hotfix.
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
