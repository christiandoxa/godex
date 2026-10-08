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
- At the Follow-up 3 checkpoint, only **one** automatic continuation was launched. The tagged source's
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

## Follow-up 4: fresh-session evidence, compressed rollouts and goal transitions

The previously open recovery gaps were re-audited against Prodex's exact
`0.436.0` tag and the rollout and goal contracts in
`crates/prodex-app/src/app_commands/runtime_launch/usage_limit_recovery.rs`,
`usage_limit_recovery/workflow.rs`,
`crates/prodex-app/src/app_commands/runtime_launch/usage_limit_recovery/plan.rs`,
and `crates/prodex-app/src/runtime_tools/usage_limit_recovery.rs`.

**New verified behavior:**

- The Godex session repository now discovers `rollout-*.jsonl.zst`, parses
  the decoded Codex session metadata and preserves model, effort and
  source metadata. The streaming decoder, output and per-record buffers
  are bounded; file symlinks are excluded.
- A known-session child-exit checkpoint can now read compressed rollouts,
  including a compressed frame appended to an existing file or an atomic
  compressed rewrite. Instead of relying on compressed physical length or
  inode reuse, it verifies the SHA-256 of the previous **decoded prefix**
  and checks only the newly decoded suffix. Historical events, changed
  history, incomplete records and oversized compressed streams cannot
  authorize relaunch.
- Goal database status is checked read-only with parameterized SQLite
  queries. `active`, `paused`, `blocked` and `usage_limited` are
  resumable; terminal goals cannot be relaunched. A newer transition from
  `active`, `paused` or `blocked` to `usage_limited` can authorize the same
  guarded continuation even without a free-form error message.
  Missing goal databases still permit standard (non-goal) recovery.
  The SQLite file URI is normalized across Linux/macOS and Windows
  drive-letter paths, with `mode=ro` and the `query_only` pragma. A
  Windows CI regression on the initial parity commit was traced to an
  unnormalized path and covered by a dedicated cross-platform DSN test
  plus read-only database mutation-rejection test.
- Managed OpenAI `RunProfiles` now performs read-only session catalogue
  snapshots around a new headless `exec` invocation and can recover a
  **fresh** session if exactly one new session is discoverable, the
  persisted `session_meta` matches its UUID/path, and its accepted turn
  terminates with a structured recovery error. No recovery occurs for
  multiple concurrent new sessions, unknown/foreign identities, cancelled
  child processes, missing durable binding-release support or unsuitable
  fallback profiles. The new `exec resume` argument planner preserves
  persisted model/effort and avoids repeating the original prompt.
- Exact Codex structured recovery variants for `usage_limit`,
  `rate_limit`, `overload`, `auth` and `transport` are recognized
  from terminal event/turn errors. A bare 429 or plain-text log does not
  trigger replay. Acceptance evidence is reset at both Codex and app-server turn
  boundaries, so an old accepted turn cannot authorize a new
  ambiguous one. Multi-variant error unions fail closed.
- Each of the above paths has focused regression fixtures. The
  fresh-session test exercises the real `RunProfiles` dispatcher with
  a synthetic child that creates a Codex rollout, while compressed
  known-session and goal transition tests exercise the real recovery
  launcher and binding-release order.

**What remains different from exact Prodex:** the native session-start
hook mechanism and goal monitor while a child is running have not been
replicated. At the Follow-up 4 checkpoint, recovery performed
at most one safe continuation; see Follow-up 5 for bounded
multi-generation behavior. Prodex can also recycle transient pools
after backoff and handle additional rollout formats and edge cases. The existence of this new recovery path is **not** sufficient
evidence of complete 1:1 workflow parity.

## Follow-up 5: bounded, source-matched multi-generation profile recovery

The exact tagged Prodex `0.436.0` recovery strategy tracks attempted
profiles and avoids repeating a profile in the same recovery-pool pass
(`crates/prodex-app/src/runtime_tools/usage_limit_recovery.rs` and
`crates/prodex-app/src/app_commands/runtime_launch/run_command_strategy.rs`).

Godex now runs a **bounded pass over distinct eligible ChatGPT-managed
OpenAI profiles** for both already-known `exec resume` sessions and newly
discovered headless `exec` sessions:

- Before **each** next child launch, Godex checkpoints the same persisted
  Codex rollout and goal database. If that child fails, a subsequent
  recovery requires a *new* accepted-turn structured error or a new
  transition from `active`, `paused` or `blocked` to `usage_limited` relative to
  the latest checkpoint. A stale error from an earlier generation never authorizes
  another launch.
- Failed/previously attempted profiles are excluded from the current pool
  pass. Durable session-owner affinity is released before each retarget,
  and each new profile undergoes the existing runtime quota/account
  selection. Authenticated profiles are independently re-resolved.
- The same canonical `exec resume` plan preserves explicit
  `--cyber-access-program`, model and reasoning-effort settings while
  removing the original user prompt. Exit 130, context cancellation,
  unavailable owner release, terminal goal status, missing/newly malformed
  rollout or lack of fresh evidence ends the recovery without replay.
- The pass is capped at 32 **distinct** candidate profiles for bounded
  execution. Red/green tests exercise A→B→C success, absence of new
  second-attempt evidence (immediate stop), pool exhaustion without
  trying B twice, and fresh-session recovery through `RunProfiles` with
  the same protections. Tests inspect actual selected profile homes and
  preserved continuation arguments.

