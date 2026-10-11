# Prodex 0.437.4 native completion recovery parity

The parity oracle is Prodex tag `0.437.4`, source commit
`893597e911a8a2922578a09500d32b8d7c400593`. The official Linux x86_64 release
binary is SHA-256
`6c45503762624f03dd010df258dcbff8ecd8d79312ed1627d034d95405e41efc` and
reports `prodex 0.437.4`.

Prodex 0.437.4 projects native Codex terminal failures from `event_msg`
payloads named `task_complete` and `turn_complete`, including unwrapped aliases.
These records carry `error.codex_error_info` without the app-server `status`
field. App-server `turn/completed` records still require `status: "failed"`.
Native `task_started` and `turn_started` records reset per-turn acceptance
evidence.

Godex now applies the same wire-shape boundary in
`internal/delivery/cli/runtime`. The existing classifier, profile eligibility,
session resume, committed-output, cancellation, and no-auto-rotate protections
remain the recovery policy owners.

Evidence captured for this change:

- The native completion regression fails on clean `origin/main` because the
  task-completion record is ignored and a successful completion leaves stale
  acceptance evidence armed.
- The same fixture passes after the parser and monitor boundary changes.
- Negative coverage rejects successful and cancelled completion records,
  untyped or quoted errors, foreign records, and old-turn evidence after a new
  native turn starts.
- Parity harness pins and release verification now use Prodex 0.437.4 and the
  checksum above.

The executable-level session-resume differential remains a separate runtime
fixture; no live OpenAI account or real model call is required for the focused
wire-format proof.
