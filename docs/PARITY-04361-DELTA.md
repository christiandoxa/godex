# Prodex 0.436.0 to 0.436.1 delta

Reference source: `/tmp/prodex-04361-reference-20261009-01`, commit
`4c61dc0a84a6cf4852feb08e3a8406c1846bb5ad`. The parent release is tag
`0.436.0`.

## Canonical observable delta

The release notes identify two runtime surfaces. A Codex client that falls back
from WebSocket to HTTP keeps that transport choice for its session, and the
HTTP SSE path applies the same precommit recovery policy. The capacity audit
then identifies the concrete failure that prompted the 0.436.1 fix: a
Responses SSE prefix remains replayable until a commit-ready event, an explicit
retryable error, EOF, the configured stream-idle deadline, or the bounded
prefetch buffer limit. The old short lookahead timeout is only a poll interval;
it cannot turn `Hold` into `Commit`.

The failed attempt also releases profile admission before its retained error
payload enters the retry loop. Successful streams retain their lease, and a
previous-response recovery keeps its intentional ownership. Visible output or
tool events still make replay unsafe.

Reference evidence:

- `docs/release-notes/0.436.1.md` describes delayed overload/rate-limit/quota
  recovery, bounded wait/bytes, and lease release.
- `migration/04361-capacity-recovery-audit.md` records the two independent
  reproductions and the no-replay rules.
- `crates/prodex-app/src/runtime_proxy/prefetch/lookahead.rs` changed the
  timeout path from `Hold -> Commit` to a stream-idle and byte-bound decision.
- `crates/prodex-app/src/runtime_proxy/response_forwarding.rs` drops the
  unsuccessful attempt's in-flight guard before retaining its payload.
- `crates/prodex-app/src/runtime_proxy/prefetch/capacity_tests.rs` is the
  canonical fixture set: delayed error, split metadata/error, metadata above
  8 KiB, visible output, deadline/bytes, and metadata-only EOF.

The remaining 0.436.1 diff contains Mojo ownership extraction for
session/thread-index, login, provider, quota-watch, sub-agent, and RTK policy
code; optional-tool freshness metadata; and a hard-affinity fixture delay. Those
surfaces need their own domain comparisons; they add no additional capacity
recovery rule to this delta.

## Godex contract mapping

| Reference rule | Godex owner | Executable evidence |
| --- | --- | --- |
| WebSocket-to-HTTP fallback keeps a session transport choice | shared `session_id` affinity in `internal/usecase/routing` | no dedicated client transport-state contract; gap remains explicit |
| Held metadata waits for a retryable signal | `internal/usecase/routing/stream.go` and `stream_read_ahead.go` | `TestProdex04361DelayedCapacityAfterLargeMetadataStaysPrecommit` |
| Failed SSE attempt releases profile admission | `internal/usecase/routing/classify.go` and `profile_inflight.go` | same test checks the active permit is zero |
| Output commits and later errors are never replayed | `internal/usecase/routing/stream_read_ahead.go` | `TestProdex04361VisibleOutputCommitsBeforeLaterCapacityFailure` |
| Metadata-only EOF and bounded precommit are terminal | routing precommit policy | `TestProdex04361SSEPrecommitRejectsIncompleteEOFAndByteLimit` |

The new tests close the missing large-prefix and delayed-fragment coverage. They
use synthetic payloads only; no credentials, provider account, or live model
call is involved.

## Gaps kept explicit

Godex does not invent Prodex's process-lifetime state-save, continuation-journal,
or probe-refresh workers. It also does not persist Codex's client-level
`disable_websockets` transport fallback bit; session affinity is shared across
HTTP and WebSocket, but transport preference resets with the client process.
Full cross-binary differential execution and durable state-layout comparison
remain separate follow-up work.
