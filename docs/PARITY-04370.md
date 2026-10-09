# Prodex 0.437.0 behavioral parity checkpoint

Canonical reference: Prodex release tag **0.437.0**, commit
**b70f7429fb163760e3cd35a6564a0e79f9281a49**.
The official x86-64 Linux release executable is pinned by SHA-256
**0082ed1348183cc53b0dc4ad44d9f6a5e009d3d3dcc8bbf372bc2eec60447d76**.
The source and binary pins replace 0.436.1 in the **active** differential
harness and the release-blocking oracle script. Historical 0.436.1
checkpoint evidence remains available in PARITY-04361.md.

## 0.436.1 to 0.437.0 delta

The source delta spans 199 tracked files. Most changes consolidate policy
into Mojo, but this checkpoint identifies two new externally relevant
contracts in the tagged release notes:

1. **Structured streamed retry advice.** A precommit SSE response.failed
   or WebSocket error event with nested Retry-After: 5 must not use the
   shorter 1-second delay embedded in the message. Nested error headers
   precede outer headers; duplicate case-variant names have insertion
   precedence, invalid values cannot replace valid ones, and explicit
   zero, HTTP dates, and the 300-second local delay cap are preserved.
   Quota and completed responses may not be reclassified as retryable.
   Godex implements the source-audited policy at the routing use-case
   boundary, using bounded WebSocket precommit JSON evidence. A postcommit
   frame may never be replayed.
2. **Codex 0.162 qualification.** Prodex qualifies upstream rust-v0.162.0
   at commit c1382380de69521303b416720a52f42d51af6248, but does not
   make that exact version mandatory. Existing capability minimum remains
   0.153.2. The local Codex installation was 0.161.0 when inspected;
   it was not modified. A separate official 0.162.0 CLI and app-server
   were downloaded to an isolated temporary directory, validated
   against GitHub's release asset SHA-256 digests and version-checked.
   The actual Godex thread-index reconciliation protocol completed
   initialize and active/archived thread/list RPCs against the official
   0.162.0 app-server with an empty temporary HOME and CODEX_HOME.
   No authenticated model-turn or unrestricted capability claim is made.

The release additionally qualifies Ponytail 5.1.0 and moves several
selection, provider, identity and rendering implementations into Mojo.
The Go project does not adopt internal Rust/Mojo architectural layers as
a substitute for independent observable behavioral verification.

## Verified and still blocked

- Structured SSE retry precedence, WebSocket precommit provenance,
  cancellation/no-replay fences, HTTP-date, invalid headers, zero delays,
  cap, and stream size limits have focused synthetic negative controls.
- A standalone differential scenario exercises a real embedded DeepSeek
  provider SSE error with two synthetic keys. The tagged Prodex translator
  has already selected its stream writer, so it forwards the terminal
  response.failed event to Codex **without** replaying or rotating to the
  secondary key. A fixture that supplied an already translated Responses
  event to the Chat Completions upstream was rejected as invalid evidence
  and replaced with an actual DeepSeek error envelope. Negative controls
  require exactly one upstream attempt, one failed SSE event, original
  code/message, source-generated UUIDv7, and bounded timestamp.
- Precommit SSE and WebSocket **OpenAI** streamed retry advice is a separate
  contract: nested Retry-After 5 overrides the shorter message, with
  explicit-zero/date/invalid-header precedence. It must not be conflated
  with an already committed DeepSeek translator stream.
- The existing DeepSeek buffered responses, tool calls, streaming, auth
  failures, 429/503, restart and retry tests remain in the reference
  harness and must be rerun against **0.437.0**, not treated as inherited
  PASS without executing them.
