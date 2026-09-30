# Runtime rotation and affinity

This document describes the managed runtime behind Godex. The public README
covers only installation and daily use.

## How rotation works

Godex treats every forwarded request as a small state machine:

~~~text
uncommitted  ->  committed  ->  completed
                     \
                      ->  failed-after-commit
~~~

Only an **uncommitted** request may move to another account.

### Selection and affinity

Before starting Codex, Godex previews the account store's deterministic
round-robin order and probes each enabled candidate once. Confirmed exhausted
accounts are excluded from fresh-work eligibility until their observed reset
deadline, or for one minute when the deadline is unknown. Probe
failures are treated as unknown rather than exhausted. Godex selects the first
known-ready candidate; if none are known ready, it falls back to the first
unknown candidate. An explicit selector is never silently replaced by another
account. Only the final chosen account advances the persisted rotation cursor
and `LastUsedAt`.

Once Codex is running:

1. Godex extracts only the opaque continuity identifiers needed by Codex:
   previous_response_id, x-codex-turn-state, and session/conversation IDs.
2. A known continuation is sent to its original account.
3. A fresh request filters disabled, launch-ineligible, and quarantined accounts.
4. The remaining accounts are selected by deterministic round-robin order.
5. Attempts are capped at the number of eligible accounts captured for that
   request; an account is not tried twice in one cycle.

If a continuation's owner is disabled, unavailable, or no longer registered,
Godex returns a continuity-preserving error. It does not silently move that
conversation to another account. Quota probing is launch-scoped. The snapshot carries a temporary eligibility
deadline separately from account enablement. The next fresh request after that
deadline may attempt the account upstream; a continuing quota failure is handled
by the normal precommit retry/quarantine policy. There are no background probes
or per-chunk checks, and hard continuation affinity always wins.

## Failure policy

| Upstream result | Before commitment | After commitment |
| --- | --- | --- |
| Connection, DNS, or TLS failure | Rotate within the bounded attempt cycle. | Keep the current request; never replay. |
| 500, 502, 503, or 504 | Rotate. | Keep the current request; never replay. |
| 429 or structured quota exhaustion | Quarantine using Retry-After when valid, then rotate. | Keep the current request; never replay. |
| 401 | Reload auth.json once, retry the same account once, then rotate if needed. | Keep the current request; never replay. |
| Other 4xx | Pass through without rotation. | Pass through without rotation. |
| Downstream cancellation | Stop immediately and cancel upstream work. | Stop immediately. |

An upstream body read failure during bounded inspection returns a local 502
before commitment. It is not silently converted to a successful truncated body
or replayed. Size-limit exhaustion remains distinct from a read failure; large
responses continue forwarding without being buffered in full.

Valid Retry-After values influence quarantine duration. Quarantine entries are
bounded and expire. Quarantine is in memory. Continuity bindings are backed by a bounded, versioned
hashed ownership snapshot, so restart and cache expiry recover the owner.
Unknown opaque continuations fail instead of becoming fresh requests. Stable
thread/session bindings are protected from expiry and eviction.

## Commitment and streaming

For a normal response, commitment occurs immediately before downstream status
and headers are written. For a streaming response, commitment occurs no later
than forwarding response headers or the first response bytes. After commitment,
Godex may propagate the failure but must not replay the request on another
account.

Fresh HTTP 200 SSE responses defer `response.created` and `response.in_progress`
metadata within a 64 KiB startup buffer. A structured quota failure before output
quarantines the account and may rotate within the original bounded attempt cycle.
An output event, unknown event, or the inspection ceiling ends this guard and
forwards the original stream. Bound continuations never rotate. If every attempt
fails, the last upstream status, headers, and SSE bytes are preserved; a failed
startup does not claim new conversation ownership.

Streams use bounded buffers, flush promptly when the downstream supports
flushing, and are never buffered in full. A stream that emits one event and
then fails is never replayed on a second account. New continuity bindings are persisted before their output bytes. Known bindings
are not written again per chunk; streams remain bounded.

SSE ownership is decoded incrementally throughout forwarding, including multiline
data and CR/LF framing. The parser retains only incomplete event data, bounded to
64 KiB per event. Oversized events are forwarded unchanged but skipped for
metadata; parsing resumes at the next event. Later valid ownership metadata still
survives restart. Compressed bytes remain untouched and are not inspected as SSE.

## Forwarding

Godex preserves upstream status, end-to-end headers, repeated header values,
trailers, body bytes, and stream framing. Transport hop-by-hop headers remain
managed by net/http. The selected profile's authentication is used for the
upstream request; Godex supplies the selected ChatGPT account routing header and preserves other
end-to-end request and response metadata.

Request and classification buffers are bounded. Compressed response bytes are
not decoded and re-encoded merely for forwarding. Any continuity identifiers
needed for affinity are extracted without retaining prompts, response bodies,
or token content.
