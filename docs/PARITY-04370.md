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
   no Codex 0.162.0 real-app-server compatibility claim is made here.

The release additionally qualifies Ponytail 5.1.0 and moves several
selection, provider, identity and rendering implementations into Mojo.
The Go project does not adopt internal Rust/Mojo architectural layers as
a substitute for independent observable behavioral verification.

## Verified and still blocked

- Structured SSE retry precedence, WebSocket precommit provenance,
  cancellation/no-replay fences, HTTP-date, invalid headers, zero delays,
  cap, and stream size limits have focused synthetic negative controls.
- A standalone differential scenario exercises a real SSE failure from a
  rate-limited primary synthetic key and expects proxy-controlled rotation
  to a separate healthy secondary key.
- The existing DeepSeek buffered responses, tool calls, streaming, auth
  failures, 429/503, restart and retry tests remain in the reference
  harness and must be rerun against **0.437.0**, not treated as inherited
  PASS without executing them.
- The raw durable-file layout comparison is still fail-closed. Equivalent
  private Rust-vs-Go persistence layout, multi-profile recovery, queue
  lifecycle, TUI, app-server, WebSocket, and other provider surfaces
  require additional independent runtime evidence.

**Full parity and Godex v0.2.1 release are NOT certified until all
material behavioral surfaces and official release gates are verified.**
A green Go test suite alone is not release authorization.
