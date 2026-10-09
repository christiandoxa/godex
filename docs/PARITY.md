# Historical Prodex 0.435.5 parity audit

The current parity checkpoint targets Prodex `0.436.1`; see
[`PARITY-04361.md`](PARITY-04361.md). This document preserves the earlier
0.435.5 audit and its historical evidence.

Reference: exact Prodex tag `0.435.5`, commit
`24223c315e7a30f527328f62c632a481411552cb`. The tag's `Cargo.toml` declares
`0.435.5`, and its compatibility audit retains Codex `rust-v0.160.0` at commit
`a956835d020762cb2b570053af06f643a11c0ecc`. Comparison reads use tagged Prodex
Git objects rather than the mutable Prodex checkout. The audited Codex commit is
not present in this local checkout, so it is treated as release-audit evidence
rather than a locally re-executed source snapshot. Historical core-closure
evidence below that names `0.434.2`/`0.159.2` remains evidence for that earlier
checkpoint, not the current parity target.


## 0.435.4 runtime reliability delta

Prodex `0.435.4` adds no user-facing command surface. Its runtime contract makes
compatible retryable profile viability authoritative over transient-failure
flags and precommit attempt bookkeeping. Fresh Responses, Standard HTTP, Compact,
and fresh WebSocket work must wait and reselect while a compatible retryable
profile remains. Selection must be reevaluated after every wait, including
request-local exclusions and current profile state. Hard continuation affinity
still fails closed, and an authoritative all-zero quota pool remains terminal.

At that historical checkpoint, this contract was not yet closed in Godex. In particular,
Godex's current fresh route still stops at its 64-attempt boundary, and its
recovery loop retains the initial candidate snapshot after a wait. Compact's
request-local quota fallback decision and WebSocket's fresh-message recovery
also require independent parity checks.

## 1:1 parity expansion

The historical project target was feature-for-feature parity with Prodex `0.435.5`, not
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
parity with Prodex 0.435.5. The preserved audit read tagged Prodex source
alongside Godex production code, callers, tests, and Codex 0.159.2. The active
expansion baseline is Prodex 0.435.5 / Codex 0.160.0; the earlier audit found
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
| Safe profile lifecycle | Repeat login/import replaces credentials while retaining native state. Metadata-only journals recover interrupted operations; owned OS locks and shared profile leases exclude concurrent credential mutation/removal. Account repository tests cover recovery, leases, and unsafe paths. |
| Account retention and native auth | Enable/disable retains the home. Managed status/logout bypass quota/rotation; logout uses an exclusive lease. Unsafe mutating auth passthrough is rejected with managed-command guidance. |
| Session discovery and launch | Bounded metadata catalog, list/current filters, text/JSON/ID/resume-command output, unique prefixes, and bare UUID resume. Native name/`--last` lookup follows Codex 0.160.0 source/preview semantics across managed profiles. Bare native resume/fork keeps Codex's own picker UI while the tagged Prodex runtime-state manifest is projected onto one shared Codex root; `auth.json` and `.credentials.json` remain profile-local while config/history/session/index/tooling state is shared. Delivery/session/runtime plus exact Codex app-server tests cover the global view and argument preservation. |
| Quota and fresh selection | One-shot compact/detailed usage windows, single-profile raw JSON, reset timestamps, fail-open probe uncertainty, deterministic bounded selection, and temporary exhaustion deadlines. If current candidates fail before commitment, routing refreshes launch-excluded profiles using the bounded request model and route; results stay fresh for five minutes per account/model/route. Responses, Compact, and WebSocket use the primary window, Standard uses both windows, and Luna reserve quota stays model-specific. Explicit selectors remain fixed. Launch preflight still uses broad primary/secondary windows. |
| Managed Codex configuration | HTTP/SSE Responses provider keeps native account/bootstrap HTTPS. Managed config enters the innermost exec scope; user overrides retain precedence. Routing/auth-store overrides, quoted/equals forms, whole provider tables, OSS/local providers, and remote app-server routing cannot bypass it. The Codex delimiter preserves literal arguments. |
| Durable conversation ownership | Hashed, bounded, versioned owner bindings survive restart/cache expiry. Opaque Responses WebSocket turn state survives router restart in a 30-minute, 2,048-file sidecar under its owning private `CODEX_HOME`; insecure homes use only memory. Requested owners beyond cache capacity resolve correctly. Independent routers serialize first-owner selection under an OS guard. Native picker/name/last resumes keep the rollout home and enabled owner pool; explicit account scope remains fixed. |
| Safe HTTP rotation and streaming | Selected bearer and ChatGPT routing ID replace caller credentials. Bounded retries occur only before commitment. Account-level response retry deadlines survive router restart and clear after success. Route-scoped transport cooldowns persist and rank candidates by remaining delay. Known continuations preserve their owner; unknown opaque continuations fail closed. Responses HTTP retries an exact invalid previous-response ID once on its bound owner only with session metadata and reconstructable full history; otherwise it preserves the original response. Streams flush and preserve bytes/headers/trailers; committed failures abort downstream without replay. HTTP/routing tests cover chains, recovery guards, concurrency, restart, and real broken streams. |
| Native process and installation surfaces | Native Codex owns models, tools, sandbox/approval behavior, session replay, queue execution, agents, mcp-server, app-server, exec-server, and token refresh. Godex preserves safe foreground arguments and meaningful child exit status; routing escapes fail before launch. Existing checksum installers and release naming remain intact; no installation/release files changed in this audit. |
| Current-home account import | New imports copy the native Codex home into an isolated managed account. Duplicate identities update authentication and preserve the existing managed home. `--insecure` bypasses the source-home permission check. Gateway and use-case tests cover full-state copy, private staging, identity updates, and cleanup. |

