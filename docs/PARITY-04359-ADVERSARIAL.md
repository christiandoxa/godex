# Adversarial parity checkpoint — Prodex 0.435.9

**Assessment: partial parity with verified contracts; exhaustive 1:1 parity is not certified.**

Reference: exact Prodex tag `0.435.9`, commit
`c9cede4b14f6865ddc3281acc82315d2693cc3c8`.
Godex's prior `main` checkpoint was
`93b01aacbb357dfefba77420ca69fcab5591d1ab`.
This document supplements the older, still in-progress `docs/PARITY.md` audit,
which begins at Prodex `0.435.5`. It does not overwrite that document's
uncommitted work.

## Observed and regression-protected

| Reference contract | Godex proof |
| --- | --- |
| Prodex 0.435.9 profile-capacity waiting does not burn upstream precommit time or recovery sweep count | `internal/usecase/routing/fanout_04359_test.go`, two independent production mutation sentinels |
| Released capacity and the waiter generation become visible atomically | `internal/usecase/routing/inflight_generation_04359_test.go`, release-signal mutation sentinel |
| Concurrent HTTP and WebSocket-message sessions isolate replies and dispatch each work item once | `fanout_04359_test.go`, `websocket_fanout_04359_test.go`; the latter tests routing rather than the complete network WebSocket stack |
| Session selector compares IDs/prefixes with ASCII-only casefold (new Mojo policy) | `internal/usecase/session/selector_04359_test.go`, Unicode mutation sentinel |
| Copilot provider binds nested `response.id`, root `id`, `response_id`, native `responseId`, and `message.id` without changing generic OpenAI event-ID rules | `internal/usecase/routing/copilot_affinity_04359_test.go`; this audit found and fixed root/native ID omissions |
| Native Copilot binding also observes IDs from *late* committed SSE events without rewriting forwarded bytes | `internal/delivery/http/proxy/copilot_affinity_04359_test.go`; provider-policy propagation mutation sentinel |
| Missing/blank IDs, field precedence, Unicode whitespace, and SSE duplicate IDs follow tagged Copilot policy | `TestProdex04359CopilotResponseIDFallbackAndSSEPrecedence`; tagged source `crates/prodex-app/src/runtime_launch/proxy_startup/local_rewrite_copilot_bindings.rs` |
| OpenAI ping CLI exposes tagged `--effort LEVEL` in help, not only argument parsing | `internal/delivery/cli/public_help_04359_test.go`; tagged source `crates/prodex-cli/src/ping.rs` |

The mutation procedure for each newly corrected observable contract is:
write a failing regression against production, fix, deliberately revert a
single production behavior, observe the sentinel fail, restore the original
production bytes, and repeat the focused/race tests.

## Still open or not independently proven

1. **Full 1:1 contract coverage is not established.** The complete Prodex
   Rust/Mojo contract corpus has not been executed differentially against
   Godex. Go CI passing is necessary but not sufficient for cross-implementation
   behavioral equivalence.
2. **All local-admission pressure decisions are not yet differential-tested.**
   Prodex's tagged `crates/prodex-runtime-proxy/src/admission.rs` includes
   global-overload marking and fresh Compact shedding under pressure, not just
   capacity admission. Godex's `internal/delivery/http/proxy/admission.go`
   has weighted/lane waiting, but no proven end-to-end equivalence for every
   fresh-versus-continuation pressure state.
3. **Other stateful runtime differences need a matching fixture corpus.**
   WebSocket continuation failure/replay, provider selection under concurrent
   quota refresh, full TCP WebSocket fanout, and remote app-server behavior
   have focused tests but no exhaustive bidirectional parity proof.
4. **Codex 0.161 transport compatibility is not validated against the exact
   upstream binary here.** Prodex 0.435.9 audits `rust-v0.161.0`
   (`migration/codex-rust-v0.161.0-audit.md`), whereas the installed
   `codex --version` during this audit reports `0.160.1`.
   This is a *verification limit*, not a claim that 0.160.1 is unsupported.
5. **No live upstream/provider authentication and failure-injection matrix**
   has been executed for every provider and operating system. Tests using
   mocks do not establish reliability against real provider/network outages.

## Release gate

For any subsequent promotion, retain exact SHA mapping, clean and shuffled
Go tests (including race tests), static checks, clean detached worktree,
cross-platform Actions, source-anchored red/green regression tests, and
adversarial mutations. **Do not relabel these checks as an exhaustive parity
certificate.** Update this list only after a counterexample is closed or
the remaining contract is independently proven.
