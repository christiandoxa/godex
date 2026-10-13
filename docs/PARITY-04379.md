# Prodex 0.437.9 behavioral parity checkpoint

Canonical reference: tag `0.437.9`, commit
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`.
The official Linux x86_64 release executable has SHA-256
`58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`,
matching its published `SHA256SUMS` entry. Both the exact source and the
binary are required by the active independent differential tools and the
release-only parity script. This replaces the prior 0.437.8 oracle target;
historical checkpoint evidence remains in `PARITY-04378.md`.

## Behavioral changes addressed

1. **Responses keepalive before output.** The exact `keepalive` event is
   transport liveness, not committed model content. Godex now keeps it in
   precommit lookahead for HTTP SSE and upstream Responses WebSocket. A later
   retryable capacity failure can rotate safely, dropping the failed prelude.
   Actual output, unknown event kinds, and a previously committed stream still
   prohibit replay. Regression tests failed on the preceding Go implementation
   and pass with the new classification. The HTTP loopback test exercises both
   declared SSE and missing upstream Content-Type with unchanged request body,
   account rotation, and post-output negative controls. Byte-split SSE tests
   verify every split of the synthetic heartbeat/overload sequence; the
   WebSocket fixture exercises its real framed upstream protocol boundary.

2. **Root native session marker.** Codex SessionStart's `session_id` now
   takes precedence over legacy notify's concrete `thread-id` when both
   appear. A legacy callback bootstraps an absent marker but cannot redirect
   an established root; a repeated root is a no-op, and an explicit new root
   replaces it. The trusted ephemeral marker is serialized under an owned
   OS lock and atomically published as a private file. The Go monitor's
   in-memory unread scan offset is not reset by repeated callbacks. Synthetic
   regressions cover conflicting identifiers, malformed canonical identity,
   explicit root changes, and simultaneous descendant callbacks. Unlike the
   Rust implementation, Godex does not persist an independent rollout-offset
   sidecar; equivalence of recovery across monitor restarts and compressed
   rollouts is **not** established.

No user-visible feature, Codex model context window, compaction setting,
credential handling, or postcommit retry policy was changed.

## Official executable qualification

All runs below used isolated local homes, synthetic credentials, no live
model inference, and official Prodex 0.437.9. Godex candidate
`0b3f1b80da5a2bd06ec4d243332e5d183a7c744e` was built from a clean
committed checkout with Go VCS provenance.

- Managed profile lifecycle oracle: **PASS**, 31 subprocess stages.
- CLI rejection oracle: **PASS**, eight probes and negative controls.
- Native model-context oracle: **PASS**, three explicit cases and cached-model
  negative control.
- Responses continuation loopback: **PASS**, observed account attempts,
  overload, quota, cancellation, and commitment protections.
- Strict DeepSeek mock differential: **PASS** for each of 18 scenarios
  invoked individually (success, tools, user-id, streamed output/limits,
  retry, 401/403/429/500/503, credential rotation/restart, embedded
  stream failure, cancellation, restart, and recovery after 429/503).
  Each invocation retained its strict negative controls and state auditor.
  The combined `--scenario all` invocation was not executed here; do not
  infer that the combined release script passed.
- `go test ./internal/usecase/routing ./internal/gateway/openai -run TestProdex04379 -count=1 -timeout=20s`: **PASS** after failing-before regressions.
- `go test ./internal/delivery/cli/runtime -count=1 -timeout=75s`: **PASS**.
- `go test ./internal/delivery/http/proxy -run TestProdex04379PublicResponsesHeartbeatRecoveryAndCommitFence -count=1 -v -timeout=30s`: **PASS**.
- Focused shuffled `-race` package tests for routing/OpenAI gateway,
  CLI runtime, and HTTP proxy: **PASS**.
- `make verify`: **PASS** (format, source size, Bubble Tea guard, six
  cross-build targets, installers, vet, shuffled race tests and Go build).

## Remaining release blockers

This checkpoint is **not full 1:1 feature parity**. Cross-binary live-goal
recovery with native SessionStart and inherited descendant hooks, independent
rollout-offset durability, cross-process state recovery, WebSocket message
deadline/ownership qualification, broader provider protocols, live OAuth and
quota interactions, and cross-platform native execution still require
independent behavioral evidence. Mock differential agreement for a subset of
contracts is not equivalent to all material behavior.

The `v0.2.1` release gate remains fail-closed. No release or tag is authorized
by these tests, and CI success is not by itself a parity certificate.