## Gaps closed by these checkpoints

- `09d5cec` and `9fa089b` close the remaining native session-name/`--last`/picker gap against Codex `rust-v0.160.0`. Name lookup is case-sensitive, active-session only, uses the native `cli`/`vscode` source set, and falls back from thread name to the first user-message preview. The native picker itself is not reimplemented: Godex prepares one shared Codex state root before launch and leaves the exact native picker in control. `CODEX_SQLITE_HOME` points at that root only after sharing is established. An opt-in integration regression runs the exact local Codex 0.160.0 app-server and proves `thread/list` launched from one profile sees picker-visible rollouts imported from two managed profiles.
- `44673d8` aligns that shared runtime state with the exact Prodex `0.435.5` manifest rather than sharing only picker-critical files. Sessions, archives, attachments, shell snapshots, memories, rules, skills, agents, plugins, tagged static files, dynamic SQLite families, and valid profile-v2 configs are linked through the shared root; `config.toml` is accepted only when its link targets the configured shared root, while `auth.json` and runtime-local `.credentials.json` remain profile-local. `history.jsonl` uses the tagged 64 MiB bounded first-occurrence dedup/timestamp merge. Non-history file authority follows profile creation order, and migration from a legacy directory symlink copies its target without deleting that legacy target. Independent regressions plus sensitivity mutations lock history merging, legacy-target preservation, creation-order authority, and shared-config target binding.
- Retryable response cooldowns previously lived only in router memory. Godex
  now persists account-level deadlines in a bounded, versioned
  `retry-backoff.json` sidecar, restores active entries at startup, and clears
  them after a successful response. Repository locks and atomic writes protect
  updates; tests cover restart, expiry, concurrent writes, and success clearing.
  Tagged reference: [health backoff](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/health_backoff.rs)
  and [health commit](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/health_commit.rs).
