# Prodex 0.436.0 parity follow-up: admission and remaining blockers

**Status: partial parity.** This checkpoint improves source-matched admission
behavior. It does not certify full 1:1 behavior against Prodex 0.436.0,
and it does not revise the protected WIP document `docs/PARITY.md`.

## Exact references

- Prodex tag `0.436.0` at
  `3e1e065b05635a21851b7d6752fb9867026b85a7`.
- Godex release v0.2.0 at
  `43b1a8f6873ba38e973a09620b4f10f9ecc06709`.
- Tagged Prodex sources:
  `crates/prodex-app/src/runtime_proxy/standard.rs`
  (`runtime_proxy_should_shed_startup_metadata_for_counts`,
  `runtime_startup_metadata_response_for_path`,
  `runtime_proxy_startup_standard_lane_priority_path`),
  `crates/prodex-app/src/runtime_proxy/lifecycle.rs` (startup priority
  bypass preserves the global limit), and
  `mojo/prodex_core/runtime_state_background.mojo` (admission policy).

## Closed in this follow-up

- Optional Codex startup requests receive Prodex's synthetic `200`
  `{"items":[],"data":[]}` or `204` response under Standard-lane
  pressure (at least half of the Standard lane limit), without dispatching
  upstream or consuming a new permit. Paths are matched after canonical
  backend-api mount and optional version-segment normalization.
- Critical `/backend-api/codex/models` and `/backend-api/ps/mcp`
  requests bypass **only** the Standard lane limit; the global limit is
  still authoritative. Unrelated Standard routes continue to wait.
- These response and priority contracts were reproduced as failing
  tests before the code change, and their intentional production
  mutations were detected by the same tests. See
  `internal/delivery/http/proxy/startup_admission_04360_test.go`.
- The original unbounded, cancellation-aware capacity backpressure
  remains intact. This checkpoint does not replace all queued requests
  with eager 503 responses.

## Still NOT closed; do not promote as full parity

1. **Local-overload pressure and fresh Compact shedding**: the tagged
   policy sheds *fresh unowned* Compact work only during pressure, while
   protecting bound owner continuations. Prodex derives pressure from
   explicit local overload backoff and three background queue backlogs,
   **not merely from observing a full admission lane**. Godex currently
   has synchronous persisted state and a backpressure-focused admission
   handler, not the same queue/backoff state owners. It would be
   incorrect to enable blanket Compact rejection on ordinary saturation.
   A compatible pressure source and end-to-end differential pressure
   fixtures are still required.
2. **Automatic usage-limit/goal retry after child exit**: exact Prodex
   `crates/prodex-app/src/runtime_tools/usage_limit_recovery.rs`
   observes child-exit signals, selects a recovery profile, retargets
   `exec resume` using the canonical native planner, carries
   model/effort and program arguments, and may schedule further retries.
   Godex's existing router handles upstream precommit recovery and its
   session catalogue resolves explicit continuations; this is **not**
   evidence of parity for the full Codex child-exit relaunch workflow.
3. **Continuations and backpressure**: Prodex permits a *verified owner*
   to bypass a saturated route lane while retaining the global cap.
   Additional safe admission-time owner-resolution tests (including
   bounded request-body inspection) are needed in Godex.
4. **Broader parity**: live provider auth/failure matrix, every
   WebSocket/HTTP continuation case, remote Codex app-server
   boundaries and Rust/Mojo-vs-Go differential output corpus remain
   unverified. CI/release success and unit-test coverage do not imply
   cross-implementation equivalence.

The v0.2.0 tag remains immutable and is **not** moved by this follow-up;
subsequent verified changes are committed directly to `main`.