**Remaining difference at the Follow-up 5 checkpoint:** the transient
recycle timer, native `SessionStart` hook and live goal observer were
unimplemented. Follow-up 6 implements the cancellable transient timer;
the live session-start/goal-monitor contracts remain open.

## Follow-up 6: cancellable transient-pool retry after five seconds

Prodex's exact `0.436.0` source uses
`GOAL_USAGE_LIMIT_RETRY_INTERVAL = Duration::from_secs(5)` and
recycles the attempted-profile pool only when
`RuntimeWorkflowRecoveryClass::retries_after_pool_round()` is
true for `rate_limit`, `overload` or `transport`. Godex now
implements the corresponding **five-second, Ctrl+C-cancellable wait**:

- Once a pass has attempted at least one qualified backup and exhausts
  its distinct candidates, an evidence-verified transient failure logs
  that a retry is scheduled and waits five seconds. The next pass
  refreshes candidate profiles and account/quota availability, clears
  the attempted set for that round, and still excludes the original
  session owner. Failures without an accepted-turn structured marker,
  non-transient errors (including usage-limit and auth), or an entirely
  unqualified pool do **not** recycle or wait.
- Main's existing `signal.NotifyContext` delivers Ctrl+C and
  shutdown cancellations to this wait; a cancellation returns immediately.
  After the wait, the same per-child decoded-rollout/goal-state
  checkpoint and binding-release protections apply to every future
  attempt. No original prompt or previous tool call is resent as an
  instruction.
- Deterministic regression tests inject a no-delay wait to prove
  the sequence `A → B → C → (wait) → B` for a transient
  structured rate-limit, no pool recycling for usage-limit, stop on
  missing new evidence, and stop when cancellation occurs during a
  retry wait. The production interval and prompt abort are separately
  tested without sleeping five seconds in CI.

**Still different/unproven at this checkpoint:** Prodex's
native `SessionStart` and online monitor can make decisions *while* a
child is running. Follow-up 7 adds the trusted native headless exec
SessionStart hook; Godex still checks the verified rollout/goal
database **after child exit**. The asynchronous background persistence/probe queues
and full live differential provider/transport matrix also remain
unverified. Therefore these additional recovery contracts do **not**
constitute full 1:1 parity.

## Follow-up 7: trusted native Codex SessionStart for managed headless exec

Tagged source: `crates/prodex-app/src/app_commands/runtime_launch/goal_resume.rs`
(`add_runtime_goal_session_tracking`, `runtime_goal_session_hook_hash`,
`handle_runtime_goal_session_notify_if_requested`).

- Godex now injects the source-matched `-c hooks.SessionStart=[...]`
  command hook and `hooks.state` trusted SHA-256 identity into
  eligible managed OpenAI `exec` starts. The canonical hash
  is derived from the exact normalized session-start command identity,
  with the tagged five-second hook timeout and platform-specific
  POSIX/Windows command escaping. A user-provided
  `hooks.SessionStart` override is never overwritten.
- The hidden `__runtime-goal-session-notify` CLI command is handled
  **before** ordinary Godex config/credential discovery. It accepts
  bounded (64 KiB) Codex JSON payloads and validates the UUID from
  `thread-id` or `session_id`, rejecting conflicts. The
  callback writes **exactly one** newly created marker under a
  user-private temporary directory; symlinks, unexpected file names,
  repeated writes and invalid payloads fail closed.
- After the child exits, the managed headless recovery path can use
  this verified marker to select **one** new session even if
  unrelated concurrent Codex rollout files were created. Historical,
  unknown and duplicate IDs do not bypass the session-owner and
  acceptance-evidence verification. Without a valid marker, the
  existing requirement of exactly one newly discovered session remains.
- Tests cover canonical hook-hash identity, Windows/Unix command
  escaping, user hook preservation, callback-before-config dispatch,
  payload bounds, symlink/replay protection, native process-marker
  delivery and real `RunProfiles` disambiguation among two
  concurrent rollouts. The official Codex CLI `0.161.0` also
  accepts the injected hooks configuration under
  `--strict-config exec-server --listen stdio` with no model
  turn. Intentional production mutations of the trust hash,
  marker validation, user override and consumer dispatch are
  detected by the same regression fixtures.

**Still open:** the exact Prodex monitor also maintains live goal
state while the Codex child runs, injects an optional `notify`
fallback where appropriate, and handles native TUI and other
app-server transports. The hook added here covers **headless
managed `exec` only**; it does not certify those other
monitoring/lifecycle paths or the asynchronous background-queue
sources as 1:1 equivalents.

## Still NOT closed; do not promote as full parity

1. **Complete queue/backoff lifecycle parity:** the Compact policy is
   implemented and tested with source-matched backlog thresholds and a
   local-rejection backoff. But normal Godex execution still does not
   expose Prodex's three asynchronous queue sources or independently
   reproduce all worker/overload scenarios. This remains unverified
   end-to-end; mere admission-lane saturation must not trigger shedding.
2. **Full goal/workflow relaunch lifecycle:** fresh-session `exec`
   discovery, bounded compressed rollout reads, read-only goal status
   transitions, structured transient error classes and per-generation
   distinct-profile rotation now have meaningful source-matched tests.
   The headless managed-exec session-start hook is now implemented
   with native Codex 0.161.0 config validation, but the online
   in-process goal monitor, TUI/other app-server hook integration,
   optional notify fallback and all native launch/continuation
   conditions are **not** fully reproduced. The five-second
   transient-pool scheduler has cancellable tests, but real-world
   workflow parity remains unproven.
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