- A real synthetic run exposed a material difference in the durable
  state: the previous Godex candidate persisted a hashed previous-response
  affinity binding for a launch-local DeepSeek key, while the tagged
  Prodex run left no equivalent durable conversation binding. Godex now
  keeps verified affinity and WebSocket turn-state for ephemeral API-key
  accounts in memory only; managed profiles retain their durable bindings
  and sidecars. Regression tests verify both sides of that boundary.
  The differential state auditor rejects any synthetic previous-response
  binding remaining in routing.json, including unknown or malformed
  state fields. The runtime also filters legacy synthetic-account bindings
  written by older Godex versions **without destructive migration**; it
  leaves managed-account ownership records intact and maintains volatile
  affinity for requests within the current process. Independent tests
  verify the initial cache, restart, and non-destructive managed-profile
  behavior. This does not change the separate managed-profile
  continuation recovery guarantees.
- A second, independent **managed-profile lifecycle differential** was
  added to the release gate. It uses the exact SHA-256-pinned Prodex
  0.437.0 executable and a clean-commit Godex binary. Nineteen separate
  CLI stages cover empty listing, create alpha/beta, default activation,
  duplicate rejection, active switch, current-profile retrieval, rejected
  unknown selection, removal without deleting the home, restart/current
  persistence, deletion with home removal, and rejected unknown deletion.
  Because Rust stores profiles in a keyed state.json map and Godex stores
  them in a profiles.json array, it compares exact **domain projections**
  rather than equal file bytes. The gate independently verifies versions,
  active names, providers, managed home paths, private file modes, no
  unexpected auth.json, no profile-directory pollution, and matching
  exit codes. A separate external CODEX_HOME fixture checks user-owned
  directory registration, non-destructive removal and refusal to delete
  that directory even when --delete-home is supplied. Negative tests reject
  corrupted/unknown state fields, stale
  last-run records, nonempty session/response bindings, mismatched managed
  homes, stale or dirty Godex binaries, and a noncanonical Prodex digest.
  Each operation is a new process with an isolated HOME and no credentials.
- The raw durable-file layout comparison is still fail-closed. The new
  managed-profile test proves one meaningful cross-process persistence
  domain but does **not** prove arbitrary multi-profile recovery,
  background queues, goals, TUI, app-server, WebSocket or every provider.
  Those surfaces require additional independent runtime evidence.

## Official Codex 0.162.0 qualification fixture

SHA-256-verified Codex assets from OpenAI release rust-v0.162.0
were checked without installing or replacing the user's Codex:

- codex-x86_64-unknown-linux-musl.zst:
  058ae1d3b280a6800fb2e625cf93a671e4700df25053acb7cea272c0aff012a8
- codex-app-server-x86_64-unknown-linux-musl.zst:
  2e38fb0a4c6246f1c514399c873ade46338c0dfc1ff52a1801fed12fd6e6c791

The opt-in test
TestProdex04370OfficialCodex0162AppServerThreadIndexBoundary
uses GODEX_TEST_CODEX_0162_APP_SERVER_BIN to exercise the real
thread-index RPC protocol against the official binary. It issues no
model request, accesses no user's home, and changes no installed binary.

**Full parity and Godex v0.2.1 release are NOT certified until all
material behavioral surfaces and official release gates are verified.**
A green Go test suite alone is not release authorization.


## Provider gateway startup selection (open parity boundary)

A credential-free local test found another semantic startup mismatch:
Prodex 0.437.0 refuses to start a DeepSeek gateway with a raw --api-key
when no compatible/active managed profile is registered; Godex originally
opened a listener. The tagged Prodex runtime selects a compatible
profile before constructing its provider gateway. Godex now refuses the
profileless case, and positive tests show a resolved compatible profile
still starts and closes the gateway. The canonical refusal is checked
on both executable CLIs in the 20-stage profile lifecycle differential.

With a *registered but unauthenticated* OpenAI profile, a separate manual
loopback fixture exposed additional differences in HTTP gateway behavior
(Prodex error responses versus Godex's successful DeepSeek proxy).
That fixture is not certified parity and needs source-based analysis
before deciding whether it is a feature extension or correctness gap.
No real provider credentials or user profiles were used.
