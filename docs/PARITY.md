# OpenAI/Codex core parity audit

Reference: exact Prodex tag `0.434.2`, commit
`82b3f9585f4b1356057243f23f2edb2687fa22cd`. The tag's `Cargo.toml`
declares `0.434.2`. Comparison reads used tagged Git objects, rather than the
newer Prodex working checkout. This audit covers Godex's declared OpenAI/Codex
core after the preserved `6aae186` baseline and the accompanying compatibility
and correctness checkpoints. It does not claim feature-for-feature Prodex parity.

## Implemented practical parity

| Capability | Godex implementation and observable coverage |
| --- | --- |
| Isolated ChatGPT accounts | Official Codex interactive/device login, per-account homes, identity deduplication, deterministic and unambiguous selectors. Account/auth tests cover registration and selection. |
| Safe profile lifecycle | Repeat login/import replaces credentials while retaining native state. Metadata-only journals recover interrupted operations; owned OS locks and shared profile leases exclude concurrent credential mutation/removal. Account repository tests cover recovery, leases, and unsafe paths. |
| Account retention and native auth | Enable/disable retains the home. Managed status/logout bypass quota/rotation; logout uses an exclusive lease. Unsafe mutating auth passthrough is rejected with managed-command guidance. |
| Session discovery and launch | Bounded metadata catalog, list/current filters, text/JSON/ID/resume-command output, unique prefixes, and bare UUID resume. Native resume/fork, including nested exec forms and root options, resolve the rollout home; local deletion/archive stays local. Delivery/session/runtime tests cover argument preservation and selector conflicts. |
| Quota and fresh selection | One-shot compact/detailed usage windows, reset timestamps, fail-open probe uncertainty, deterministic bounded selection, and temporary exhaustion deadlines. Explicit selectors remain fixed. Quota/runtime tests cover exhaustion, uncertainty, and reset eligibility. |
| Managed Codex configuration | HTTP/SSE Responses provider keeps native account/bootstrap HTTPS. Managed config enters the innermost exec scope; user overrides retain precedence. Routing/auth-store overrides, quoted/equals forms, whole provider tables, OSS/local providers, and remote app-server routing cannot bypass it. The Codex delimiter preserves literal arguments. |
| Durable conversation ownership | Hashed, bounded, versioned bindings survive restart/cache expiry. Requested owners beyond cache capacity resolve correctly. Independent routers serialize first-owner selection under an OS guard. Native picker/name/last resumes keep the rollout home and the enabled owner pool; explicit account scope remains fixed. |
| Safe HTTP rotation and streaming | Selected bearer and ChatGPT routing ID replace caller credentials. Bounded retries occur only before commitment. Known continuations preserve their owner; unknown opaque continuations fail closed. Streams flush and preserve bytes/headers/trailers; committed failures abort downstream without replay. HTTP/routing tests cover chains, concurrency, restart, and real broken streams. |
| Native process and installation surfaces | Native Codex owns models, tools, sandbox/approval behavior, session replay, and token refresh. Godex preserves arguments and meaningful child exit status. Existing checksum installers and release naming remain intact; no installation/release files changed in this audit. |

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
dependency, daemon, or background worker was introduced.

## Deliberate differences and exclusions

- OpenAI/ChatGPT through Codex only: no multi-provider bridges, Super mode,
  dashboard/custom TUI, enterprise gateway, SQL/Redis/Postgres infrastructure,
  daemon, observability backend, plugin runtime, or self-update.
- HTTP/SSE model transport is explicit; Godex does not implement Prodex's
  WebSocket/Realtime forwarding. Unexpected upgrades fail before upstream work.
- Import-current is auth-only, not full native-home migration or encrypted
  credential bundle import/export. Existing native configuration, rollouts,
  history, and databases remain owned by Codex; homes are not symlink-shared.
- Remove keeps its existing destructive contract. Disable and logout provide
  retained deactivation; no second archive tree or changed removal default.
- Native names/pickers/last remain profile-local rather than a shared-session UI.
  Explicit UUIDs/prefixes provide the cross-profile workflow.
- Quota is a bounded one-shot view; no live dashboard, background polling,
  automatic credit redemption, or provider-wide quota catalog.
- Godex reloads Codex-owned auth on an authentication retry; it does not implement
  OAuth/token refresh, aggressive history rewrites, or silent model relaunch.

## Reference evidence

Tagged Prodex sources inspected include:

- `crates/prodex-runtime-launch/src/args.rs` for governed HTTP provider config and
  exec config scope, and `crates/prodex-cli/src/runtime_features.rs` for flags.
- `crates/prodex-cli/src/session_context.rs` and `profile.rs` for session/profile
  contracts, and profile login lifecycle/removal implementations in `prodex-app`.
- `crates/prodex-app/src/runtime_proxy/upstream.rs`, `selection/affinity.rs`,
  `responses/affinity_state.rs`, `standard/attempts/precommit.rs`, and
  `response_forwarding/streaming_writer.rs` for auth, ownership, and commitment.

Installed `codex-cli 0.159.2` was exercised with local parser/capability smokes.
Strict validation accepts managed provider configuration and actual wrapper
feature output. Deliberately unknown strict-config fields stop exec, nested
resume/fork/review, and direct resume/fork/review before model work. No live
login, quota endpoint, refresh exchange, or model request was performed.

## Verification and limits

The completion gate is `make verify`: formatting, source size, vet, shuffled
race tests, and build. Focused tests and both opt-in native parser smokes run
before it. Windows amd64 is cross-built locally; native Windows/macOS execution
and live OpenAI behavior are not established by that build. Prodex's Rust/Mojo
suite and release snapshots are outside this source/test audit.

Passed commands:

```sh
rtk go test ./internal/gateway/codex ./internal/delivery/cli/runtime
rtk go test ./internal/delivery/cli/runtime ./internal/usecase/runtime ./internal/usecase/session
rtk go test -race ./internal/usecase/routing ./internal/repository/routing ./internal/delivery/http/proxy
GODEX_TEST_CODEX_BIN="$(command -v codex)" rtk go test ./internal/gateway/codex ./internal/delivery/cli/runtime -run 'TestInstalledCodex.*Smoke' -v
rtk make verify
GOOS=windows GOARCH=amd64 go build -trimpath -o /tmp/godex-final-windows-amd64.exe ./cmd/godex
git diff --check
```

Stable bindings remain protected up to 8,192; opaque bindings expire after
30 days, and the in-memory cache remains at 4,096. Unknown/removed owners fail
closed. Affinity inspection remains bounded, including SSE metadata. The global
first-owner guard can serialize unrelated new conversations; use bounded lock
shards only if measured contention warrants it. No further small correctness
gap was found in the reviewed declared core after these fixes.