- Recognized transport failures now persist per account and route in a bounded,
  versioned `transport-backoff.json` sidecar. Cooldowns start at 15 seconds,
  double to 120 seconds, soften to 15 seconds after restart, and clear after
  same-route success. Fresh recovery waits for the cooldown and candidate
  ranking defers the backed-off account while retaining it as a fallback.
  Tests cover transport classification, restart, route isolation, growth, and
  expiry. Tagged reference: [transport failure classification](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/transport_failure.rs),
  [backoff policy](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/health_backoff.rs),
  and [runtime constants](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/core_constants.rs).
- `profile import-current` previously copied only `auth.json`. It now follows
  Prodex `0.435.2`'s `copy-current` flow: new accounts receive a private native
  home copy, while a duplicate identity refreshes authentication and keeps its
  managed state. `--insecure` bypasses the source directory permission check;
  symlinks remain rejected. Tagged source: [CLI arguments](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-cli/src/profile.rs#L77-L87), [import-current dispatch](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/profile_commands/import_export/import.rs#L97-L105), [copy path](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/profile_commands/manage/add_profile.rs#L64-L73), and [duplicate auth update](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/profile_commands/manage.rs#L132-L230).
- Strict capability checking previously used a command that rejected
  `--strict-config`. It now uses `exec-server --listen stdio`, closed stdin,
  a temporary home and working directory, and a ten-second deadline. Relative
  executable paths resolve before changing the probe directory.
- Exec config placement and routing/credential override detection now cover
  nested resume/fork/review, quoted keys, equals forms, and literal delimiters.
- Root option values and generated config previously hid native commands from
  dispatch. Session intent now crosses delivery into the use case through
  `model/session.Launch`, without reparsing CLI placement in the use case.
- Responses HTTP now recovers an exact invalid previous-response ID once on its
  bound owner when session metadata and reconstructable full history are present.
  It removes that stale response binding and its profile turn-state sidecar;
  incomplete follow-ups keep the original response. Tests cover JSON and SSE
  failures, ownership, near-match errors, and the one-retry limit. Tagged source:
  [request shape](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/mojo/prodex_core/runtime_proxy_request.mojo),
  [HTTP recovery](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/responses/attempt.rs),
  and [recovery tests](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/tests/support/main_internal/runtime_proxy_continuations/http_followups/invalid_previous_response_id.rs).
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
The earlier core closure added only `helper/sse`, a technical framing parser
with real consumers in routing and HTTP delivery. Quota classification stays in
the routing use case; HTTP delivery owns stream commitment and forwarding.
Import decoding stays private to the Codex gateway, journal recovery stays in
account persistence, and eligibility validation stays in the runtime use case.

## Remaining 1:1 parity gaps

- Prodex `0.435.5` is the exact parity baseline. Its audited Codex target remains
  `rust-v0.160.0`; the accepted minimum remains 0.153.2. Prodex `0.435.5` adds no
  new user-facing surface; its WebSocket hard-affinity quota-replay delta is closed
  by the checkpoint below. The inherited reliability work continues to prove that
  quota-aware rotation does not leak local admission failures. Godex now
  proves through production paths that 1% remaining quota stays selectable,
  all-zero quota stops before launch/dispatch, structured rate-limit 429 and
  its cooldown recovers, overload 503 and precommit transport failures recover
  across repeated sweeps, bare 429 passes through, cancellation stops recovery,
  known affinity stays with its owner, and committed streams are not replayed.
  Responses and standard HTTP tests capture attempt order; Compact captures the
  full retry sequence; a fake-clock Responses test crosses more than 30 seconds
  of simulated retry backoff. Godex continues recovery while a transiently
  failing profile remains quota-usable, matching Prodex `0.435.4`; each wait is
  capped at 30 seconds. Pool exhaustion and request cancellation remain terminal.
  Godex has no request-time quota-probe worker, so the same-request cold-start
  probe race does not apply. It ranks weighted in-flight work using Prodex's
  default soft limit of four units: Responses and WebSocket requests count
  twice, while Compact and Standard count once. Above-limit accounts rank
  behind lower-load candidates. Godex still lacks Prodex's local-overload
  pressure mode and capacity-admission wait. Responses WebSocket now routes
  each client text message through the routing use case, binds response IDs to the selected
  account, retries eligible precommit failures, and streams committed frames
  through their terminal event. The OpenAI gateway holds at most 64 KiB or 64
  precommit events; Realtime/live paths retain the raw tunnel. Responses
  messages reuse terminal upstream sessions per client tunnel and selected
  account, with a 128-session bound and 60-second idle reconnect. Godex retries
  an explicit connection-limit event on a reused session once with a fresh
  connection on the same account. WebSocket precommit quota failures use the
  configured auto-redeem retry path. For a known-owner
  `previous_response_not_found`, Godex retries the owner up to three times
  with returned turn state at 75 ms, 200 ms, and 500 ms, then preserves the
  upstream retryable event and retains that request's affinity. Prodex applies a stateful policy: it may retry the owner
  and, when it classifies the WebSocket continuation as stale, fails closed
  without releasing locked affinity; rotation releases affinity separately.
  Godex now retains that owner for a later full-context replay. HTTP Responses
  also persist route-scoped `previous_response_not_found` scores using a hashed
  response ID, with Prodex's threshold of two failures, score cap of 16,
  one-point decay per 180 seconds, and 14-day retention. A bound HTTP
  continuation stays with its owner until the second failure reaches the
  threshold; Godex then releases response, turn, and session affinity. Fresh
  candidates exclude that account for the same response and route while its
  score remains active. The bounded snapshot and in-memory cache retain the
  4,096 newest records. This covers the HTTP
  negative-cache threshold and release path; WebSocket retries remain pinned to
  their owner, and same-request cross-owner retry remains open. Prodex's
  [negative-cache recording](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/continuation.rs#L467-L568)
  and [candidate exclusion](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/selection/previous_response/discovery.rs#L119-L140)
  plus the [score and decay limits](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/core_constants.rs#L108-L151)
  and [score retention](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-runtime-store/src/lib.rs#L36)
  establish that policy.
  Its [orchestration policy](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/previous_response_orchestration.rs)
  and [Codex full-context recovery test](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/tests/support/main_internal/runtime_proxy_continuations/websocket_invalid_previous_response.rs)
  establish the client replay workflow.
  Hard-affinity continuations bypass launch-time `EligibleAfter` blocking and
  auto-redeem, so Godex sends the message to its bound owner, matching Prodex's
  [WebSocket gate](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/websocket/response_tracking/quota_gate.rs#L24-L98)
  and [shared quota decision](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/quota/gate.rs#L188-L311).
  Prodex `0.435.5` adds a recovery exception to that hard-affinity rule: a
  quota-blocked previous+session owner with a legal future full-context fallback
  returns 400 `previous_response_not_found`, releases only that exhausted owner's
  previous/turn/session affinity, and lets the client's full-context replay bind
  another profile. Fallback availability for this signal is evaluated without the
  stale continuation constraint, so one bounded `LastChance` profile may bypass
  soft quarantine/circuit pressure; the hard in-flight cap and a truly unavailable
  pool remain terminal. Pre-send blocked-owner regressions and the public WebSocket
  production path lock this `0.435.5` behavior.
  Fresh work refreshes launch-excluded profiles after current candidates fail
  before commitment; the bounded request model and route select availability,
  and the five-minute cache follows the reference's quota-cache freshness
  interval. Luna reserve capacity applies only to Luna; retired Spark remains
  unavailable. This covers stale-exclusion refresh behavior from Prodex's
  [model-specific quota pairing](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-quota/src/render/model_capacity.rs#L26-L106)
  and [route quota gate](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/quota/gate.rs#L188-L311).
  Godex now orders eligible accounts by weighted in-flight load and rotates
  equally loaded accounts deterministically. Its default soft limit is four
  units; Responses/WebSocket use two units per request, and Compact/Standard
  use one, matching Prodex's [runtime defaults](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/core_constants.rs#L75-L80)
  and [route weights](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-runtime-proxy/src/health/inflight.rs#L12-L39).
  Prodex's candidate plan includes in-flight count plus route health, backoff,
  quota pressure, and a soft limit
  ([candidate inputs](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-runtime-proxy/src/selection_plan.rs#L44-L64),
  [runtime plan construction](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/selection_plan.rs#L236-L313));
  Launch preflight and routing share five-minute usage snapshots. Warm snapshots
  provide model- and route-aware quota availability and pressure without another
  usage fetch; routing skips a cached exhausted account when a ready alternative
  remains, and fails open when every cached candidate is blocked. A cold cache
  does not trigger a pre-send probe for otherwise eligible accounts. Candidate
  order now uses transient backoff time, cached quota pressure, weighted
  in-flight count,
  a bounded route-specific health penalty, and deterministic rotation among
  equal-ranked candidates.
  Accounts above the in-flight soft limit are deferred while lower-load
  candidates remain, but stay available as fallbacks. Godex persists route-health
  penalties for fresh and bound-owner responses in `route-health.json`; scores
  decay by one point per minute and are ignored after 14 days. Active transient
  backoff defers a candidate while preserving it as a fallback. Account-level
  response and route-scoped transport backoffs now persist. Route circuits now
  persist in `route-circuits.json` per account and route, open at health score 4,
  and grow from 20 seconds to a 10-minute cap under repeated failures. Restart
  softens active circuits to a health-scaled half-open probe. The repository
  reserves one probe atomically, and success clears the circuit. Prodex's
  broader health and ranking inputs remain
  open ([candidate inputs](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-runtime-proxy/src/selection_plan.rs#L44-L64),
  [health updates](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/health_performance.rs),
  [route-circuit policy](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_proxy/health_circuit.rs),
  [health decay and retention](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-runtime-store/src/profile_backoff/score.rs)).
  Known-owner WebSocket reused-session transport failures reconnect once on the
  same account when session turn state is available. A fresh handshake that
  then reports `previous_response_not_found` retries up to three times on that
  owner with returned turn state. Response-ID-to-turn-state lookups survive router
  restarts for 30 minutes in a bounded sidecar under the owning private
  `CODEX_HOME`; filenames contain response-ID digests, and `routing.json` still
  contains no raw response IDs or turn-state values. Same-request cross-owner
  retry remains open.
  Inbound binary messages receive the tagged error event and client pings
  receive local pong frames. Full 0.435.5 parity remains open. The 0.435.1
  provider-catalog and Codex-owned provider/history invariants remain historical
  evidence and still apply.
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
  GET Models list/single requests serve the Prodex 0.435.1 Gemini catalog locally;
  missing models return the tagged 404 response and non-GET requests pass upstream.
  Login remains guidance-only for API keys, and OAuth profiles stay disabled as
  in the tag. Legacy Gemini profile quota returns the tagged disabled-auth error
  without a network request. Profile-backed custom model providers now report
  the tagged configured-provider metadata from bounded Codex config inspection.
  Gemini OAuth bundles now preserve Prodex `0.435.1`'s required Gemini provider
  `email`, optional `project_id`, exact `secret_files[].path` value
  `gemini_oauth.json`, and tagged `GeminiOAuthSecret` field types. New imports write only that credential file
  under the managed profile home; same-provider imports update it in place.
  Unencrypted bundle payloads contain the credential, password-protected
  payloads encrypt it, and profile metadata plus CLI summaries omit it. OAuth
  runtime use remains disabled as in the tag.
  Native Antigravity launch and global login use `PRODEX_AGY_BIN` (default `agy`)
  with the shared Codex home and the tagged child arguments/environment. Native
  launch skips profile startup and update lookup, prepares the shared home during
  dry-run without spawning `agy`, rejects resume and unsupported provider options,
  and preserves the child exit status. Child launches hold the shared Codex
  session lock through process exit. Godex accepts the tagged `s`/`super` Gemini
  syntax plus its existing `run` spelling. Its dry-run TTY panel uses Bubble Tea
  with the tagged panel fields and inline layout.
- DeepSeek now has the 0.435.1 raw-key runtime
  plus its dedicated Codex model catalog and advanced request-side Responses
  adapter: exact key precedence/provider defaults/stable key rotation, launch-model
  catalog precedence, reasoning effort, primitive sampling/token controls,
  stop/logprobs/user normalization, JSON mode, message/tool replay, RTK tool
  arguments, strict-schema normalization with config.toml-over-env precedence,
  named tool choice, `pro/flash` model fallback, Chat/Messages passthrough,
  native DeepSeek Messages URL/auth, local Models emulation, and local Compact
  fallback are implemented. DeepSeek Responses now match the tagged sparse
  response defaults, while SSE matches the tagged raw function-argument,
  empty-delta, and `[DONE]` event shaping. Search-option mapping, off-mode
  rejection, config-over-environment mode selection, and strict-tools beta-base
  routing are implemented. The native Anthropic Messages bridge handles
  supported request shapes and has bounded first-event inspection. Request
  selection now matches the tagged mode policy: default/`auto` and `anthropic`
  use native Messages only when web-search options are present, and only
  default/`auto` may safely fall back to chat. An unset mode uses the `auto`
  fallback policy. Tests cover default fallback and explicit `auto`,
  `openai_chat`, and `anthropic` behavior ([selection and fallback](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/crates/prodex-app/src/runtime_launch/proxy_startup/local_rewrite_deepseek_send.rs#L460-L490), [mode policy](https://github.com/christiandoxa/prodex/blob/8000065c66381d876ca4b369ddcf01bf3d4506f0/mojo/prodex_core/deepseek.mojo#L2963-L3020)). Its SSE path drops empty text deltas and preserves whitespace, matching the tagged runtime; other response/SSE behavior remains a parity gap. The Prodex local OpenAI-compatible
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
  redacted private bundles; full import-lifecycle journal recovery, session-index
  repair, and policy suggestions remain. Existing status/quota/log, doctor panels,
  redeem-confirmation, human session-list, profile bundle password, and
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
  tokenless profile persistence. Process-crash lifecycle-journal recovery for
  multi-profile imports also remains
  to match Prodex exactly. OpenAI plain/encrypted bundle wire formats, Bubble Tea
  protection/password prompts, and identity-safe runtime rollback are implemented.
- OpenAI WebSocket upgrades use a local Responses session and retain a raw
  Realtime/live tunnel. Responses messages reuse terminal upstream sessions per
  client tunnel and account, with a 128-session bound and 60-second idle reconnect.
  A reused session's explicit connection-limit event gets one fresh connection
  attempt on the same account. The gateway buffers tagged precommit events within a 64 KiB
  or 64-event bound, exposes retryable failures to routing, and streams committed
  frames through the terminal response event. Response IDs bind nested
  `previous_response_id` continuations to the selected account. Exchanges close
  before the next client message. Handshakes hide upstream credentials and
  cookies; binary input receives the tagged 400 error event, and client pings
  receive local pong frames. WebSocket precommit quota failures use the
  configured auto-redeem retry path. Known-owner `previous_response_not_found`
  failures retry up to three times with returned turn state when available, then preserve
  the upstream retryable event and retain the failed request's affinity. A later
  full-context replay stays on that owner, matching Prodex's stale-continuation
  policy.
  Hard-affinity continuations proceed on their bound owner despite launch-time
  quota deadlines and do not auto-redeem. Fresh and soft-affinity routes refresh
  launch-excluded OpenAI profiles after current candidates fail before
  commitment. Warm five-minute usage snapshots are shared by launch preflight
  and routing; cached availability and pressure are model- and route-aware.
  Fresh cached quota failures are skipped when another eligible account remains,
  while a fully cached-blocked pool fails open. Cold caches do not cause
  pre-send quota probes for otherwise eligible accounts. Candidate ranking uses
  transient backoff time, quota pressure, weighted in-flight count, route-health
  penalties, an in-flight soft limit with fallback retention, and deterministic
  rotation for equal ranks.
  Route health for fresh and bound-owner responses persists in versioned
  `route-health.json`, decays by one point per minute, and is ignored after 14
  days. Account-level response and route-scoped transport backoffs persist.
  Prodex's broader health scoring, other ranking inputs, and same-request
  cross-owner retry remain open. The turn-state
  sidecar is bounded to 2,048 files per profile with a 30-minute expiry; values
  stay in the owning private `CODEX_HOME`, while
  the global routing snapshot stores only digests and account IDs.
  Unsupported upgrades and paths fail before upstream work.
- Remove keeps its existing destructive contract. Disable and logout provide
  retained deactivation; no second archive tree or changed removal default.
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
  `--repair-import-auth-journals` recovers profile-store auth replacements using
  path/phase metadata and a private backup kept in that profile's `CODEX_HOME`;
  account-store import updates remain outside this journal path. Bundle runtime
  events omit account IDs, quota diagnostics omit identity/email and raw gateway
  errors, and file output is private/atomic. Full session-index repair and
  runtime-policy suggestions remain unsupported and fail explicitly.
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
  persisting the token. Manual `redeem PROFILE` now matches the usage
  preflight, one-hour
  reset confirmation guard, idempotent consume endpoint, base-URL override, and
  no-proxy controls. Runtime `--auto-redeem` now matches the managed OpenAI
  foreground HTTP/SSE policy: normal ready/fallback selection precedes redemption;
  quota-blocked profiles get one same-profile redeem/retry before rotation; a
  whole-pool redeem requires complete OpenAI quota evidence with no weekly-usable
  profile; the 0.435.1 plan/reset/order planner, Spark exclusion, five-minute
  natural-reset guard, UUIDv7 idempotency key, post-redeem quota refresh, and
  hard-affinity owner preservation are implemented. Failed/missing quota probes,
  non-quota failures, and non-OpenAI providers never spend a credit. Responses
  WebSocket precommit quota failures use the configured auto-redeem retry path.
  Known-owner reused-session transport failures reconnect once on the same
  account when session turn state is available. A fresh handshake that then
  reports `previous_response_not_found` retries up to three times on that owner
  with returned turn state; exhaustion preserves the upstream retryable event
  and retains affinity, so a later full-context replay stays on that owner.
  Durable WebSocket response-ID-to-turn-state lookup now survives
  router restarts through a bounded sidecar in the owning private `CODEX_HOME`;
  same-request cross-owner retry remains open. Routing refreshes
  launch-excluded profiles with bounded request model/route context and a
  five-minute freshness interval. Warm snapshots provide model- and route-aware
  availability and quota pressure; routing reuses them without an additional
  usage fetch. Fresh cached quota failures are skipped when another eligible
  account remains, and a fully cached-blocked pool fails open. Candidate ranking
  combines transient backoff time, quota pressure, weighted in-flight count,
  route-health penalties, and an in-flight soft limit while retaining deferred
  candidates as fallbacks.
  Equal-ranked candidates rotate deterministically. Route health for fresh and
  bound-owner responses persists in versioned `route-health.json`, decays by
  one point per minute, and is ignored after 14 days. A cold usage cache fails
  open for otherwise eligible accounts; account-level response and route-scoped
  transport backoffs persist. Other Prodex ranking inputs remain open.
  Profile-backed Gemini's disabled OAuth quota response and custom Codex
  model-provider metadata now match the 0.435.1 behavior; neither path persists
  API keys or claims provider usage data.
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
- `crates/prodex-app/src/runtime_proxy/precommit_loop.rs`,
  `health_backoff.rs`, `responses.rs`, and
  `standard/compact/recovery.rs` for Prodex 0.435.4 retry sweeps, recovery waits,
  Responses attempts, and Compact recovery.
- `crates/prodex-runtime-proxy/src/{attempt_outcome,previous_response_orchestration}.rs`
  and `crates/prodex-runtime-proxy/tests/src/attempt_outcome.rs` for bounded
  same-owner previous-response retries at 75 ms, 200 ms, and 500 ms.
- `crates/prodex-app/src/runtime_proxy/websocket.rs`, `websocket_message.rs`,
  `websocket_message/continuation_handling.rs`, `failure_response.rs`, and
  `lineage/{remember,lookup,release}.rs`, `core_constants.rs`, and
  `websocket/response_tracking/{precommit,quota_gate,previous_response,session}.rs`,
  plus
  `crates/prodex-app/tests/src/runtime_proxy/websocket/precommit_regressions.rs`
  for event-level selection, quota checks, affinity, commitment, and stale
  continuation behavior.
- `crates/prodex-app/src/runtime_proxy/quota/{gate,summary,cache}.rs` and
  `crates/prodex-runtime-quota/src/{summary,window}.rs` for pre-send quota
  decisions, five-minute probe freshness, and requested-model window selection.
- `crates/prodex-provider-core/src/translators/deepseek/{response,stream}.rs`
  and `mojo/prodex_core/openai_chat_response.mojo` for DeepSeek response and SSE
  translation behavior.

Local Codex source at `a04940cb` supplied queue grammar and the native debug,
app-server proxy, and daemon branches that discard CLI overrides or detach.

Historical local parser validation used an ephemeral official npm package
`@openai/codex@0.159.3` without replacing the developer's global Codex installation.
It reported `codex-cli 0.159.3`; managed provider strict-config and runtime-feature
smoke tests passed, and unknown strict-config fields stopped exec/resume/fork/review
before model work. That remains useful regression evidence, but it is not presented
as a 0.160.0 execution result. The Prodex 0.435.4 compatibility audit retains
Codex 0.160.0 as the target. The prior 0.435.1 provider-catalog/default/fallback
hotfix did not require a Codex model-transport change.
No live login, quota endpoint, refresh exchange, or model request is used for this
baseline migration.

## Verification and limits

The completion gate is `make verify`: formatting, source size, TUI framework,
all six release-target cross-builds, installer syntax/smoke checks, vet,
shuffled race tests, and the host build. Focused tests and both opt-in native
parser smokes run before it. The six targets are cross-built locally; native
Windows/macOS execution and live OpenAI behavior are not established by those
builds. Prodex's Rust/Mojo suite and release snapshots are outside this
source/test audit.

Commands passed for the earlier core checkpoint:

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

Commands passed for the historical 0.435.2 routing review; they do not verify
the 0.435.4 recovery delta:

```sh
rtk go test -race ./internal/usecase/routing ./internal/delivery/http/proxy ./internal/gateway/codex -count=1
rtk make verify
```

Stable bindings remain protected up to 8,192; opaque bindings expire after
30 days, and the in-memory cache remains at 4,096. Unknown/removed owners fail
closed. Affinity inspection remains bounded, including SSE metadata. The global
first-owner guard can serialize unrelated new conversations; use bounded lock
shards only if measured contention warrants it. The earlier core audit found no
further small correctness gap within its declared scope; it did not close the
0.435.2 gaps listed above. Startup SSE inspection can wait for output and shares
the global first-owner guard; there is no background polling or new reader
goroutine. SSE metadata remains bounded to 64 KiB per event, and compressed
stream bytes are preserved without inspection. These limits fail conservatively
and never permit replay after commitment.

An initial closure test run failed because the new test composition passed a
typed nil repository through an interface. The fixture was corrected, and focused
race tests and the final verification gate passed afterward. No checks were
weakened. No live authentication, quota, refresh exchange, or model turn was used
to establish this result.
