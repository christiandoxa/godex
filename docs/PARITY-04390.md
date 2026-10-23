# Prodex 0.439.0 behavioral parity

Full 1:1 parity is **not certified**. PASS applies only to the evidence named
in a row. FAIL includes missing current-release evidence; BLOCKED identifies
verification requiring an unavailable external capability. Historical reports
do not qualify the current candidate.

## Exact reference

- Repository: <https://github.com/christiandoxa/prodex>
- Tag: `0.439.0`; commit: `433aad44bb249d5c25a231a6b06c2fa522a39a1f`
- Linux x86-64 executable SHA-256:
  `2dec9565fd94eb59aeb83ae5e7cc4b772db130cbd45258d86a2104c8f4ba186a`
- Release manifest SHA-256:
  `ba2f1f8d3430bd3fe64addd886ebee1207bf59b9db0d0e34b834b0dc7a0008f8`

The clean detached source resolves to the exact release commit. The downloaded
official executable and manifest match the published SHA256SUMS; the executable
reports `prodex 0.439.0`. All six native-platform pins come from those sums.
Only the Linux x86-64 executable has been executed in this campaign.

The previous finite gate targets 0.438.4, documented in
[PARITY-04384.md](PARITY-04384.md). The exact 0.438.4-to-0.439.0 source delta is
566 files, 45,098 insertions and 5,146 deletions. The release includes compiled
presentation, auth-header, provider-config, session and launch-policy changes;
internal migration alone proves no behavioral equivalence. Intermediate
0.438.0/1/2/3 contracts remain recorded in their versioned parity documents.
Their current executable regressions remain in the gate.

## Evidence matrix

| Reference behavior | Godex behavior / difference | Test or fixture | Owner | Result / limitation |
| --- | --- | --- | --- | --- |
| Exact tagged source, binary and manifest | Active oracles pinned to 0.439.0; older identities rejected | Release checksums, version, provenance guards | `tools/*parity`, `scripts/verify-*parity*` | PASS reference provenance; candidate differential pending |
| Custom cache key independent of session header; omitted key stays absent | Raw forwarding exists; explicit new-release comparison pending | Tagged `http_backend_passthrough.rs`; HTTP oracle | `gateway/openai`, `usecase/routing`, HTTP delivery | FAIL missing 0.439.0 executable evidence |
| Merged turn metadata and unknown fields survive precommit rotation | Raw request forwarding and rotation exist | Tagged `http_followups/memory_metadata.rs` | `usecase/routing`, `gateway/openai` | FAIL missing 0.439.0 executable evidence |
| Repeated root/exec/resume config overrides retain precedence | Scope-aware Codex gateway exists | Tagged `args_resume.rs`; context oracle | CLI runtime delivery, `gateway/codex` | FAIL expanded scoped comparison needed |
| Inline native commands remain prompt text | Native argument forwarding exists | Tagged `args_resume.rs` | CLI runtime delivery, `gateway/codex` | FAIL current executable comparison needed |
| Codex 0.164.0 initialization and thread listing | Opt-in official qualification currently covers 0.163.0 | Official app-server fixture | `gateway/codex` | FAIL new official binary probe needed |
| Native metadata-patch and model-instruction ownership | Delegated to official Codex; no wrapper merge | 0.164.0 schema / source qualification | `gateway/codex` | FAIL exact upstream contracts need qualification |
| Failed compaction attempts stay isolated | Native compaction delegated; translated Compact is separate | Native lifecycle and Compact oracles | `gateway/codex`, `gateway/compact` | FAIL no comprehensive 0.439.0 evidence |
| Compressed rollout resume preserves invocation choices | Existing compressed-session support | Session repository and resume tests | Session domain, `gateway/codex` | FAIL current executable qualification needed |
| Hard continuation ownership and no postcommit replay | Existing bounded routing and recovery | Continuation HTTP/WebSocket oracles | `usecase/routing` | FAIL current executable rerun needed |
| Precommit quota/retry, weighted capacity and watchdogs | Existing admission and recovery | HTTP/WebSocket deadline/capacity phases | Routing use case, HTTP delivery | FAIL current executable rerun needed |
| Concurrent durable state and recovery | Atomic stores, journals and conflict checks exist | Profile, restart, ownership oracles | Owning repositories/use cases | FAIL finite coverage must be rerun; formats differ |
| CLI parsing, diagnostics and exit status | Existing profile and CLI matrices | `profileparity`, `clierrorparity` | CLI domain delivery | FAIL current executable rerun needed |
| Terminal shutdown, protocols and saved-session guidance | Existing PTY differential | `sessionfooterparity` | `gateway/codex` | FAIL current executable rerun needed |
| Native TUI semantics | Bubble Tea owns corresponding Godex surfaces | Owning delivery tests, TUI guard | CLI domain delivery | FAIL incomplete interactive differential coverage |
| DeepSeek request/response/stream behavior | Existing finite matrix; schema-alias gap retained from 0.438.4 | `tools/differential` | `gateway/deepseek` | FAIL alias gap and current full-matrix rerun |
| Gemini request/response/stream behavior | Existing finite matrix | `tools/differential` | `gateway/gemini` | FAIL current full-matrix rerun needed |
| Other providers, catalogs, quota and terminal migrations | Existing implementations; changed source needs comparison | Tagged migration baselines and owning tests | Owning gateways/use cases/delivery | FAIL migration inventory not fully qualified |
| Authentication and refresh against real services | Official clients retain authentication ownership | Synthetic auth and lifecycle fixtures | Auth/profile domains, native gateways | BLOCKED live OAuth unavailable; synthetic tests remain runnable |
| Native macOS/Windows runtime behavior | Six cross-build targets and native runners exist | `check-cross-build.sh`, native parity runner | Platform-specific adapters | BLOCKED native execution unavailable here; cross-build is weaker evidence |

## Current checkpoint

- Worktree: `$HOME/.cache/godex-parity-04390-20261023`
- Branch: `parity/04390-20261023`, based on `origin/main` at `cea78671d`.
- Original checkout, its local-only quota commit and untracked files preserved.
- Active oracle identities migrated together; historical parity records kept.
- PASS: `rtk proxy go test -json ./internal/gateway/codex ./internal/delivery/cli/runtime ./internal/usecase/routing ./internal/delivery/http/proxy`
- PASS: `rtk proxy go test -json ./tools/...`
- PASS: `bash -n scripts/verify-release-parity.sh`; workspace oracle Python
  syntax and `git diff --check`.
- Next: commit the pin migration, build the clean candidate, execute 0.439.0
  differentials, and close verified production gaps. No current full-gate or
  native foreign-platform PASS is claimed.
- No release version, tag or package publication is authorized by this work.
