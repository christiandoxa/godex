# Differential harness

Build and run from a clean, committed Godex worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
/tmp/differential \
  --prodex /absolute/path/to/prodex-0.437.1 \
  --godex /tmp/godex-candidate \
  --prodex-source /absolute/path/to/prodex-0.437.1-source \
  --godex-source "$PWD" \
  --godex-commit "$(git rev-parse HEAD)"
```

The harness creates separate temporary homes, starts one loopback mock upstream,
and runs both products independently through eighteen bounded scenarios: a successful request; a
synthetic 429 followed by a retry owned by the **Codex shim**; a single-key
429, 500, or 503 that must remain terminal without **proxy** retry; cancellation while
upstream is delayed; two launches against the same home; and a terminal 429 followed by a
fresh process with the same home and a healthy provider, which must succeed. Each
run records bounded stdout/stderr, child response, upstream requests, ordered
upstream events, retry count, cancellation observation, exit status, and a
redacted durable-state snapshot. The comparison also has mutation sentinels
for every observed category. It requires the exact Prodex source commit and
version before running. In addition to the Godex source commit, it verifies the
candidate executable's embedded Go VCS revision and requires `vcs.modified=false`.
A stale binary, dirty build, or build without VCS metadata fails before execution.
Both reference and candidate source trees must also have no tracked modifications
or untracked files; a locally edited Prodex checkout cannot serve as the oracle.
The resulting PASS is limited to the seventeen named synthetic scenarios and is
not equivalent to a global provider, live-TUI or transport parity certificate.

Use `--scenario success|tool-call|deepseek-user-id|sse-stream|sse-rate-limit|retry|deepseek-sse-terminal|single-key-401|single-key-403|single-key-429|single-key-500|single-key-503|key-rotation-429|key-rotation-restart|cancel|restart|recover-after-429|recover-after-503` to rerun one case while
investigating a mismatch; the default is `--scenario all`.

The `deepseek-user-id` fixture sends the Responses `user` alias through the
real Codex shim and requires the upstream DeepSeek request to contain the
canonical `user_id` field from the 0.437.1 provider conformance fixture.


Run only from a committed, clean candidate checkout; build the Godex binary
from that exact source. The supplied --godex-commit pin must match HEAD **and the binary's embedded
Go VCS build revision**. The harness refuses binaries built from dirty or
uncommitted source.
The canonical reference is Prodex tag 0.437.1, not an arbitrary binary release.

**Fail-closed behavior:** the harness prints a structured JSON comparison and
returns a nonzero process exit for any mismatch, missing child/upstream evidence,
or a run that violates the scenario-specific expected outcome. The cancellation
scenario expects both interrupted children to exit 1 and requires observed
cancellation; ordinary success, shim-retry, and restart runs require exit 0, status
200 and a real response body. The single-key 429 case requires exit 2,
status 429, the original structured rate-limit error, and no proxy-owned
retry; all cases require the expected number of upstream requests.
The negative controls alone never imply parity. Current header, durable state,
stderr, and upstream metadata differences are intentionally still reported.

Do not interpret this seventeen-scenario harness as proof of full 1:1 parity;
additional transport, provider, and persistence contracts remain to verify.

### Strict fixture oracle and audited transport metadata

The mock verifies the exact synthetic Bearer credential **before redacting** its
bytes, and every upstream request must use POST /v1/chat/completions with the
expected model, user text, and non-streaming request. Every successful response
must contain the expected translated output and usage totals. Equal invalid
credentials or equal corrupted model output cannot pass.

HTTP header comparison remains exact except for three explicitly audited
transport fingerprints: the current Date value (both must parse as fresh HTTP
dates), Prodex's default Server: tiny-http (Rust) versus Godex's absent server
fingerprint, and the tagged HTTP client's duplicate identical Go-http-client/1.1
User-Agent versus one occurrence in Go. All other headers, including
Authorization evidence, Accept, Content-Type, Content-Length, Retry-After, and
X-Codex-*, retain strict comparison and negative-control tests. These exceptions
are **not** global exclusions or proof that opaque durable-state layouts are
equivalent.


The source-pinned DeepSeek runtime produces a known fixed stderr startup
banner on Prodex, whereas Godex emits none. The narrow comparator accepts
only that exact banner, and for cancellation it requires both programs to
report the same client timeout semantics through their validated respective
loopback routes. Any unexpected auth error, warning, quota diagnostic,
non-loopback destination, or changed cancellation reason fails the test.
This is a documented CLI-presentation difference, **not literal stderr
byte-for-byte parity**.


### Fixture durable-state integrity

Both products are checked independently for output artifacts that the
synthetic provider workflow must not create: foreign Codex sessions, polluted
Codex history, invalid JSON state, leaked synthetic credentials, and
inconsistent routing/last-good recovery snapshots. File permissions are
checked against world-writable artifacts and file size/type bounds. These
checks run on **actual temporary state files** after each product exits, with
negative controls for corruption and secret leakage.

The Rust and Go repositories use deliberately different private bookkeeping
layouts. Raw durable file inventories and digests remain in every report for
review, while the comparison uses the fixture's audited semantic state
contract: empty Codex history, no active routing/health/session state, valid
bounded artifacts, no synthetic secrets, and no unknown files. Coordination
locks, guards, housekeeping timestamps, runtime logs, and the child evidence
file are private implementation details and are excluded from that semantic
projection. Any unknown artifact, malformed state, non-empty routing or health
snapshot, polluted history/session, leaked credential, or divergent recovery
sidecar still fails closed before a comparison can pass. The restart and
provider-failure scenarios provide the behavioral proof that the audited empty
state can be loaded by a fresh process.


The recover-after-429 case runs **four real product processes** across two
isolated per-product homes: the first process receives a terminal structured
429, then a fresh process with the same persistent home must complete a
healthy request without stale retry/backoff state. The oracle checks actual
upstream attempt counts and both process generations, rather than simply
comparing equal file hashes. This tests one lifecycle path, not every
persistence or concurrent-recovery contract.


The added single-key-503 scenario checks the canonical terminal HTTP 503:
a sole provider must expose the outage unchanged rather than waiting for
an ineligible same-key recovery attempt and returning a local timeout.


The single-key-500 scenario applies the same terminal-response contract to a
generic provider HTTP 500 and verifies that Godex preserves its status and
structured error body without a same-key retry.


The recover-after-503 fixture uses the same isolated state home before and
after a real process restart. It ensures provider health changes caused by
an upstream outage do not block a later healthy request. Prodex writes a
runtime-scores sidecar after the failure; this is a real state transition,
so the differential audit must review its semantics rather than treating
it as harmless private-file noise.


The exact Prodex 0.436.1 runtime can create a versioned
runtime-scores.json health sidecar, associated locks, and numeric
runtime-scores.json.*.tmp artifacts following an upstream overload. The
temporary files are only accepted with the audited three-number naming
scheme, bounded file type and size, and synthetic-secret scanning.
These artifacts are not treated as proof of equal **health scoring**:
the fixture verifies the subsequent process's observable routing outcome,
while multi-profile health priority still needs its own oracle.


The synthetic fixture's sole-key Prodex health-score writer was observed to
leave a versioned envelope with generation >= 1 and an empty value map.
The state auditor now decodes committed and in-flight health-score sidecars
and fails on corrupt envelopes, unknown fields, or nonempty scores, which
would affect provider selection. Godex's route-memory list is separately
required to be empty. This proves both lack active health-selection state
for the single-key fixture, not that arbitrary multi-profile health ranking
policies agree.


The two-key DeepSeek fixture loads two **synthetic** credentials from
DEEPSEEK_API_KEYS instead of passing --api-key. The loopback provider
always rate-limits the primary credential and accepts the secondary one,
while the Codex shim makes a single client request. The verifier checks
the exact primary→secondary key-slot sequence, two upstream requests,
translated output, and no provider key persisted or printed. Retrying
the same credential does not satisfy the independent oracle.


### Closed: key-rotation persistence regression

The cross-process key-rotation-restart fixture uses two provider keys on
each of two launches against the same isolated home. The official Prodex
0.436.1 binary and candidate Godex binary now both attempt
primary→secondary on each process generation. Godex's launch-only API-key
pool uses in-memory health and retry state, while managed profiles retain
durable backoff. Independent tests reject a reattempt within the same
process, durable cooldown leakage across processes, or loss of managed
profile retry persistence. The raw durable file comparison remains
fail-closed; this narrow fix alone does not establish full persistence parity.


In two-key mode the diagnostic contract uses the exact tagged Prodex
startup message indicating rotation across two keys, rather than treating
a single-key banner as equivalent. Both modes reject any additional
unexpected auth/quota or process-error diagnostic.


### New wire-contract scenarios: tool calls, authorization and streaming

The exact Prodex 0.436.1 DeepSeek adapter is also tested with (1) a
Chat Completions tool call translated to Responses with exact call ID,
name and structured arguments, (2) one-key terminal HTTP 401 and 403
authentication/access errors with preserved status, code, and message,
and (3) an actual streaming Responses request with a synthetic upstream
SSE stream.

For SSE the oracle requires exactly five frames in this order:
response.created, response.output_item.added, response.output_text.delta,
response.output_item.done, and response.completed. The tagged reference
does not emit response.output_text.done. The streaming completed response
is sparse: created_at belongs to the event envelope, not the nested
response; object is also absent. DeepSeek event headers are projected to
the Codex SSE MIME type without forwarding arbitrary upstream headers.

Rust and Go assign different opaque numeric output-item IDs, but the
comparator accepts a difference **only** for a valid unsigned numeric
msg_deepseek_ suffix that is internally consistent across added/done
events. Every other event field, sequence, output text, usage value and
response metadata is compared to an independent exact fixture, with
negative controls for extra fields/events, wrong IDs and incorrect usage.
The raw persistent-state layouts remain a separate hard comparison.


The SSE rate-limit scenario supplies deterministic, future absolute
reset timestamps (in seconds and milliseconds) and synthetic consumption
fractions. Its Codex projection is verified for both requests and tokens
with strict field names, percentages, and absolute reset epochs. Unrelated
upstream headers, cookies, and framing must not leak through the translated
SSE response, and the reference event sequence remains the exact same five
frames. Negative controls reject malformed reset values, bogus percentages,
and unexpected headers.


### Prodex 0.437.1 DeepSeek embedded-error terminal scenario

A single Codex client request starts a DeepSeek translation stream with
two synthetic credentials configured. The primary upstream responds with
HTTP 200 and an embedded Chat Completions error carrying a structured
Retry-After value. The canonical Prodex 0.437.1 translator has already
committed its outgoing SSE writer: the error must be forwarded as a
single response.failed frame, **not** silently retried on the secondary
credential. This scenario detects incorrect retries, changed failure codes,
messages, malformed or stale timestamps, non-UUIDv7 response identities,
extra frames, and corrupted provider state.

The separate source-audited unit/integration tests for *precommit* SSE
and WebSocket errors verify nested structured Retry-After precedence
(5s versus a 1s message), date parsing, zero advice, caps and no-replay
after commit. A DeepSeek error embedded in an already committed translator
is not an eligible precommit retry and must remain client-visible.

Raw durable file layout remains an independent fail-closed release blocker.


### No synthetic-key conversation binding survives a process restart

The isolated DeepSeek API-key fixture has no managed account identity.
A provider request must not persist a previous-response affinity owner
for that launch-local key. The state auditor checks routing.json's exact
versioned shape and requires an empty bindings array; malformed, unknown,
or populated records fail even if both products' immediate responses
match. Managed-profile ownership and WebSocket turn-state still persist
on their separate tested paths. Private file layouts and other state
contracts remain fail-closed until independently verified.


### 0.437.1 precommit delta

The exact-tagged 0.437.1 reference adds support for original
Responses stream:true requests whose upstream returns HTTP 200 SSE
without Content-Type, plus bounded retry of precommit overload on a
turn-state-only pinned owner. Those OpenAI Responses contracts are
verified by source-audited routing and HTTP wire regressions.
The DeepSeek binary harness does not claim to exercise every
OpenAI-compatible Responses or WebSocket provider policy.
The release gate remains fail-closed on durable file discrepancies.
