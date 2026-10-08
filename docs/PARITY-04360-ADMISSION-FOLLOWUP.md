# Prodex 0.436.0 parity follow-up: admission, recovery and remaining blockers

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

## Follow-up 2: pressure policy and verified-owner lane admission

Prodex source anchors: `mojo/prodex_core/runtime_state_background.mojo`,
`crates/prodex-app/src/runtime_proxy/lifecycle.rs` (owned-lane bypass),
`crates/prodex-app/src/runtime_proxy/standard/compact.rs` (fresh Compact
pressure shed), `crates/prodex-app/src/runtime_proxy/standard/compact/admission.rs`
(503 JSON), and `crates/prodex-app/src/core_constants.rs` (backlog thresholds).

- A verified, still-registered previous-response/turn owner bypasses a
  saturated Responses lane; a Compact turn/session owner (including its
  persisted compact-session lineage alias) bypasses a saturated Compact
  lane. **Neither bypasses the global request cap.** Unknown or conflicting
  bindings cannot obtain lane priority.
- Ownership is checked at actual HTTP ingress through
  `Router.HasVerifiedAdmissionOwner` using an inspection bounded by
  `MaxRequestBytes`, and any read bytes are replayed unchanged to
  downstream handling. WebSocket admission permits only existing header
  metadata without reading upgrade payloads.
- Under explicitly reported local overload or a background queue backlog
  of at least state-save **8**, continuation journal **8**, or probe-refresh
  **16**, fresh Compact requests are shed with the source-matched
  `503 service_unavailable` JSON. Verified Compact owners continue; a
  mere saturated lane **does not** imply overload.
- A genuinely rejected local admission starts a **3-second** backoff and
  returns a Retry-After hint; an otherwise successful capacity wait
  does not. Optional startup metadata can also be shed on reported
  Standard-lane background/local pressure, even below half its lane cap.
- Red/green tests exercise the public HTTP handler, full body replay,
  conflicts, aliases, global-cap refusal and actual source-derived
  thresholds. Intentional production mutations of lane bypass, Compact
  shedding, queue threshold and body replay were detected and restored.

**Coverage limitation:** `Proxy.Config.PressureSnapshot` is an optional
provider of those queue backlogs. Godex does **not** have the exact
three Prodex asynchronous state-save, journal and probe queues to wire
into it in ordinary production yet; unset inputs mean no imaginary
backlog pressure. This closes the *decision policy when a truthful signal
exists*, not the complete source-to-output parity of all overload scenarios.

## Follow-up 3: guarded known-session child-exit recovery and native launch

Exact tagged source owners:
`crates/prodex-app/src/app_commands/runtime_launch/usage_limit_recovery.rs`
(rollout evidence windows, usage-limit classification),
`crates/prodex-app/src/app_commands/runtime_launch/usage_limit_recovery/plan.rs`
(success/cancellation/no-auto-rotate guards and profile selection),
`crates/prodex-app/src/runtime_tools/usage_limit_recovery.rs` (the relaunch
orchestration), `mojo/prodex_core/launch_args.mojo` (RetargetExec),
`crates/prodex-runtime-launch/tests/src/lib/args_codex_0161.rs` (Cyber option
retarget), and `crates/prodex-app/src/runtime_launch/profile.rs` (rotation
eligibility).

**Now covered for a previously resolved `exec resume` session:**

- The session catalogue passes the resolved report and durable binding-release
  callback to a recovery-aware launcher. Existing launchers still use the
  unmodified launch interface.
- Before invoking Codex, a checkpoint records the end of a private, regular
  `.jsonl` rollout. After a non-success child exit, a recovery is eligible
  only if **new, complete** records show an accepted user turn followed by
  an exact structured usage-limit error for this session. The scan is bounded
  (1 MiB total and 64 KiB per record), checks nested session identity, and
  rejects symlinks, old errors, untrusted plain-text claims, truncated events
  and cancelled executions.
- The fallback requires auto-rotation to be allowed, a second enabled,
  authenticated OpenAI/ChatGPT managed profile, and successful durable
  release of the old session-owner binding. Existing profile/quota preflight
  runs again before the backup launch. No profile is silently created.
- Only **one** automatic continuation is launched. The tagged source's
  `exec resume` native argument policy is preserved: original prompt and
  `--thread-source` are discarded, the new session ID replaces `--last`
  or a previous ID, and explicitly selected model/reasoning settings and
  Codex `--cyber-access-program` remain intact. The continuation prompt
  explicitly instructs Codex not to repeat completed tool actions.
- Synthetic child-process integration and session-catalog dispatch tests
  demonstrate the successful recovery path **and** missing marker, missing
  binding release, backup not authenticated, `--no-auto-rotate`, and
  conflicting/foreign/stale signal fail-closed cases. No upstream model
  turn was sent by these tests.
- The default Godex `run --dry-run` and selected OpenAI launch now use the
  exact Prodex decision to **omit a synthetic rotation proxy** without a
  qualifying profile pool (at least two profiles, existing selected home,
  and a quota-compatible profile). A real `prodex 0.436.0` binary and
  locally built Godex were run in isolated credential-free homes; both
  reported `Provider: openai`, `Runtime proxy: disabled`, and preserved
  the native `--cyber-access-program standard` argument. Synthetic
  production-launch tests verify that the non-qualified native path is
  executed, rather than merely printed in the dry-run UI.
- `run.go`'s original native-command helpers were moved to a dedicated
  source file, lowering its source-size baseline from 477 to 431 lines.

**Not yet equivalent:** the exact Prodex monitor also discovers **fresh**
session IDs via Codex's session-start hook, watches goal database state,
reads compressed `.jsonl.zst` rollouts, classifies additional transient
workflow errors, and retries qualified profile pools across multiple
recovery generations. Godex's new child-exit recovery is **only for a
previously resolved `exec resume` session with a recent structured
usage-limit signal and a verified backup OpenAI account**. Do not infer
full retry/replay or goal-monitor parity from its existence.

## Still NOT closed; do not promote as full parity

1. **Complete queue/backoff lifecycle parity:** the Compact policy is
   implemented and tested with source-matched backlog thresholds and a
   local-rejection backoff. But normal Godex execution still does not
   expose Prodex's three asynchronous queue sources or independently
   reproduce all worker/overload scenarios. This remains unverified
   end-to-end; mere admission-lane saturation must not trigger shedding.
2. **Complete child-exit goal/workflow recovery:** known `exec resume`
   usage-limit continuation is now guarded and exercised end-to-end with a
   synthetic Codex process. Fresh-session ID discovery, goal database
   transitions, compressed rollout scanning, other transient workflow
   classes and multi-generation pool retry remain different or unproven.
   This is not a full replacement for Prodex's recovery monitor.
3. **Full continuation/transport coverage:** HTTP Responses/Compact
   admission now bypasses the saturated lane for a verified owner and
   retains the global cap, with bounded request-body inspection.
   The entire native WebSocket-message and persistence/restart matrix
   still needs differential proof against the Prodex executable.
4. **Broader parity**: live provider auth/failure matrix, every
   WebSocket/HTTP continuation case, remote Codex app-server
   boundaries and Rust/Mojo-vs-Go differential output corpus remain
   unverified. CI/release success and unit-test coverage do not imply
   cross-implementation equivalence.

The v0.2.0 tag remains immutable and is **not** moved by this follow-up;
subsequent verified changes are committed directly to `main`.
