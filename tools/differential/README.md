# Differential harness

The 0.438.4 Gemini `request-primitive-request-signature-*` cases compare exact
outgoing function/custom/MCP call signatures, five alias paths, nonempty-string
precedence, blank legacy preservation, duplicate-ID ownership and orphan
filtering. Full expected wire bytes remain independent of production code;
matching-corruption controls reject modified signature values/fields and IDs
even when both products change identically. Each case also checks response,
status, headers, child exit, stdout, authenticated attempts and safe durable
state against the pinned official release with synthetic credentials.
The `request-primitive-mcp-wire-*` cases reuse the source-equivalent function
alias fixtures for MCP identities and arguments, retaining complete independent
wire/response/state expectations and all matching-corruption checks.
The 27 `request-primitive-bridge-output-*` cases extend those independent
expectations to custom/MCP standalone output kinds: IDs, content/result
aliases, selected text parts, structured JSON and null/false/empty controls.
The 16 `request-primitive-custom-call-*` cases qualify custom-call input
wrapping, canonical nested JSON/Unicode, name-shadow fallbacks, identity aliases
and ignored fields against full independent wire/response/state expectations.
The 16 `request-primitive-mcp-name-*` cases cover empty-name MCP messages,
retained inline raw results, earlier-call adjacency, signature ownership,
content/result/error alias capture, absent-result controls and implicit-tool
metadata. Original legacy Chat conversion is checked independently.
The 18 `request-primitive-shell-call-*` cases compare historical shell-call
admission separately from root-first command rendering, selected null/mixed
values, ID fallback, embedded optional-field order, serde/Unicode values and
ignored signatures. These fixtures never execute the recorded commands.
The nine `request-primitive-ignored-messages-*` cases require Gemini Responses
to ignore an unrelated top-level Chat messages field while retaining actual
input, absent-input fallback, null-input rejection and earlier serde/candidate
validation. Full independent wire/response/state expectations remain required.
The Gemini `request-primitive-reasoning-*` cases cover strict shape and alias
precedence, ASCII effort mapping, thinking tool history with signatures, and
tool-choice omission. Real Chat SSE checks canonical framing/IDs/usage, exact
terminal omission metadata and UTF-8 content type, while buffered native and
Chat fallback responses retain the same request metadata. Mutation tests reject
metadata moved to another SSE event as well as changed fields/values.
The 31 `request-primitive-metadata-*` cases check response-only metadata,
client/cache validation, explicit/base/provider collision precedence, blank
keys, arbitrary retention strings, canonical numbers/Unicode, generated
omission-note precedence, failed Chat fallback and completed Chat SSE. They
retain independent full wire/response/status/header/stdout/exit/credential/state
checks. Matching-corruption controls reject lost/renamed metadata, JSON number
or escaping changes, recursive provider-field filling and metadata moved onto
another stream event. These cases add no DeepSeek scenarios.
The 30 `request-primitive-format-*` cases cover text/JSON/schema forms and
aliases, first-present precedence, invalid shapes, ignored text controls, JSON
guidance restricted to user/system messages, normalized developer content,
split text, degradation-note collisions, thinking and buffered/fallback/real
SSE responses. Expected complete wire bytes and metadata remain independent
of production code. Matching-corruption controls reject changed JSON guidance,
format controls and degradation notes in both products.
The 37 `request-primitive-choice-*` cases check tool-choice admission and
canonical projection, explicit null/scalars, named target binding, first-string
aliases, namespace separator/prefix rules, 128-byte names, thinking omission and
actual SSE. They use nonblank source-supported declaration descriptions to
isolate choice behavior. Matching-corruption checks change raw and decoded
requests together, including tool names/declarations and namespace delimiters.
The 29 `request-primitive-declaration-*` cases cover function projection,
blank descriptions, extra fields, typed strict/description admission, disabled
strict mode, name limits, stable duplicate/generic-schema selection, serde
number kinds, 128-function limits after deduplication, named choices and actual
SSE. Their original three wrong-admission expectations remain unqualified FAIL;
corrected unchanged-binary reruns prove those terminal 502s independently.
The 25 `request-primitive-mcp-declaration-*` cases cover MCP sets/single tools,
sorted allowed/config unions, defaults, alias barriers, segment normalization,
description preservation, schema aliases, named choices and real SSE. They also
distinguish early structured 400s for unsupported/invalid translated tools from
later terminal 502 shape failures. Eight original wrong-stage expectations stay
unqualified FAIL; corrected unchanged-binary reruns carry their actual scope.
The 20 `request-primitive-namespace-declaration-*` cases check namespace/MCP
delimiters, selected whitespace, child fields, duplicate functions, named
choices and actual SSE. They separately require early structured 400s and later
terminal 502s. Matching-corruption checks change raw/decoded names and delimiters
together, description whitespace, error indexes and exact error wording.
The 18 `request-primitive-custom-declaration-*` cases compare complete custom
and apply_patch descriptions/input schemas, selected whitespace, strict/format
admission, canonical embedded numeric/Unicode JSON, duplicate policy, named
choice and real SSE. Matching corruptions change raw and decoded requests
together, including grammar, schema limits, description separators and numeric
text inside the embedded format string.
The 46 `request-primitive-builtin-*` cases cover builtin aliases/key presence,
function precedence, later host field types, omitted tools, MCP-prefixed
functions, projected counts/choices and SSE. Per-tool recursive media controls
require path-key presence and preserve invalid earlier/later tool errors.
Matching mutations reject incorrect tool presence, names, types and choices.
The 27 `request-primitive-search-declaration-*` cases verify fixed search names,
raw schemas, strict/description admission, early translated errors, duplicate
selection, MCP-prefixed extra functions, named choices and SSE. Function/media/
builtin precedence distinguishes absent schemas from present null values.
The 30 `request-primitive-tool-kind-*` cases distinguish missing/non-string
types from unsupported strings. They cover early bridge rejection, nested
function validation, omitted declarations, MCP aliases, skipped host metadata,
late schema/strict/name rejection, media, choices, mixed tools and actual SSE.
The 16 `compact-*` cases compare the API-key Compact Chat path, selected model,
exact rewrite and raw summary formats, trace-gated envelopes, fallback headers,
original-request summaries, 4 MiB boundaries and bounded model-attempt order.
One ordinary Responses control verifies that its independent mapping survives.
Matching-corruption tests cover request/client bytes, status, headers, stdout,
exit, authenticated attempts and durable-state integrity.
The seven `compact-owner-*` cases seed an ordinary Responses request and issue
Compact in the same process. They cover cached users, projected Chat assistants,
current/old instructions, duplicate input, session-only and unknown-ID controls,
and a paired tool call with a numeric field above 2^53. Seed response, original
upstream bytes, attempt order and durable ownership remain independent checks;
matching numeric rounding and missing history are rejected even in both products.
Three `compact-owner-stream*` cases seed the exact independently validated Chat
SSE trace, use its actual generated public response ID for Compact, and compare
cached user/assistant text. EOF and unknown-ID controls require fresh Compact
requests. Seed event/framing/usage, raw Compact bytes, state and matching
corruption checks remain independent of production code.
Four `compact-owner-local-session-*` cases distinguish local invocation history
from gateway request namespaces: same, changed, omitted and newly supplied
session IDs use the same exact authenticated seed and public previous ID.
Only the local Super executable path is certified by those cases; separate
composition/runtime tests retain shared-gateway and credential isolation.
Five `deepseek-history-*` cases seed a successful buffered tool call, then send
a tool result with a synthetic safety_identifier. That valid field selects the
reference runtime reconstruction path. Exact seed/client output, both original
request bodies, order/count, credential, state and matching-corruption checks
cover projected fields, RTK shell arguments, large integer fields and null/empty
controls. Plain input without the field uses a separate tagged stateless
request override and is not certified by these reconstruction cases.
Four `deepseek-history-simple*` cases qualify that emitted-body override
separately: known and unknown tool-output IDs, full inline history, and a third
reconstructed request after the stateless second call. The third call requires
the original pending history and an exact intermediate response, so clearing
the cache to make the second wire match cannot pass. All attempts, body bytes,
seed/client output, state and matching mutations remain mandatory.

