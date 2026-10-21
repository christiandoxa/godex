# Differential harness

Build and run from a clean, committed Godex worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
/tmp/differential \
  --prodex /absolute/path/to/prodex-0.438.2 \
  --godex /tmp/godex-candidate \
  --prodex-source /absolute/path/to/prodex-0.438.2-source \
  --godex-source "$PWD" \
  --godex-commit "$(git rev-parse HEAD)"
```

### Pinned Gemini API-key Super executable differential

Use `--gemini` with the same `--prodex`, `--godex`,
`--prodex-source`, `--godex-source`, and `--godex-commit` flags
to run thirteen **additional** cases against the published Prodex 0.438.2
binary, not against a guessed Gemini protocol. The reference was
observed to send API-key Super requests as OpenAI-compatible
`POST /v1beta/openai/chat/completions`, yet it decodes upstream
**Gemini native GenerateContent** envelopes for the client response.

The first six cases require an exact single upstream request with
synthetic credential authentication and translated model/message fields:
(1) a successful native GenerateContent result,
(2) an unpaired tool call that must not leak into upstream context, and
(3) an HTTP-200 Chat Completion-shaped response that must become
`gemini_empty_response` with Prodex's exact failed envelope, rather
than being treated as successful content, and
(4) a **streaming** Chat-compatible request to a synthetic upstream
which emits native GenerateContent SSE chunks. The tagged Prodex must
send `stream_options.include_usage=true`, emit exactly
`response.created` then `response.completed` with empty output,
and never surface native Gemini text as Chat Completion delta events, and
(5) a genuine Chat Completions SSE stream with two text chunks and a
finish/usage chunk. It must produce the canonical six-event lifecycle:
`response.created`, `response.output_item.added`, two
`response.output_text.delta` frames, `response.output_item.done`,
and `response.completed`. The item and response IDs must remain
consistent across frames, with the exact terminal model, finish reason,
assembled content, and usage totals observed in tagged Prodex, and
(6) a complete multi-turn tool history with two assistant tool calls,
two results delivered out of order, an intervening user message, and a
third unanswered tool call. The outbound Chat Completions request must
pair results immediately after their matching calls in original call
order, drop the unanswered call, and preserve both user messages.
Oracle mutation controls reject reordered or missing results, duplicated
tool calls, changed IDs, altered tool output, and lost user prose.

Two additional fresh-request **dual-key** scenarios qualify tagged 0.438.2
Gemini quota retry and rotation:
(7) a plaintext HTTP 429 must pass through to the client **verbatim**
after one attempt on the first key, without rotating; the synthetic
child exits 2, and
(8) a structured JSON `rate_limit_exceeded` 429 must try models
`gemini-3.1-pro-preview`, `gemini-3.8-flash`,
`gemini-3.7-flash`, `gemini-3.6-flash`, and
`gemini-2.5-pro` **in order on the first key**, before retrying
the initial model on the second key, producing one successful native
Gemini Responses result with exit status 0. The mock records credential
**slots**, not key bytes. Both products are checked for identical
request payloads and attempt count/order, exact child exit codes,
the presence and absence of a timestamp, UUIDv7
`resp_deepseek_` response identity, finish metadata and usage.
Negative controls reject changed credential slots, fallback model order,
status/exit codes, truncated attempts, malformed response IDs and
invented timestamps.

Source tag, executable hash, clean worktrees and embedded Godex VCS
metadata are verified **before** either executable runs. The comparator
checks provider route, authorization validity, request body, HTTP
status, child exit status and full JSON response semantics. Randomized
UUIDv7 suffixes are normalized only **after** checking the exact
response ID namespace/version and whether `created_at` must be absent. The streaming oracle independently
verifies event labels, ordering, sequence numbers, matching per-stream
response and item IDs/timestamps, and absence of fabricated native
output or missing Chat SSE text, item lifecycle, finish metadata and
token usage.
Mutation sentinels must detect
altered route, authorization, model, status, content, quota usage and
metadata. These checks use a loopback fake upstream with no live
credential. They are automatically required by
`scripts/verify-release-parity.sh`.

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
The resulting PASS is limited to the eighteen named DeepSeek synthetic scenarios and is
not equivalent to a global provider, live-TUI or transport parity certificate.

Use `--scenario success|tool-call|deepseek-user-id|sse-stream|sse-rate-limit|retry|deepseek-sse-terminal|single-key-401|single-key-403|single-key-429|single-key-500|single-key-503|key-rotation-429|key-rotation-restart|cancel|restart|recover-after-429|recover-after-503` to rerun one case while
investigating a mismatch; the default is `--scenario all`.

The 0.438.4 `deepseek-sse-error-presence`, `deepseek-sse-error-i64` and
`deepseek-sse-error-float` scenarios verify embedded-error primary-field
presence and signed numeric kinds. Each must produce one failure frame
with the expected code/message, HTTP 200, one primary-key attempt and no
rotation despite a configured second key. They run in `--scenario all`
and can also be selected individually.

The `deepseek-user-id` fixture sends the Responses `user` alias through the
real Codex shim and requires the upstream DeepSeek request to contain the
canonical `user_id` field from the 0.438.2 provider conformance fixture.


Run only from a committed, clean candidate checkout; build the Godex binary
from that exact source. The supplied --godex-commit pin must match HEAD **and the binary's embedded
Go VCS build revision**. The harness refuses binaries built from dirty or
uncommitted source.
The canonical reference is Prodex tag 0.438.2, not an arbitrary binary release.

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

Do not interpret this eighteen-scenario DeepSeek harness as proof of full 1:1 parity;
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
profile retry persistence. Raw inventories remain in the report, while the
semantic state gate stays fail-closed on unknown or active state; this narrow
fixture alone does not establish full persistence parity.


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


### Prodex 0.437.8 DeepSeek embedded-error terminal scenario

A single Codex client request starts a DeepSeek translation stream with
two synthetic credentials configured. The primary upstream responds with
HTTP 200 and an embedded Chat Completions error carrying a structured
Retry-After value. The canonical Prodex 0.437.8 translator has already
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


### 0.437.8 precommit delta

The exact-tagged 0.437.8 reference adds support for original
Responses stream:true requests whose upstream returns HTTP 200 SSE
without Content-Type, plus bounded retry of precommit overload on a
turn-state-only pinned owner. Those OpenAI Responses contracts are
verified by source-audited routing and HTTP wire regressions.
The DeepSeek binary harness does not claim to exercise every
OpenAI-compatible Responses or WebSocket provider policy.
The release gate remains fail-closed on durable file discrepancies.

## Prodex 0.438.2 streamed tool-argument oracle

Four additional credential-free differential scenarios exercise actual
DeepSeek Chat Completions SSE `tool_calls` envelopes and their downstream
Responses translations, rather than calling only an internal JSON validator:

- `--scenario sse-tool-surrogate` rejects lone escaped high-surrogate Unicode.
- `--scenario sse-tool-depth` rejects JSON nesting above 127 containers.
- `--scenario sse-tool-overflow` rejects an overflowing JSON exponent.
- `--scenario sse-tool-paired` accepts a valid escaped surrogate pair.

Each requires exact event types, sequence numbers, tool-call identity, no
premature `arguments` in `response.output_item.added`, canonical error text,
and terminal completion/failure behavior. Mutation tests reject altered SSE
fields even when both clients return HTTP 200; each execution independently
checks the official pinned executable, source tag and clean candidate VCS
metadata. These scenarios are included in `--scenario all`.

## Gemini wrapped native response oracle (0.438.2)

The active 0.438.4 oracle also checks original top-level `id` type presence.
When it is a string (including empty), the API-key bridge retains the native
`responseId`/trace identity produced by the Gemini transform. Otherwise it
generates the `resp_deepseek_` UUIDv7 used by the wrapped fixtures below.
The original `id` is never borrowed as a native identity alias. Canonical
wire cases compare every response byte except the validated random UUID;
non-object envelope cases require the exact failed empty-response contract.

The full `--gemini` differential includes two additional buffered native
GenerateContent cases with a nested `response` envelope: with a string
`traceId` and with no `traceId`. Both independently require visible text,
model, usage, no arbitrary outer fields, and a generated API-key Responses
identity in the `resp_deepseek_` UUIDv7 namespace. Direct native Gemini
transport is not changed by this identity contract. The source-pinned binary
oracle now exercises thirteen Gemini cases, including all earlier eight fixtures, two invalid function-tool request rejections and one null function-call response.

### Bounded full DeepSeek oracle

`sse-tool-id-empty` and `sse-tool-id-padded` require present string tool IDs
to survive unchanged in every added/delta/done/final event. The positive
`sse-tool-id-missing` control requires one stable generated UUIDv7 in the
`call_deepseek_` namespace across the same full stream. Only that validated
generated value is normalized. Renamed/missing IDs, mismatched event identity,
changed sequence numbers and tool types are rejected even if both products
return the same wrong trace.

`sse-tool-signature` requires a thought signature with surrounding tab, space
and newline bytes to survive unchanged in both completed tool output and the
final response. It compares the full five-event stream and rejects trimmed
signatures, renamed fields, changed sequence numbers or tool identities.

`sse-payload-bom` and `sse-payload-bom-repeat` exercise one or two BOMs at
the start of a provider data field. The provider JSON reader preserves those
bytes, so the exact output is one `provider_stream_error` with `incomplete
JSON segment`, without response creation, text or success. The Gemini matrix
includes corresponding `stream-chat-bom` and `stream-chat-bom-repeat` cases.
Only validated generated UUIDv7 and current timestamps vary; all remaining
failure bytes must match. Mutations reject altered errors, sequence numbers,
framing, duplicate failures and borrowed upstream identities.

`sse-apply-patch-canonical` compares all four Responses SSE events for an
apply-patch tool whose arguments use the same nested u64::MAX, large integer,
decimal/exponent, negative-zero and null fixture as tool-search. The canonical
JSON input string must appear unchanged in both the completed item and final
response. Independent mutation controls reject rounded numbers, altered empty
added input, changed tool type and incorrect event sequence numbers.

`sse-tool-order-finish` and `sse-tool-order-done` send sixteen invalid tools
in descending index order followed by one valid tool at index zero. Both
require the complete 36-event byte trace and the error at index three, the
lowest invalid index, whether validation begins at finish or DONE. No tool
completion or later response completion may follow. Mutation controls reject
matching wrong error indices, altered sequence numbers, tool indices, terminal
events and error codes. Both scenarios are included in `--scenario all`.

`tool-search-canonical` and `sse-tool-search-canonical` compare complete
buffered and three-event SSE bytes for structured tool-search arguments.
The fixture includes nested u64::MAX, integers above 2^53, exponent input,
negative zero and null. Both original canonical number spellings and tool
identity must match; the oracle performs no float decoding or normalization.
Mutation controls reject rounding, changed integer kinds, execution, tool
type and final output even when both products agree on the same wrong value.

`sse-tool-finish-error` and `sse-tool-empty-finish-error` send invalid tool
arguments with a present finish string, then a different upstream error before
DONE. Both require the original exact invalid-tool failure trace; the later
error cannot take ownership. Empty finish strings trigger the same validation.
The existing malformed-tool and valid-tool stream contracts remain in the
matrix. Local event-boundary tests separately prove valid tools emit their
done item at finish and retain that tool in the final response after usage.

`sse-metadata-u64` requires exact integers above 2^53 in logprobs and
u64::MAX in annotations. The provider metadata object must match the tagged
bytes before the existing full SSE contract runs; rounded/dropped counters,
altered text and extra events are negative controls. The scenario is part of
the complete DeepSeek matrix.

The wire-bound cases are `sse-large-packet` (an admissible 2 MiB packet),
`sse-line-limit`, `sse-event-limit` and `sse-comment-utf8`. The first requires
the normal exact five-event trace. Each failure case independently requires
one failed event with the exact tagged error and no text/completion; UUIDv7
and current epoch are validated before normalization. All use one synthetic
authenticated request with normal child exit and unchanged durable-state
checks. Wrong error text/code, extra output and shifted sequence are negative
controls. These join `--scenario all`; the shared decoder also requires the
Gemini matrix as a separate regression.

`sse-json-fragments` splits a JSON string across SSE events, and
`sse-data-lines` places separate JSON values on two data lines of one event.
Both must produce the existing exact five-event text/usage trace, with no
partial text or alternate terminal event. They join the complete DeepSeek
matrix; shared framing changes also require the Gemini matrix regression.

`sse-terminal-close` sends the canonical text/usage stream, flushes DONE,
and leaves the upstream HTTP response open until the product closes it. Both
clients must return the complete five-event response before the existing
eight-second shim deadline; waiting for upstream EOF cannot pass. No delay or
sleep releases the mock. Its request context owns handler cancellation.

`deepseek-empty-identity` and `deepseek-sse-empty-identity` require present
empty response ID/model strings to remain empty. The stream's later packets
offer a different ID, which must not replace the already supplied empty ID.
The oracle checks exact identity field presence and every event's ownership
before applying the existing full response/SSE fixture contract. Missing,
null, fallback and late replacement identities are negative controls.

The `deepseek-usage-null` and `deepseek-sse-usage-null` scenarios require an
explicit zero-counter object for present null usage. The streaming mock first
sends nonzero counters, then null, proving the last present snapshot replaces
the previous one. Exact zero integer fields are independently checked before
the existing full response/SSE oracle runs. Missing, null, stale and decimal
zero counters are negative controls.

The 0.438.4 matrix includes `deepseek-usage-u64` and
`deepseek-sse-usage-u64`. Their synthetic upstream usage covers integers above
2^53 and i64, present u64::MAX cache/reasoning counters, and a missing total
that must saturate at u64::MAX. The SSE fixture also uses a u64::MAX creation
epoch. Independent `UseNumber` assertions require exact integer spellings
before the existing ordinary-response/complete-stream contracts run. A float
rounding match cannot satisfy these assertions. Mutation tests reject rounded
input/output, missing cache details, changed reasoning, total and epochs.

`--scenario all --scenario-workers 3` runs every scenario inside a separate
loopback mock and private HOME, with up to three scenario pairs in flight.
Product comparisons remain sequential within each scenario (Prodex then
Godex), and the report restores the canonical declaration order. Any worker
failure fails the entire run; no partial report is accepted. The default
`--scenario all` uses one worker and preserves sequential execution. Source,
SHA-256, Go build VCS revision, synthetic credentials, error envelopes,
headers, durable state, and negative controls are checked in both modes.
Parallel workers are allowed only for the complete DeepSeek scenario set.

`./scripts/verify-release-parity.sh` remains the full release gate. When an
external shell has a short synchronous execution deadline, execute each
explicit phase on the **same clean Git commit**: `--phase=preflight`,
`--phase=deepseek`, `--phase=gemini`, and `--phase=websocket`. Each phase
independently clones/verifies the exact tagged Prodex source and public
release SHA-256 plus manifest, and builds/validates the Godex candidate.
A successful subset is not a complete gate. Never skip an unverified phase
or treat partial-phase PASS as general production readiness.

### Tagged 0.438.2 Gemini function-tool rejection parity

Two additional **source/binary-pinned** cases, `invalid-wrapped-tool-parameters`
and `invalid-flat-tool-parameters`, assert that malformed API-key Gemini
function declarations are rejected *before* the provider sees a request.
The exact official Prodex 0.438.2 and clean Godex binaries must both return
HTTP **400** and child exit **2**, with the **identical** structured JSON
`error.code=invalid_request_error` and an indexed diagnostic at
`tools[0].function.parameters`, including the distinction between wrong
object type and missing required field. The mock must record **zero upstream
POSTs**; local validation is never permission for another credential,
model or HTTP transport retry. Any difference in status, request counts,
error code, indexed field, message bytes or child exit fails the comparison.
Focused Go regressions check that valid flat/wrapped tools still reach the
provider and that malformed requests do not become deferred Router retries
or plaintext HTTP errors. Existing successful Gemini cases and mutation
negative controls remain mandatory. These tests use synthetic credentials
and never make real Gemini model requests.

### Public Gemini Chat-compatible `functionCall:null` response (0.438.2)

`buffered-null-tool-text` is a source/SHA-pinned two-executable test with a
synthetic Gemini native GenerateContent payload: text, `functionCall:null`,
then text. The public Prodex **API-key Chat-compatible** adapter keeps the
adjacent text in one assistant message, emits a second `function_call` with
`name=tool_call`, `call_id=call_1` and `arguments={}`, and preserves native
usage/metadata in a client-visible Responses JSON. Godex initially dropped
the null tool, then an incorrect internal-buffered suppression assumption
hid the text and used a generated fallback ID. The public contract was
established against the actual tagged Prodex executable, not conflated with
its separate provider-core buffered normalizer (which suppresses text).

The entire **13-case** Gemini differential now compares the exact two output
items and their ordering, including text, call name/ID, arguments, usage,
metadata, HTTP status, model, and a single upstream POST. Only the
nondeterministic client Responses UUID and clock fields are normalized, after
verifying the required ID namespace. Mutation negative controls reject
missing call, leaked/modified text, wrong fallback ID, wrong arguments,
extra messages and missing/incorrect upstream dispatch. A separate Go test
keeps direct native/OAuth GenerateContent semantics unchanged.
