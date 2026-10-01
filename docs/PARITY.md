# OpenAI/Codex core parity audit

Reference: exact Prodex tag `0.434.2`, commit
`82b3f9585f4b1356057243f23f2edb2687fa22cd`. The tag's `Cargo.toml`
declares `0.434.2`. Comparison reads used tagged Git objects, rather than the
newer Prodex working checkout. This audit covers Godex's declared OpenAI/Codex
core after the preserved `6aae186` baseline, the `93db608` checkpoints, and the
final closure below. It does not claim feature-for-feature Prodex parity.


## 1:1 parity expansion

The project target is now feature-for-feature parity with Prodex `0.434.2`, not
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
profile, session, and foreground managed HTTP/SSE runtime scope. The final audit
read the exact tagged Prodex source alongside Godex production code, callers,
tests, and local Codex 0.159.2 source. It found and closed these remaining gaps:

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

- Multi-provider runtime/login bridges (Gemini, Anthropic/Claude, Copilot, Kiro,
  DeepSeek/local/AGY where present in Prodex), including provider catalogs and
  provider-specific auth/routing semantics.
- Super mode and its hidden expose/broker/MCP bridge/sub-agent execution stack,
  including optional Presidio integration.
- The standalone gateway surface, full live TUI parity, process/resource metrics,
  audit-log backend, and the richer runtime-policy/diagnostic bundle surfaces.
- Self-update, reset-credit redemption, and cost-bearing `ping openai` diagnostics.
- Built-in non-OpenAI profile import sources (Claude, Copilot, Kiro) and provider
  secret-file bundle payloads. Interactive password-selection/password-entry TUI
  and process-crash lifecycle-journal recovery for multi-profile imports also
  remain to match Prodex exactly. OpenAI plain/encrypted bundle wire formats and
  identity-safe runtime rollback are implemented.
- HTTP/SSE model transport is explicit; Godex does not implement Prodex's
  WebSocket/Realtime forwarding. Unexpected upgrades fail before upstream work.
- Import-current is auth-only, not full native-home migration. Existing native
  configuration, rollouts,
  history, and databases remain owned by Codex; homes are not symlink-shared.
- Remove keeps its existing destructive contract. Disable and logout provide
  retained deactivation; no second archive tree or changed removal default.
- Native names/pickers/last remain profile-local rather than a shared-session UI.
  Explicit UUIDs/prefixes provide the cross-profile workflow.
- Quota currently has bounded one-shot compact/detail/raw views; Prodex live
  quota dashboard/watch behavior, automatic credit redemption, and provider-wide
  quota catalog remain to be implemented for 1:1 parity.
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

Installed `codex-cli 0.159.2` was exercised with local parser/capability smokes.
Strict validation accepts managed provider configuration and actual wrapper
feature output. Deliberately unknown strict-config fields stop exec, nested
resume/fork/review, and direct resume/fork/review before model work. No live
login, quota endpoint, refresh exchange, or model request was performed.

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