Build and run from a clean, committed Godex worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
/tmp/differential \
  --prodex /absolute/path/to/prodex-0.438.4 \
  --godex /tmp/godex-candidate \
  --prodex-source /absolute/path/to/prodex-0.438.4-source \
  --godex-source "$PWD" \
  --godex-commit "$(git rev-parse HEAD)"
```

### Pinned Gemini API-key Super executable differential

Use `--gemini` with the same `--prodex`, `--godex`,
`--prodex-source`, `--godex-source`, and `--godex-commit` flags
to run the Gemini cases against the pinned official Prodex 0.438.4 binary.
The original thirteen scenarios below were introduced at 0.438.2 and are
rechecked with the current source and executable pins. The reference was
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
and runs both products independently through bounded scenarios: a successful request; a
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
The resulting PASS is limited to the named DeepSeek synthetic scenarios and is
not equivalent to a global provider, live-TUI or transport parity certificate.

Use `--scenario success|tool-call|deepseek-user-id|sse-stream|sse-rate-limit|retry|deepseek-sse-terminal|single-key-401|single-key-403|single-key-429|single-key-500|single-key-503|key-rotation-429|key-rotation-restart|cancel|restart|recover-after-429|recover-after-503` to rerun one case while
investigating a mismatch; the default is `--scenario all`.

The 0.438.4 `deepseek-buffered-view-*` cases cover literal, overflow, escape,
surrogate, depth-limit, depth-empty, depth-over, early-surrogate, early-range
and patch suffixes. They require byte-exact successful or failed Responses,
original argument strings, the pinned serde diagnostic where applicable,
HTTP status/type, stdout, child exit, one exact authenticated upstream request,
zero retries and safe durable state. The surrogate and two depth-boundary
cases are positive controls. Equal corrupted outputs fail independent invariants.
The array-comma, array-separator and trailing suffixes, plus their search-
prefixed equivalents, require the exact serde array/trailing-input diagnostics
for ordinary tools and structured tool-search calls.
The eof-list, eof-object, eof-value, eof-array-value and search-eof-value
suffixes require the exact container/value EOF message and byte position.
The eof-early-range and eof-early-surrogate controls retain the first serde
failure before truncated input. Other lexical classes remain separate audit
targets.

The eof-string, eof-key, eof-string-unicode, eof-string-escape and
search-eof-string suffixes require exact quoted-string EOF diagnostics for
ordinary and structured tools. The eof-string-surrogate and
eof-string-surrogate-pair cases retain the earlier surrogate error within an
unfinished string; eof-literal-value requires value EOF. The
eof-string-early-range control retains the numeric error before a later string
EOF. The search-output-surrogate-quote case requires the consumed second-escape
position in the full item envelope (column 99).

The `deepseek-buffered-body-under`, `deepseek-buffered-body-limit`,
`deepseek-buffered-body-over` and `deepseek-buffered-body-over-rotation` cases
exercise complete serialized upstream bodies at 4 MiB minus one, exactly 4 MiB,
and 4 MiB plus one. Ignored ASCII padding leaves the translated response small.
Accepted bodies require the exact successful tool response. Oversized bodies
require plain HTTP 502, `provider response could not be processed`, child exit 2,
one original authenticated attempt, no retry and safe durable state. The rotation
case configures two synthetic keys and requires only the primary attempt. These
runtime tests do not claim executable coverage of larger provider-core limits.

The Gemini `buffered-body-under`, `buffered-body-limit`, `buffered-body-over`
and `buffered-body-over-rotation` cases run with `--gemini --scenario <name>`.
They use complete native upstream envelopes at 4 MiB minus one, exactly 4 MiB,
and 4 MiB plus one, projected through the API-key chat-compatible runtime.
Accepted output must match every byte after validating and normalizing UUIDv7.
Oversized output requires exact plain HTTP 502 and exit 2. All four cases audit
stdout, original request, credential slot, one upstream attempt and safe durable
state; the two-key case forbids model fallback and credential rotation. The
recorded child exchange uses the standard private fixture-artifact path, so the
durable-state auditor needs no extra exception. Independent matching mutations
cover output/type/status/exit/stdout, attempts, auth, key, request and state.

The `deepseek-buffered-view-search-output-*` cases cover overflow, overflow-line,
surrogate, depth-limit, depth-over and early-surrogate. They require the exact
outer Mojo-item serde message and byte position, or a successful structured
response for the public depth-limit control (124 arrays).
`deepseek-buffered-view-search-depth-kernel-reject` keeps the inner diagnostic
when the 257-array argument fails structural admission.
`deepseek-buffered-view-search-runtime-depth-over` and its `-rotation` variant
require empty HTTP 500, no Content-Type, child exit 2, one authenticated attempt,
no retry and safe durable state for 125 nested arrays. The rotation variant uses
two synthetic keys. A 126-array item is accepted by provider-core but fails the
public Responses envelope; it is covered separately by focused gateway tests.

Gemini raw-key session identity cases cover ASCII/Unicode byte limits,
overflow and embedded controls. Run one with
`--gemini --scenario raw-key-session-over-limit`; `--scenario all` remains the
default. Gemini model catalogs still require all cases and no DeepSeek workers.
The raw-key cases also audit stdout and durable state. The existing Godex
conversation-selection lock is accepted only as an empty private guard file.

Gemini `raw-key-bound-response-same-process` requires a successful native seed,
then exact HTTP 503 `bound_continuation_unavailable` with no additional upstream
request. `raw-key-unbound-response` requires a fresh HTTP 200 response from the
original input when the previous-response ID is not bound. Both run with
`--gemini --scenario <name>` and enforce exact body/type/exit/stdout, one original
authenticated attempt, credential slot and safe durable state. The state audit
pins the observed Gemini provider, credential and endpoint identities, including
the accepted unknown-ID alias. Missing child responses are explicit failures.
The shared same-process shim rejects a failed seed, preventing its status from
masquerading as the continuation result. These fixtures do not claim Gemini
session/turn executable or Compact payload coverage.

Gemini `raw-key-bound-response-restart`, `raw-key-bound-response-restart-key`
and `raw-key-bound-response-restart-endpoint` launch a successful native seed,
verify durable response ownership, then restart against the same isolated home.
The latter cases replace the key or endpoint only after the seed. Every case
requires the exact successful seed body/type/exit/stdout and a terminal 503
after restart, with one total authenticated original upstream request. The
report retains the seed and its persistence evidence. Matching corruptions
of the seed, persistence, final response, request or state must fail.
Successful Gemini body and session-boundary cases also authorize only their
observed response IDs and the exact accepted fixture session. The state auditor
checks the binding kind and its independent hash namespace, pins provider/key/
endpoint identities for reference snapshots, and requires Godex's conflict
sentinel. Unauthenticated or failed responses authorize no new binding.
The same-process fixture records the actual successful first exchange inside
the existing child-result file and checks its exact body, status and type.
A terminal rejection alone never authorizes durable seed ownership.

Gemini `request-tool-numbers-buffered` and `request-tool-numbers-stream` require
exact original upstream request bytes for a function schema containing signed
and unsigned 64-bit boundaries, integers above 2^53, exponents, negative zero,
a long fraction and unsigned overflow into serde's f64 representation. Both
request stream-flag values use a buffered native response; these are not SSE
response fixtures. The streaming request must include the exact usage option.
The mock retains raw request bytes for these cases because a decoded float64
map cannot detect rounded large integers. Both products must independently
satisfy exact body/type/status/exit/stdout, authenticated credential slot,
one original attempt and safe durable state. Equal rounded or reclassified
number tokens fail even when their decoded float64 maps are identical.

Gemini `request-user-id-*` cases cover absent, canonical, user-alias,
safety-alias, blank, trim-limit, null, number, over-limit, invalid-character
and number-rotation suffixes. Exact original requests require canonical
`user_id` and field-presence priority; Unicode-trimmed 512-byte input is
accepted, while blank selected input blocks lower-priority aliases. Invalid
input requires the released executable's plain HTTP 502 before any upstream
attempt, including two configured keys. The fixtures compare exact response,
type, exit, stdout, authenticated request bytes and safe durable state.
Missing child exchanges remain explicit failing observations.

Gemini `request-parallel-*` cases cover true, false, null, number, string,
array and object values. Every complete valid request must omit the ignored
`parallel_tool_calls` field on the exact authenticated upstream wire and
return the same native response after one attempt. A false value must not
become an unrelated local rejection. Existing independent fresh-response
invariants are reused; matching leaked raw control bytes must also fail.

Gemini `request-control-*` cases require the released terminal plain 502 and
zero upstream attempts for null/string/number stream controls, null/array
instructions, and null/array/number previous-response IDs. The stream-false
control must preserve the exact successful original request and response.
The existing strict accepted/preparation-failure fixture contracts are reused,
including body/type/status/exit/stdout, credential, raw request and safe state.
Matching corruptions cannot turn an invalid control into fresh work.

The same `request-control-model-*` family requires the selected default model
for null, number, boolean, array, object, empty/blank string and unsigned-limit
values. The pinned pipeline substitutes its selected model before adapter
shape validation. A malformed surrogate, raw invalid UTF-8 or numeric overflow
must instead remain a terminal plain 502 with zero attempts; model substitution
cannot repair an invalid original serde body. These cases retain the existing
exact request/response/state and independent corruption checks.

Gemini `request-ignored-<field>-<kind>` cases cover `presence_penalty`,
`frequency_penalty`, `seed` and `n` with true, false, null, number, string,
array and object values. Every case requires one exact original authenticated
request without the ignored field and the same exact native response. Even
`n:7` is ignored by the tagged Gemini bridge. The preceding `request-parallel-*`
cases use the same fixture runner and independent raw-request/state/response
invariants; matching leaked controls cannot pass.

Gemini `request-json-*` fixtures cover a valid Unicode control; null, array,
number, boolean and string roots; malformed/trailing JSON; invalid surrogate/
UTF-8; numeric overflow and excess depth. Parse failures require exact terminal
plain 502; valid nonobject roots require exact empty 500 with no content type.
Both require exit 2 and zero upstream attempts, and the null-rotation fixture
configures two synthetic keys. The valid control requires one exact original
authenticated request and exact native response. All retain strict stdout and
durable-state checks; equal status/body/type/exit/attempt/state corruptions fail.

Gemini `request-primitive-*` and DeepSeek `deepseek-request-primitive-*` cases
cover output, precedence, all, shadow-null, shadow-zero, shadow-fraction, u64,
overflow and parallel-true suffixes. The fixtures pin exact canonical request
bytes, all-alias admission and the absence of an ignored parallel control.
Invalid maximum-token aliases require terminal plain 502 and no upstream
attempt. Positive controls require one authenticated original request and
exact native Responses output/type/status/exit/stdout. Both providers retain
safe durable-state checks. Matching raw-body, auth, slot, response, retry,
stdout and state mutations fail independent invariants; missing exchanges are
explicit failures. The same technical fixture data serves both existing
provider harnesses without introducing a production provider abstraction.

The same family adds `numeric-control`, `numeric-exponent`,
`numeric-negative-zero`, `numeric-fraction`, `numeric-u64`, `numeric-unicode`,
`numeric-temperature-null`, `numeric-temperature-string`, `numeric-top-p-bool`
and `numeric-top-p-array`. Accepted requests must preserve exact pinned serde
number bytes, literal HTML-sensitive characters and Unicode line separators.
Invalid numeric-control types require plain terminal 502, exit 2 and zero
attempts. Fixed expected wire bytes remain independent of production encoding;
the existing response, credential, state and matching-corruption checks apply.

The `logprobs-false`, `logprobs-zero` and `logprobs-max` suffixes require exact
accepted request bytes and responses. `logprobs-null`, `logprobs-string`,
`logprobs-top-null`, `logprobs-top-negative`, `logprobs-top-float`,
`logprobs-top-over`, `logprobs-top-u64`, `logprobs-top-missing-flag` and
`logprobs-top-false-flag` require terminal plain 502, exit 2 and zero attempts.
Both provider runners reuse the same fixed fixture data and independent
matching-corruption contracts; a shared incorrect rejection or dropped valid
control cannot satisfy the oracle.

The `stop-string`, `stop-empty-string`, `stop-empty-array`, `stop-snake`,
`stop-camel`, `stop-precedence`, `stop-alias-precedence` and `stop-limit`
suffixes require the exact canonical `stop` bytes, including empty values,
unchanged whitespace, selected-alias precedence and the 16-string boundary.
`stop-shadow-null`, `stop-null`, `stop-bool`, `stop-array-number` and `stop-over`
require terminal plain 502 with no attempt. Lower-priority invalid aliases
must not override a valid selected value, while a selected null cannot fall
through to a valid alias. Both products must satisfy independent fixed request,
response, authentication, exit/stdout, attempt and durable-state contracts.

The DeepSeek-only `deepseek-request-primitive-admission-*` suffixes cover
`stream-false`, `stream-null`, `stream-string`, `stream-number`,
`instructions-null`, `instructions-array`, `previous-null`, `previous-array`
and `previous-number`. The valid false control must retain the exact original
authenticated request and response; wrong types require terminal plain 502,
exit 2 and zero attempts. The existing primitive mutation contracts also cover
these fixtures. They add no duplicate cases to the Gemini executable matrix.

DeepSeek `deepseek-request-primitive-json-*` cases preserve original raw JSON
for a valid Unicode control, five nonobject root kinds and six parse/serde
failures (syntax, trailing data, surrogate, UTF-8, overflow and depth).
Valid nonobject roots require empty terminal 500 with no Content-Type header;
malformed bodies require plain terminal 502. Both require exit 2 and zero
attempts. Rejected bytes are never re-encoded by the fixture; equal empty-header,
status/body/exit/stdout/retry/state mutations cannot satisfy its invariants.

DeepSeek `deepseek-request-primitive-user-id-*` reuses the existing identity
fixtures for absent, canonical, user/safety aliases, blank, Unicode-trimmed
512-byte values, null, number, over-limit and invalid-character cases.
Accepted controls require exact canonical `user_id` wire bytes or omission;
invalid identities require terminal plain 502, exit 2 and zero attempts.
Selected null/blank values suppress lower-priority aliases. Existing independent
auth/slot, response, retry, stdout and durable-state mutation checks apply.

DeepSeek `deepseek-request-primitive-preparation-*` covers rejected frequency/
presence penalties, seed, n, malformed include/store, background execution,
automatic truncation, malformed text/tools/reasoning, missing named tools and
invalid web-search context size. Each requires the same exact terminal plain
502, exit 2 and zero upstream attempts. These cases exercise the common
translation-failure boundary alongside all earlier successful control cases;
matching errors, missing exchanges or unsafe durable state still fail.

Gemini adds `request-primitive-preparation-tools`, `preparation-reasoning`,
`preparation-tool-params`, `preparation-tool-missing-params`,
`preparation-tool-missing-name`, `preparation-tool-description` and
`preparation-input` under that prefix. Malformed tool declarations require
their exact indexed structured 400, `invalid_request_error`, JSON content type,
no-store/nosniff headers, exit 2 and zero attempts. Invalid input/reasoning
requires plain terminal 502. Independent mutations cover both error forms;
Content-Type mutations use a value different from every valid fixture type.

Gemini `request-primitive-tool-priority-*` combines malformed function
parameters with invalid stream, temperature, token count, user ID, input or
reasoning. Each must preserve the exact indexed structured 400 and its terminal
headers with no attempt. The serde suffix instead includes an unpaired
surrogate and must preserve plain terminal 502 before tool validation.
The original raw body is retained and existing independent error/state/header/
attempt mutations cover every combination.

Gemini `request-primitive-candidate-*` covers null/one acceptance, both aliases,
strict integer admission, canonical equal booleans/strings/arrays/objects and
floating values, and alias conflicts including integer versus float. Candidate
errors win over malformed tools, controls, input and reasoning; invalid Unicode
still wins first. Rejections require exact structured 400 messages, terminal
headers, exit 2 and zero attempts. Accepted counts are omitted on the Chat wire.
The same independent body/header/exit/attempt/state mutations cover these cases.

Gemini `request-primitive-beta-*` rejects present top-level completion
`prefix`, `suffix` and `prompt`, including null, false and empty strings, with
plain terminal 502 and zero attempts. Controls retain unrelated nested metadata,
earlier candidate/tool structured 400s and complete serde failure precedence.
Original request bytes and all existing independent mutations remain checked.

Gemini `request-primitive-input-item-*` rejects nonobject entries before or
after a valid message, and any present item `prefix`, including null/false on
messages or function calls. Controls retain ordinary message arrays, ignored
item metadata, earlier candidate/tool 400s and complete serde failure behavior.
Plain 502/exit 2/zero-attempt expectations and matching-corruption mutations
prevent silently dropping an invalid item before sending the remaining input.

Gemini `request-primitive-input-kind-*` rejects null, scalar/container and
unknown explicit types, including loose input/output text and reasoning items.
Explicit and implicit message controls remain accepted; rejected image controls,
earlier candidate/tool 400s and malformed serde retain their exact outcomes.
The existing exact wire, zero-attempt and independent mutation checks apply.

Gemini `request-primitive-message-role-*` rejects wrong-type, empty, unknown,
tool and padded roles on explicit messages. Controls accept canonical user,
omitted role and escaped user, preserving exact canonical request bytes;
earlier candidate/tool errors and invalid Unicode retain their prior outcomes.
All rejected cases require plain terminal 502, exit 2 and zero upstream work.

Gemini `request-primitive-message-content-*` places null/boolean/numeric
explicit-message content before a valid message, preventing silent discard
followed by an ordinary upstream attempt. String and text-part-array controls
retain exact canonical bytes; earlier candidate/tool errors and invalid Unicode
retain their exact terminal outcomes and independent mutation checks.

Gemini `request-primitive-projection-*` checks exact message wire bytes for
string/alias arrays, empty separators, standalone canonical objects, empty/
system/developer messages, implicit roles and instructions-only requests.
Part cache-control and malformed objects fail with zero attempts; ignored media,
scalar parts, nested prefix metadata and whitespace instructions retain the
tagged behavior. A complete expected request body keeps these checks independent
of the production content projector and preserves matching-corruption mutations.

Gemini `request-primitive-call-id-*` rejects missing, wrong-type, blank or
empty-shadowed tool identities before conversion/orphan filtering. Function,
custom, MCP, shell and output controls share the exact terminal admission
contract; ordinary valid IDs and earlier candidate/tool/serde outcomes remain
checked with exact wire bytes, zero attempts and independent mutations.

Gemini `request-primitive-call-name-*` rejects missing, wrong-type, blank and
empty-shadowed names before conversion/orphan filtering. Function, custom and
MCP cases retain terminal 502 behavior, valid-name controls and earlier local
tool/candidate/serde precedence with independent matching-corruption checks.

Gemini `request-primitive-tool-output-*` rejects missing result fields before
conversion. Matched function-call/output controls accept explicit null, empty
and string payloads with exact call/output wire bytes. Existing custom/MCP
rejections and earlier local errors remain checked with zero-attempt and
independent mutation requirements. Orphan-output shaping is a separate audit.

Gemini `request-primitive-orphan-output-*` requires unmatched function/custom/
MCP results to disappear before the exact upstream request. Null/empty/string
orphan results retain the ordinary user message; output-only requests require
the tagged empty-user fallback. The ordinary control, matched-output scenarios
and all independent wire/status/attempt/state mutations remain active.

Gemini `request-primitive-function-alias-*` compares full independent request
bytes for ID/name/argument/output aliases, absent versus null arguments,
canonical nested JSON, ignored namespace and selected-name shadowing. Matched
call/output fixtures expose field loss before adjacency; accepted name-validation
edge cases retain the tagged empty assistant message and discard orphan output.

Gemini `request-primitive-empty-input-*` verifies absent/empty-array/blank-
instruction requests use exactly one empty user message. Empty-string and
system-only controls remain accepted; null/object input, earlier candidate
errors and malformed Unicode retain exact rejection and zero-attempt behavior.

DeepSeek scenario workers evaluate their complete independent invariants as
each scenario finishes, preserving failures in the ordered report. The strict
five-minute generated SSE timestamp window is unchanged; later cases cannot
age already observed valid frames into a false failure. Stale/future timestamp
mutations must still fail when observed. This changes test-harness timing,
not product clock, stream, retry or comparison policy.

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

`sse-tool-name-padded` keeps a padded `tool_search` name as an ordinary function
call. `sse-tool-name-blank-namespace` preserves a name whose apparent namespace
separator has a blank side. `sse-tool-name-blank-late` requires a later present
empty name to invalidate the earlier name and fail before tool completion.
All compare full event bytes and independently reject altered names, sequence
numbers and call identities.

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

The 0.438.4 DeepSeek history matrix also checks three-call numeric pending
snapshots at 2^53+1 and uint64 maximum. Its independent expected bytes detect
rounding on the third request even when the preceding request matches.

The Gemini reasoning SSE cases compare exact summary lifecycle and completion
metadata for separate, mixed, empty and reasoning-only deltas. Two streamed
Compact seeds verify retained user/assistant history, including null assistant
content for reasoning-only output. Equal-but-wrong wire, request, metadata and
state mutations must fail independently. Local reasoning retention is capped at
64 MiB; executable parity above that safety ceiling remains unqualified.

Five additional Gemini SSE metadata cases require the latest nonnull logprobs
and nonempty system fingerprint, including opaque numbers, replacement and
null/empty controls. Exact completion bytes and matching numeric corruption
remain independent assertions.

The unsigned Gemini stream timestamp cases require upstream created values before
the first emitted event, covering zero, 2^53+1, uint64 maximum, chunk updates and
invalid numeric controls. The oracle checks timestamp order before normalization.

Five model-presence stream cases distinguish explicit empty model strings from
absent/null/wrong-type fields, including replacing a previous model. Exact client
wire and matching model-injection mutations remain independent assertions.

Five finish-reason presence cases preserve explicit empty strings, including
replacement and later null/absent updates. Null, wrong-type and absent controls
retain the strict terminal-sentinel lifecycle and independent state checks.

Fifteen Gemini Chat-compatible SSE usage cases compare independent exact usage
bytes for null, empty/non-object values, unsigned counters, invalid numeric
kinds, saturating/default totals and cache/reasoning detail presence. Five
controls retain valid counters, explicit zero totals and absent usage. Original
requests, stdout, single attempts and durable state remain strict; matching
usage, request, auth, attempt and state mutations must fail the oracle.

Eight refusal/annotation SSE cases require append order, arbitrary annotation
elements and full unsigned precision, including metadata-only completions with
no invented text item. Null, empty, wrong-type and absent updates preserve the
tagged first-choice projection. Equal-but-wrong metadata, numeric precision,
SSE sequence, request, auth and durable state must fail independent assertions.
Refusal/annotation retention above the local 64 MiB safety budget is unqualified.

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

The synthetic shim normally rejects incomplete response reads without recording
an exchange. Expected transport-failure probes can explicitly set the private
`DIFFERENTIAL_CAPTURE_READ_ERROR=1` fixture environment. Such a capture retains
status, headers, bounded partial bytes and a nonempty `read_error`, still exits
1, and never retries or uses the failed read as a continuation seed. The 1 MiB
limit remains mandatory even in this mode. A read-error record alone is never
parity evidence: a focused invariant must verify the expected error, wire,
status, upstream attempts and state for each product independently.

Sixteen Gemini stream-projection cases distinguish five non-object tool-entry
kinds and five non-object first-choice kinds from ignored/empty tool-call and
valid-first-choice controls. Non-first choices retain their ignored behavior.
The negative
cases require the tagged pending response: header 200, the exact SSE content
type, empty partial bytes, the explicit read deadline, stdout newline and child
exit 1. Successful controls retain complete exact SSE bytes. Every case checks
original requests, one upstream attempt and state; matching failure-tuple,
status, bytes, auth, attempt and state corruption must fail. PASS qualifies the
expected failed-read contract, never a successful response. Object entries
missing a function object are a separate admission behavior.

Six semantic-failure cases cover object tool entries with absent/null/wrong
function objects, a tool-only rejection, and embedded errors before/after text.
They require exact failed-event sequencing and subsequent text, no completion,
full reads, status 200 and child exit 0. Existing ignored-tool controls remain
independent successes. Original request bytes, auth/slot, attempts, stdout and
durable state are asserted separately for each product; matching corruption of
those values or of the failure/continuation wire must fail. Successful tool
lifecycles, accumulated arguments and tool snapshots remain outside this scope.
