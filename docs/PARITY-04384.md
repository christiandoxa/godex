# Prodex 0.438.4 executable parity campaign

**Target:** the published Prodex `0.438.4` release, not the installed system's
previous Prodex `0.438.3`. All runtime comparisons use synthetic fixtures,
separate private profile homes and an authenticated loopback upstream. This
is a bounded differential campaign, **not** a certificate of 1:1 parity
across all providers and platforms.

## Release provenance

- Official upstream tag: `0.438.4`
- Official source commit: `4cf03ccff2b6132058e2359fd253ad6e2d107011`
- Published Linux x86-64 SHA-256:
  `b736e1b2430b80ff2af35cff18e884b42ca83e8028c4fd47df5285f5d0d584e9`
- Published `release-manifest.json` SHA-256:
  `28b7a894cd10fb69c528245e25dc01b9f204cfcc78b5ee575e22b4967c72c650`
- The remaining five native-platform binary digests are taken directly from
  the official 0.438.4 `SHA256SUMS`; the native platform runner must verify
  each binary and manifest independently before running local tests.

The pinned source's root `Cargo.lock` resolves `serde 1.0.229` and
`serde_json 1.0.154`. Earlier entries below that mention 1.0.228/1.0.151
describe historical probes, not the locked dependencies of this release.
Those probes alone do not certify 0.438.4 numeric or typed-serde behavior;
new source checks must use the actual lockfile versions. The native-stream
error goldens below were regenerated with the correct versions and were
byte-identical to the earlier probe's 32 fixtures. The locked-dependency
audit below separately revalidates 48 numeric goldens and a bounded
21,317-input value corpus; typed Kiro behavior is not covered by that probe.

Downloaded artifacts are checked against the official SHA256SUMS and manifest;
Prodex reports `prodex 0.438.4`. The pin was migrated coherently in the
release gate, workspace, differential, profile, context, CLI, footer and
continuation oracles. The release gate fetches the tagged source and matching
binary rather than trusting the installed `prodex` on the host.

## HTTP/SSE event-kind normalization and retry boundary

The released Rust response metadata parser normalizes SSE `event_type` with
`.trim()` **before** calling the compiled `rich_route.mojo` event-kind policy.
Mojo then compares the normalized type exactly. An initial hypothesis that
`" response.created"` or `"response.created "` must immediately commit
was **incorrect** when tested against the official 0.438.4 executable.
Both are precommit hold events, just like canonical `"response.created"`.
A later quota failure may therefore rotate to a Ready alternate before the
client receives output. The Godex SSE inspector and the original HTTP proxy
regression retain that normalization.

The focused `--http-sse-event-kind` source/binary/manifest-pinned oracle
compares canonical and padded first events with an actual flushed upstream
boundary, a following quota failure, exact upstream request bytes, two
managed Ready attempts, and recovered downstream SSE bytes. Those controls
also run inside `--http-turn-transport`. Any apparent contradiction from
reading one Mojo function without the Rust normalization layer must be
resolved against the complete tagged execution path.

## Migration and limits

The tagged upstream changed substantially from 0.438.3 to 0.438.4, including
its compiled Mojo policy and source tests. Existing 0.438.3 fixture labels
remain as historical provenance, while all **active executable, source,
manifest and platform SHA pins** now select 0.438.4. Passing old regression
cases is evidence for those behaviors only. Expanded 0.438.4-specific
source/behavior comparisons remain required for any claim of overall 1:1
parity, especially provider-specific proxy paths, Kiro CLI 2.33.0, and
native macOS/Windows execution.

## Kiro ACP update policy and unsigned usage

The 0.438.4 Rust `runtime_kiro_acp/protocol.rs` delegates the precise
`session/update` discriminator and required field checks to the compiled
`kiro_acp_update_policy.mojo`. Its turn-state collector calls
`parse_session_notification().ok()`: malformed updates are **ignored**, not
converted into tool activity, usage or streamed model output. The notification
must include a string `sessionId`, an update object and a string
`sessionUpdate`. Recognized variants require their appropriate fields:
`tool_call` needs string `toolCallId` and `title`, `tool_call_update` needs
string `toolCallId`, `usage_update` needs full-range unsigned `used` and
`size`, `plan` needs valid typed entries, and the command/mode updates need
their appropriate list/string. The exact discriminator is case- and
whitespace-sensitive. Unknown variants remain valid but have no state effect.

Godex previously projected raw `session/update` maps directly, even if the
Rust parser would reject them. Fail-before regressions demonstrated phantom
tool activities with no ID/title, overwriting a good usage snapshot with a
malformed update, and client-visible live text from a notification missing
`sessionId`. The scoped Go validator now gates both buffered collection and
streaming observation before mutating turn state, preserves earlier valid
updates, and retains the original network request bytes. A second fail-before
regression found `runtimeUint` parsing `json.Number` with `Int64` rather than
`Uint64`: accepted u64 usage values above `2^63-1` became **zero**. The
projection now preserves all accepted 64-bit unsigned usage and computes
remaining capacity without signed overflow.

The matrix checks the exact recognized/unknown variants, missing and
wrong-type fields, valid plan and usage controls, cost parsing, and streamed
activity suppression. It is matched to the published 0.438.4 Mojo/Rust source
and is exercised by local and race-enabled Kiro package tests. A local
`kiro-cli` reports version 2.13.0; the release's Kiro CLI 2.33.0 smoke
qualification has **not** been reproduced with that live binary, and no
personal Kiro credential, conversation, or provider session was accessed.

## Kiro ACP optional-field presence and clearing semantics

Tagged Rust `runtime_kiro_acp/turn_state.rs` models plans, available
commands, current mode and session title/timestamp as `Option` values.
Its compiled `kiro.mojo` `AcpMetadata` serializer checks whether each
field is **present**, rather than whether the value contains any elements
or non-whitespace text. Thus a valid `plan` update containing `entries: []`
produces `"plan": []`, not an absent field; `availableCommands: []`
produces an empty array; an explicit `currentModeId: ""` emits an empty
string. A later `session_info_update` with `title: ""` or `updatedAt: ""`
overwrites an earlier value. Missing optional title/timestamp keys leave
the earlier values untouched. Exact Unicode and leading/trailing whitespace
in these optional strings must be preserved.

Godex previously dropped empty plans and commands with `len` checks,
normalized mode/title using `runtimeString` and omitted all-empty session
metadata. Fail-before tests reproduced lost empty fields, stale previous
entries following a clear update, and silently trimmed session titles.
The new projection keeps explicit presence for current-mode and optional
session title/timestamp, distinguishes unset from a present empty slice,
and sends nil only for truly absent optional members. This changes no
client request bytes, and uses synthetic ACP notifications without loading
real Kiro CLI credentials. Source/compiled-Mojo alignment and local
race-enabled regression tests have passed; real Kiro 2.33.0 interaction
remains outside this bounded proof.

## Kiro ACP tool detail-redaction evidence distinguishes two Rust Option kinds

The tagged `runtime_kiro_acp/turn_state.rs` calculates whether a synthetic
tool activity must hide its details from four optional fields. `rawInput`
and `rawOutput` are `Option<serde_json::Value>`: **a present JSON null is
Some(Null)** and still sets `details_omitted: true`. By contrast `content`
and `locations` are `Option<Vec<Value>>`: only **a nonempty array** triggers
the flag. Empty arrays and non-array JSON values do not count as evidence.

Godex previously used Go `map` value non-nil checks for all four, which
lost explicit raw-input/raw-output nulls and falsely treated empty or
wrongly-typed content/location fields as hidden details. Fail-before
regressions cover each form for both initial `tool_call` and subsequent
`tool_call_update`, including nonempty arrays and true missing-field
controls. The scoped correction checks raw-field **presence** and typed
nonempty arrays separately, preserving the source's safer redaction
intent without including sensitive raw tool arguments in generated
activity descriptions. This case is verified against the exact 0.438.4
Rust source and local/race tests; no real provider/tool credentials were
accessed, and it is not a new timed Kiro CLI 2.33.0 executable oracle.

## Kiro ACP tool label inheritance uses field presence, including empty IDs

The tagged `kiro_acp_tool_activity_policy.mojo` computes label inheritance
from **Option<String> presence**, not from whether a value is nonblank.
An explicitly present `title: ""` or `kind: ""` suppresses a previous label
and allows the downstream sanitizer to choose the corresponding safe
fallback. Only an absent optional field may reuse a previously verified
label. The compiled ledger also accepts a zero-length string `toolCallId`
as a key: the only ID-length limit is **at most 256 UTF-8 bytes**.

Godex previously called `runtimeString(value, previous)` and therefore
reused prior labels for explicitly empty or whitespace-only updates. It
also refused to store labels for an empty `toolCallId`. Fail-before
regressions cover absent/present title and kind separately, both empty,
whitespace-only, and an empty ID used by two consecutive valid tool
notifications. The correction uses field presence and the same tagged
Mojo byte-length and previously-known-label conditions for recording the
safe redacted label. The resulting synthetic tool-activity text contains
only sanitized names, not raw tool arguments. This is supported by the
published 0.438.4 Mojo/Rust source and local tests; no actual CLI provider
sessions or secrets were read.

## Kiro ACP typed-envelope serde boundaries

`RuntimeKiroAcpEnvelope::parse` in tagged Rust deserializes directly to a
**typed serde struct** before any notification is considered. The Go
`json.Unmarshal` of that struct accepted duplicate known top-level fields,
which serde rejects even if the duplicate values are equal. The same typed
constraint also applies to nested `RuntimeKiroAcpError` (`code`, `message`,
`data`) and its required non-null `code: i64` and `message: string`. By
contrast, repeated *unknown* fields and repeated fields inside the generic
`params: Value` are allowed and use the last map value. A fail-before suite
now verifies that precise boundary, not a blanket duplicate-key ban.

Serde also rejects malformed UTF-8, lone Unicode surrogate escapes, excess
container nesting, and its own floating-point overflow boundary. Go's
standard decoder can replace invalid Unicode or accept numbers like
`1.7976931348623158e308` that the pinned serde default rejects. Historical
offline tests using Rust `serde 1.0.228` and `serde_json 1.0.151`
confirmed the typed duplicate, Unicode, recursive
and near-f64-maximum cases. Godex now checks those invariants before the
normal ACP JSON-RPC decoder can turn an invalid notification into Kiro
output, tool activity or usage. The source-matched serde-f64 parser was
moved into `internal/helper/jsonutil` and is shared with its existing
Responses/usage validation, avoiding divergent numeric acceptance rules.

These are isolated source- and dependency-aligned semantic regressions on
synthetic Kiro envelopes; they **do not** claim a live authenticated Kiro
CLI 2.33.0 handshake or full provider parity. Actual private credentials
and the installed older Kiro CLI are not exercised.

## Kiro tool activity labels and statuses require bytewise ASCII case folding

The published `kiro.mojo` `kiro_activity_contains` checks sensitive activity
labels with **ASCII A–Z byte folding only**, and
`kiro_activity_status_code` uses the same rule after trimming status
whitespace. Go's `strings.ToLower` performs Unicode case conversion instead:
U+212A KELVIN SIGN (`K`) becomes ASCII `k`, and U+0130 dotted capital I
(`İ`) becomes ASCII `i`. This made names such as `toKen report` or
`credentİal report` falsely match the secret-name denylist, and accepted
`FAİLED` as a recognized `failed` status when the pinned kernel considers
it `unknown`. Both mismatches were reproduced as fail-before local tests.

Godex now folds only ASCII capital bytes for both sensitive-label scanning
and known activity statuses, retaining all non-ASCII UTF-8 bytes unchanged.
Canonical `TOKEN`, `Authorization`, `RUNNING`, `FAILED` and padded ASCII
controls remain correctly redacted or classified; Unicode confusables no
longer steal those identities. The normalization is limited to internal
synthetic activity metadata, not credentials or the original ACP wire.
This is source-pinned Kiro 0.438.4 parity, not an authenticated 2.33.0
Kiro CLI binary experiment.

## Kiro ACP text extraction recurses into nested `content` values

Published `kiro.mojo` `kiro_json_write_content_value` processes a text
string directly, walks arrays in order, and recursively processes an
object's `content` member when the object has no direct string `text`.
The Rust provider-core wrapper uses that compiled kernel for buffered ACP
turn state and streaming text. Godex's earlier `acpContentText` supported
strings, arrays, and the direct `text` member only. Nested envelopes such
as `{"content":[{"text":"hello"},{"text":" 世界"}]}` silently yielded an
empty answer even when the upstream provided real assistant content.

Fail-before regressions cover nested maps and arrays, text-vs-content
precedence, a numeric non-text field with content fallback, ignored unrelated
members, and valid direct controls. A buffered Kiro turn and a live Chat SSE
chunk both now retain the entire original Unicode text. The source-aligned
recursive walker is capped at 128 levels, mirroring the compiled Mojo depth
guard and avoiding unbounded recursion. This change is bounded to internal
ACP content projection; it does not fabricate tool calls, expose raw tool
arguments, or alter the input wire. The comparisons use tagged source and
local synthetic fixtures; an authenticated Kiro CLI 2.33.0 integration is
still separately unverified.

## Kiro ACP optional cost and canonical serde f64 serialization

The 0.438.4 `RuntimeKiroAcpCost.amount` is a typed Rust `f64`. The source
provider's `kiro_provider_core_acp_usage_update_json` serializes that typed
value into the Mojo usage-update JSON; it does **not** retain the original
ACP numeric literal. Previous Godex usage metadata did retain `json.Number`
lexemes, so source `5` stayed `5`, `1e16` stayed `1e16`, and long decimals
retained every digit instead of the canonical serde `5.0`, `1e+16`, and
shortest round-trip f64. The source additionally always passes a JSON extra
object containing `"cost"`: missing or null `Option<Cost>` becomes an
**explicit `"cost":null`**, rather than omitting the member entirely.

Fail-before regressions cover integer-typed and fractional amounts,
negative zero, small/large exponent thresholds, long mantissas, u64 maximum,
valid underflow and absent/null cost. The shared pinned serde_json 1.0.151
f64 parser now has a matching canonical-format helper used by both quota
JSON size accounting and the Kiro cost projector. The accepted numeric
value is unchanged; only its exact serialized representation is corrected.
The optional null key is also always retained. This is source/dependency-
aligned local JSON parity, not a full Kiro CLI 2.33.0 runtime oracle.

## Kiro ACP stop reason uses first-present member precedence

The tagged compiled `KIRO_ACP_STOP_REASON` kernel searches the prompt-result
object in this order: `stopReason`, `stop_reason`, then `status`. Its rule is
**first-present key**, not first member with a string value: a present
`stopReason:null`, number or object yields `null` and does not borrow a
later `"max_tokens"`, `"refusal"`, or `"cancelled"` alias. A real empty
string likewise blocks fallback. Godex previously searched for the first
*string-typed* member and could incorrectly mark a successful Kiro turn
`incomplete` on malformed but higher-priority stop metadata.

Fail-before regressions exercise canonical, snake-case and status-only
controls as well as null, numeric, object, Boolean and empty-string
precedence, including the resulting Kiro incomplete-response classification.
The scoped parser now stops at the first present member and returns an empty
reason when that member is not a string. It does not change the original
ACP prompt-result bytes or the independent Kiro cost/tool-activity policy.
This source-aligned synthetic case is not a claim of live Kiro CLI 2.33.0
end-to-end certification.

## Kiro ACP sub-agent permissions select from heterogeneous JSON values

The tagged Rust `runtime_kiro_acp_reject_unsupported_server_request`
handles `session/request_permission` in sub-agent mode by reading
`params.options` as a generic `serde_json::Value` array. It looks for the
first `kind:"allow_once"` (or, if none, the first `"allow_always"`), then
checks **only that chosen option's** `optionId` as a string. Unrelated
options can have null, numeric, array or malformed field types and are
simply skipped. An already selected `allow_once` with a missing/non-string
ID instead produces the cancelled outcome; it must not silently borrow
another option.

Godex previously parsed all options into a strict `[]struct` and cancelled
the entire permission request when an unrelated option contained a number
or an array, even if a valid one-time choice followed it. Fail-before tests
cover those heterogeneous lists and the actual JSON-RPC response map,
including allow-once precedence, the allow-always fallback, malformed
selected IDs, empty IDs, and non-array options. The scoped correction uses
a typed outer JSON map with generic option objects, preserving the exact
Rust precedence and fail-closed selected-ID behavior. No real Kiro
permission, account, process or credential was exercised.

## Kiro `usage_update.cost: null` is a malformed PRESENT input

The released Rust `RuntimeKiroAcpSessionUpdate::parse_value` parses cost as
`value.get("cost").cloned().map(serde_json::from_value::<Cost>).transpose()`.
This is **not** a serde `Option<Cost>` decode: an absent cost is `None`,
whereas a present JSON `null` becomes `Some(Null)` and fails to deserialize
the required `Cost { amount: f64, currency: String }` struct. In contrast,
the *output* metadata always serializes the accepted `None` as an explicit
`"cost": null`. Godex previously conflated those stages and accepted a
present null, silently overwriting previously valid quota usage.

A pinned serde_json 1.0.151 offline probe confirmed `Some(Null).map(...)
.transpose()` returns a parse error while serde's ordinary `Option<Cost>`
would instead accept null. Fail-before buffered and live Kiro usage tests
cover absent, valid present, malformed present null, empty-object and
non-object cost, preserving the last valid usage and its canonical f64 cost.
The scoped update now validates all **present** cost values, including null,
before updating the turn. Synthetic source/dependency tests are not a live
Kiro CLI 2.33.0 provider run.

## Gemini media metadata and extension matching are ASCII-casefold only

The released `gemini_response_media.mojo` classifies request image data URLs
by searching semicolon-delimited metadata for exactly the ASCII `base64`
marker, permitting **ASCII** uppercase but not Unicode simple-fold
lookalikes. It also identifies filename extensions with ASCII-only
case-folding. The older Go Gemini request parser called
`strings.EqualFold(segment, "base64")`, so U+017F LONG S in
`data:image/png;baſe64,AAAA` could falsely authorize inline image data.
Its extension matcher used `strings.ToLower` on entire URLs, mapping U+0130
DOTTED CAPITAL I in `.GİF` to ASCII `.gif` and falsely marking an opaque
URI as `image/gif`. Both differences were reproduced as fail-before tests.

The scoped fix uses bytewise ASCII-only comparisons for the base64 marker
and URI extension normalization. It preserves legitimate lowercase,
uppercase and mixed-case ASCII forms, query/fragment suffix handling, and
valid explicit base64 segments alongside invalid confusable segments. Tests
cover both the parser and actual Gemini image-part request conversion.
These are published-Mojo source and local parser parity checks; the
existing official Gemini executable differential is a broader regression
gate, not a claim that every Unicode media fixture reached the native
Gemini API-key provider path in that oracle.

## DeepSeek provider error classification uses ASCII-only case folding

Prodex 0.438.4 routes DeepSeek API errors through the compiled
`provider_error.mojo` classifier. Its `provider_error_ascii_lower` folds
**ASCII bytes only** when normalizing structured codes and matching
case-insensitive provider status texts. Godex previously called
`strings.ToLower` on raw codes and messages, allowing U+212A KELVIN SIGN
(`K`) in `invalid_api_Key` to become the genuine authentication error
`invalid_api_key`, or U+0130 dotted capital I (`İ`) in
`rate_lİmit_error` / `MODEL İS NOT SUPPORTED` to become a rate-limit or
model-not-found signal. Those false classifications could incorrectly
invalidate a credential, quarantine it, or attempt an unrelated fallback.

A fail-before regression demonstrates each incorrect decision, with exact
ASCII, mixed-case, quota and rate-limit positive controls. The token
projector also used Unicode lowercasing and mutated unrelated international
codes such as `THÉ_雪`, whereas Mojo preserves all non-ASCII bytes. Both
structured token normalization and downstream classification now apply a
single bytewise ASCII-only fold. The error body and client response are
unchanged. Local tests validate source-aligned classifications; the official
DeepSeek executable differential remains the broader regression gate, not
a claim that the synthetic Unicode confusables were tested against a live
provider or authenticated third-party service.

## DeepSeek structured error codes require complete serde-valid JSON

The tagged `prodex-provider-core/src/errors/body.rs` obtains provider
error-code candidates from `mojo_json::values_from_json_or_sse`, which
first requires a **complete valid serde_json Value** for each body or SSE
`data:` event. Godex previously used Go `json.Decoder` with `UseNumber`
without the serde-specific Unicode, depth, and floating-point checks.
Malformed JSON containing a correct `rate_limit_exceeded` code plus an
unrelated lone `\uD800`, invalid UTF-8, an over-deep container or a number
like `1.7976931348623158e308` was incorrectly allowed to mint a trusted
rate-limit classification. For an HTTP 429 this could quarantine a key or
rotate a request even though Prodex would ignore the entire event.

Fail-before tests reproduce these cases in buffered HTTP JSON and SSE error
frames, with positive paired-surrogate and near-maximum long-mantissa
controls. The DeepSeek code extractor now uses the shared, tagged-compatible
JSON boundary and serde numeric checks **before** decoding each candidate
body/event. On rejection, the original provider body remains untouched but
no synthetic structured error signal is trusted. Earlier ASCII-only error
code/token classification and ordinary valid quota/rate/auth paths are
preserved. These are local source/dependency-pinned semantic regressions;
the official executable DeepSeek suite provides a separate broader transport
no-regression check, not a claim of live provider coverage for all malformed
Unicode/f64 fixtures.

## DeepSeek reasoning-effort labels require ASCII-only case folding

Prodex 0.438.4's `deepseek_trimmed_effort_equals` Mojo kernel trims
surrounding Unicode space but folds only ASCII uppercase bytes before
classifying `none`, `minimal`, `low`, `medium`, `high`, `xhigh` and `max`.
Godex's reasoning object and top-level `reasoning_effort` decoder previously
used Unicode `strings.ToLower`; U+0130 dotted capital I in `HİGH`,
`MEDİUM`, `MİNİMAL` or `XHİGH` was converted into ASCII `i` and an
otherwise invalid effort could silently enable, disable, or increase model
thinking. A fail-before regression reproduced the wrong outbound thinking
fields for both accepted request shapes.

The scoped correction uses the already validated DeepSeek ASCII-only byte
folding helper after whitespace trimming, retaining the canonical ASCII
uppercase/mixed-case positive controls. Unsupported Unicode confusables now
produce the ordinary invalid-effort error before reaching the provider.
This is a tagged source and local request-semantic parity result, separate
from the broad official DeepSeek executable suite and live-provider paths.

## DeepSeek model catalog lookup uses exact ASCII-folded identities

The tagged 0.438.4 provider models endpoint delegates catalog resolution
to `prodex_mojo_rich_catalog_resolve_exact_v1`, whose
`catalog_byte_equal_folded` lowercases **ASCII A–Z only** and preserves
all non-ASCII UTF-8 bytes. Go's DeepSeek `/models/{id}` endpoint previously
used Unicode `strings.EqualFold`. The query `deepseeK-v4-pro` (U+212A
KELVIN SIGN) was therefore incorrectly matched to the real
`deepseek-v4-pro` model and returned HTTP 200 instead of a
`model_not_found` 404.

A fail-before local endpoint regression covers canonical, uppercase and
mixed-case ASCII identifiers as positive controls, plus Kelvin-confusable
and padded model IDs as negative controls. Model lookup now uses an explicit
bytewise ASCII fold, keeping the original catalog and exact model response
body unchanged. This source-pinned synthetic parity slice does not claim
every dynamic provider catalog/model alias combination has been verified
in a live Prodex 0.438.4 deployment.

## Gemini models endpoint cannot trim catalog query identity

Prodex 0.438.4 obtains provider model JSON through
`runtime_provider_model_json_for` and the compiled
`rich_catalog_resolve_exact_v1` identity resolver. The reference kernel
compares the original query ID with case-insensitive **ASCII** bytes, but
never removes leading or trailing whitespace. This behavior is covered by
the official source test rejecting `" gpt-main "` even though `"GPT-MAIN"`
resolves normally. Godex's Gemini `/models/{id}` route previously called
`strings.TrimSpace` first, incorrectly resolving
`" gemini-3.1-pro-preview"`, `"gemini-3.1-pro-preview "` and `" auto "`
to real catalog entries with HTTP 200 instead of returning `model_not_found`.

A fail-before endpoint regression covers those padded model and alias
queries plus canonical/uppercase ASCII positive controls. Godex now passes
the raw route identity into the existing ASCII-only catalog comparator and
preserves both the original error message and valid model response bodies.
These are tagged-source and local-catalog parity assertions; dynamic model
catalogs and live Gemini credentials are covered separately by broader
executable differentials.

## Gemini reasoning-effort identifiers use ASCII-only case folding

The tagged 0.438.4 Gemini bridge request policy uses the compiled
`gemini_request_content_string_equals(..., fold_ascii=True)` matcher and
the shared DeepSeek/Gemini reasoning policy's ASCII-only effort classifier.
Only ASCII capital letters are folded; Unicode U+0130 DOTTED CAPITAL I in
`HİGH`, `MİNİMAL`, `MEDİUM`, or `XHİGH` must not be normalized into a
legitimate control. Godex had used Unicode `strings.ToLower` for both the
Responses `reasoning_effort` and nested `reasoning.effort` fields. These
lookalikes were consequently accepted and could change upstream model
thinking, including disabling thoughts or raising the reasoning tier.

Fail-before regressions cover both request shapes, valid ASCII case changes,
unsupported Unicode forms and the actual derived Gemini thinking config.
The scoped fix reuses the already source-aligned Gemini ASCII-only byte
normalizer at both request control validation and generation config
projection. All normal ASCII efforts and provider request bytes remain
unchanged. This is a local source-derived semantic slice; the full official
Gemini 0.438.4 executable differential is a separate no-regression gate,
not a claim that every Unicode effort string was sent to a live provider.

## Gemini/Gemma thinking model family uses ASCII-folded substring matching

The published `provider_constraints.mojo`
`gemini_bridge_request_string_contains` searches model IDs for `gemini-3`,
`gemma-3`, or `gemma-4` using **ASCII-only case folding**. Matched model
families receive the modern `thinkingLevel` shape; unrelated older models
use numeric `thinkingBudget`. Godex had used case-sensitive `strings.Contains`
for this family decision. For example, `GEMINI-3.1-PRO-PREVIEW` with effort
`low` was incorrectly assigned `thinkingBudget: 1024` instead of
`thinkingLevel: LOW`. Mixed-case Gemma 3/4 model names had the same issue.

A fail-before matrix covers lowercase/uppercase/mixed-case Gemini 3 and
Gemma 3/4, a matching substring in a custom model ID, and an unrelated
Gemini 2.5 legacy control. A non-ASCII dotted-I confusable intentionally
remains distinct from a genuine ASCII Gemini model family. Godex now
applies its shared Mojo-compatible ASCII byte fold to the model string
before the family check, retaining the already verified independent
reasoning-effort validation and all existing provider wire shapes for
canonical model IDs. This is a finite tagged source and local policy slice;
the official Gemini executable differential is a broader no-regression
check rather than an assertion that each synthetic mixed-case model was
sent through the provider gateway.

## Gemini native usage accepts full unsigned 64-bit JSON integer values

Prodex 0.438.4 `gemini_response.mojo` `gemini_raw_u64` parses only ASCII
`0`–`9` digits and checks overflow against the **full `uint64` maximum**;
when an input/output count is invalid, it uses zero, and an absent or
invalid `totalTokenCount` falls back to the saturating sum of valid prompt
and candidate tokens. `gemini_responses_usage` from the pinned provider-core
uses that kernel. Godex's `nativeUintPresent` previously used
`json.Number.Int64()`, which discarded valid token counts in the range
`[2^63,2^64−1]`. It also accepted integral-looking Go `float64` values,
where serde's canonical JSON contains a decimal point and is rejected by
the source digit-only parser. Both cases could change displayed usage and
its total-token saturation semantics.

Fail-before tests cover ordinary integers, the maximum `uint64`, values
above signed `int64`, overflow beyond `uint64`, decimal and exponent
literals, negative integers, explicitly zero total, valid large cached
and reasoning token details, and saturating sums. The native usage decoder
now uses `strconv.ParseUint(..., 10, 64)` for JSON numeric literals and
does not project floating-point values into integers. All previously valid
small token counts and the original provider response bytes are retained.
This is a source-pinned local semantics check; the official Gemini
executable differential remains a broader no-regression fixture rather
than a guarantee that every extreme usage number has been tested through
a live API-key service.

## Gemini response media uses first-present branch precedence

Prodex 0.438.4 `gemini_response_media.mojo` first selects the **present**
`inlineData`/`inline_data` member, then `fileData`/`file_data`, then a
`text` data URL. Once a higher-priority member is present, the Mojo kernel
validates its type and required fields but does **not** fall back to a lower
priority field if that value is null or malformed. The separate special
media projector likewise chooses `executableCode` before
`codeExecutionResult` before `videoMetadata` by presence, not by Go map
cast success. Godex formerly cast each optional object before selecting its
branch; a present `inlineData:null` with a valid `fileData` or present
`executableCode:null` with `codeExecutionResult` could emit visible media or
code output that tagged Prodex suppresses.

Fail-before regressions reproduce invalid/null/scalar primary members,
invalid primary members before valid aliases or later media, and the native
completed-response envelope with no fabricated image/tool output. Valid
inline images, valid file URI output, code-execution results, and actual
code text remain positive controls. The scoped fix retains member presence
until the selected branch is processed and moves the media projection into
its own module, bringing the original native-response file below the
repository's 400-line source-size default. The obsolete source-size baseline
entry is removed. These synthetic cases establish source-backed internal
parity, not live provider authentication, while the official Gemini
executable differential remains a broad no-regression gate.

## Gemini executable labels preserve explicit empty strings

The same `gemini_response_media.mojo` `media_put_special_text` retrieves
`executableCode.language` and `codeExecutionResult.outcome` with
`pj_string_field`. Only a *missing or non-string* value triggers the
`"text"` or `"OUTCOME_UNSPECIFIED"` default. An explicit empty string is
valid metadata and is written into the client-visible display text as an
empty label. Godex previously compared the decoded string to `""`,
incorrectly turning explicit empty language/outcome into those defaults.

A fail-before matrix checks empty, missing and wrong-type fields on both
executable code and execution-result branches. The corrected projection
uses string-type presence rather than non-empty value to select defaults;
no unrelated media or executable payload changes. This is a tagged-source
synthetic parity test, with the official Gemini 0.438.4 executable suite
retained as the broader transport no-regression gate.

## Gemini grounding status aliases stop at the first present member

Prodex 0.438.4 `gemini_grounding_source_status` selects the first **present**
field in this order: `urlRetrievalStatus`, `url_retrieval_status`, `status`.
If that field is JSON null, the kernel intentionally emits **no** status
instead of borrowing a lower-priority alias. The previous Godex source
projector skipped null statuses and incorrectly exposed a later
`"status":"FAILED"` even when the provider explicitly supplied
`"urlRetrievalStatus":null`.

A fail-before regression covers null primary/secondary overrides, valid
primary/secondary/tertiary fields, explicit empty strings, non-string
Booleans, and a complete `web_search_call` projection with source arrays.
The scoped fix stops at the first present status field, preserving null
suppression and all genuine non-null values. This source-backed synthetic
metadata check complements the broader official Gemini 0.438.4 binary
transport gate; it is not a claim of live grounding-provider verification.

## Binary-verified Gemini media and grounding precedence

The isolated, release-SHA-pinned Gemini differential now includes four
additional synthetic upstream **native JSON** fixtures passed through the
API-key Chat-compatible transform in both official Prodex 0.438.4 and Godex:
null `inlineData` shadowing `fileData`; null `executableCode` shadowing an
execution result; suppression of executable-code media even with an explicit
empty language; and null grounding status shadowing a lower-priority source
status. Importantly, source-native `gemini_text_from_special_part` can expose
an empty code-language label, but the **Chat-compatible provider transform**
can suppress that code output: the initial independent oracle incorrectly
expected the direct-native result from the API-key executable. The pinned
binary comparison demonstrated suppression on *both* products and the
fixture was corrected instead of modifying a valid transformation.

The first binary comparison also exposed a real Gemini identity mismatch:
Prodex uses `gemini_resp_prodex` as the provider-transform fallback
`responseId` and builds a grounding item `ws_gemini_resp_prodex`, whereas
Godex initially used a random native `resp_gemini_*` identity. The
provider-transform and direct-native identity selectors now have separate
source-matched fallbacks and first-present field/alias behavior. All cases
require one authenticated synthetic upstream call, canonical text and usage,
and a checked downstream semantic shape. Deliberate mutations of image/code
output and source status must fail. No real Gemini keys or live provider data
are used.

## Model endpoint: source exact resolver plus static catalog alias fallback

The tagged Rust provider bridge's `runtime_provider_model_json_for` first
tries Mojo `resolve_catalog_model_exact` across its merged (static + dynamic)
catalog, **then** calls `prodex_provider_core::provider_model_json` if that
exact lookup finds nothing. The latter uses `provider_catalog_entry`, which
applies `model.trim()` before its **static model ID and alias** lookup.
Therefore the bare exact resolver's rejection of `" dynamic-x "` is not
sufficient evidence that the full `/models/{id}` route rejects padded
*static* model IDs or aliases. Earlier Gemini/DeepSeek tests assumed that
all padded queries must return `model_not_found` based only on the first
resolver, without checking the host fallback.

A new isolated `--gemini-models` executable differential compares the full
published 0.438.4 binary to Godex on canonical model IDs, uppercase ASCII,
`default` alias, padded static ID and padded alias, a Unicode confusable and
a missing model. Each query uses the pinned source/manifest/binary, a
synthetic Codex shim, local provider-model routing, no real credentials or
model usage, and independent positive/negative invariants on HTTP status,
canonical model identity, and absence of upstream model completion calls.
Only dynamic models without a static fallback should use the strict exact
identity rule. Results from the executable gate must override an incomplete
source-only inference; the local provider model tests and code are updated
accordingly if the full runtime path differs.


## Prodex 0.438.4 raw HTTP request-target gate precedes model lookup

The initial `--gemini-models` binary differential exposed a critical
host-layer distinction: source `provider_model_json` may trim static model
IDs, but the published local-rewrite proxy invokes
`runtime_request_target.mojo` **before** catalog lookup. Raw target spaces,
UTF-8 non-ASCII and percent-encoded spaces/non-ASCII bytes are rejected with
HTTP **400** `{"error":{"code":"invalid_request_target","message":"request target is invalid"}}`
without any model, profile, provider or broker activity. Earlier Godex
returned 404 from its local model adapter; this matched neither the binary
nor the host's early request safety boundary. The source-only speculation
that padded static models might return 200 is not public HTTP behavior.

The scoped public proxy gate checks the untouched `RequestURI` (falling back
to URL.RequestURI only when missing) using the tagged 8KiB ASCII request
target parser. It rejects raw control/space, fragments, backslashes in paths,
leading or repeated slashes, dot and parent traversal segments, malformed
percent encoding and encoded path separators, dots, controls and non-ASCII.
Safe query escaping and ordinary ASCII paths remain valid. Tests exercise
byte-precise raw/encoded boundaries and the actual HTTP 400 response before
a router or authenticated account is constructed. The pinned
`--gemini-models` executable oracle now expects HTTP 400 for padded and
Unicode-confusable URL paths, 200 for canonical and case-insensitive ASCII
model/alias paths, and 404 for ordinary unknown ASCII model IDs. Every case
must avoid upstream completion requests. This demonstrates observable full
HTTP-path parity independently of the static catalog fallback's library
semantics.

## Provider Content-Type SSE detection uses ASCII-only casing

The published 0.438.4 local-rewrite response dispatcher computes
`content_type.to_ascii_lowercase()` from the original upstream header
before checking for `"text/event-stream"`. Rust's ASCII-only conversion
**does not** turn Unicode U+017F LONG S (`ſ`) into ASCII `s`.
Several Godex Gemini and DeepSeek response paths previously used Unicode
`strings.ToLower`: the header `text/event-ſtream` would therefore be
misclassified as an SSE response, potentially sending a valid buffered
JSON response to the SSE translator, committing the stream too early, or
losing assistant output.

A source-aligned regression matrix verifies canonical lowercase, uppercase
and mixed-case ASCII MIME labels as genuine SSE, but rejects long-s
lookalikes. Complete synthetic buffered Responses are decoded through the
native Gemini and Chat-compatible Gemini adapters and the DeepSeek
Chat-compatible adapter: each retains a normal JSON response and visible
assistant output. The scoped fix shares bytewise ASCII-only media-type
helpers across the affected provider branches, including DeepSeek SSE
precommit inspection and Anthropic Messages translation. Provider headers
and upstream request payloads are unchanged. The broader pinned executable
Gemini/DeepSeek differential is a separate no-regression check, not a
claim of live HTTP header injection into real providers.

## Gemini response Content-Type detection uses ASCII-only case folding

The tagged 0.438.4 `local_rewrite_response_dispatch.rs` and Chat-compatible
response dispatcher classify streamed upstream Gemini content only after
calling Rust `str::to_ascii_lowercase()` and checking for the literal
`text/event-stream`. Godex previously used `strings.ToLower` (Unicode
casefolding) in its native Responses and Chat-compatible bridges, allowing
U+017F LATIN LONG S in `text/event-ſtream` to impersonate the ASCII SSE
MIME type. That could send an otherwise valid buffered Gemini JSON response
through the SSE decoder, losing the response object and client output.

Fail-before tests cover canonical, uppercase, mixed-case ASCII types,
Unicode long-s variants, and end-to-end native and Chat-compatible response
translation. The correction shares one ASCII-only content-type detector,
preserving the genuine stream path while keeping the lookalike response in
buffered JSON mode with its visible assistant text intact. The original
upstream bytes are not rewritten. The source-pinned local regression is
separate from the comprehensive Gemini executable transport oracle, which
serves as a broader no-regression check rather than a claim that this
specific Unicode header traversed a real provider.

## Gemini buffered and streamed responses require complete serde-valid JSON

The official `prodex-provider-core` Gemini buffered response transform uses
`serde_json::from_slice::<Value>` **before** projecting any output. The tagged
Gemini SSE reader similarly relies on the shared `RuntimeProviderSseJsonReader`
whose complete-value decisions use serde_json. Godex previously used a Go
`json.Decoder` for both paths without the stricter serde validity boundaries.
A legitimate `candidates[0]` assistant text could consequently be returned
even when an unrelated field contained an unpaired `\\uD800` surrogate,
invalid UTF-8, an over-depth JSON container, `1e309`, or
`1.7976931348623158e308` (finite to Go, rejected by the pinned default
serde f64 parser). This meant accepting a response that Prodex would reject
or treating its SSE segment as incomplete.

Fail-before tests cover complete buffered native and Chat-compatible output,
as well as direct SSE chunk processing, including genuine paired-surrogate
and valid long-mantissa near-f64-maximum controls. The scoped correction
uses a shared Gemini response decoder with the existing source-matched
Unicode/depth and serde number-token validations before Go map decoding.
It also requires exactly one complete JSON document per event. Invalid
frames no longer create synthesized assistant text; the streaming bridge's
existing provider-stream error handling remains responsible for terminal
failure. No provider request, valid response body, or SSE output is rewritten.
The differential Gemini executable matrix is retained as a separate broad
no-regression gate rather than an unsupported claim that each malformed
synthetic payload was exercised against an authenticated provider.

## DeepSeek built-in model aliases follow exact-ID lookup fallback

The tagged 0.438.4 `runtime_provider_model_json_for` first searches the
live/dynamic list with the Mojo exact ASCII-folded ID resolver; on no match
it calls `prodex_provider_core::provider_model_json`, which resolves static
provider entries and their aliases. Godex previously returned a 404 as soon
as the dynamic ID lookup failed, so built-in DeepSeek aliases such as
`pro`, `auto`, and `flash` were not returned as valid model objects even
though the published catalog maps them to `deepseek-v4-pro` and
`deepseek-v4-flash`.

The scoped correction retains dynamic exact-ID precedence, then resolves
the provider's static model catalog and returns the existing matching
model JSON. Regressions cover aliases, ASCII uppercase/mixed case,
unknown IDs, a Unicode-Kelvin confusable and authenticated-provider
noninterference. An isolated library lookup may trim static catalog
aliases as the source fallback does; the separate raw public HTTP path
gate still rejects padded/encoded invalid URLs **before** model lookup.
This is a source-aligned catalog behavior, with the official executable
DeepSeek matrix retained as a broader transport no-regression gate.

## Gemini request bodies must be serde-valid before any provider translation

The published 0.438.4 Gemini `request_transform.rs` parses the **complete**
Responses request with `serde_json::from_slice::<Value>` before inspecting
any model, content, tool or thinking fields. The corresponding
Chat-compatible request normalizer must not silently turn an invalid raw
JSON input into a valid re-serialized one. Godex previously relied on Go's
more permissive `encoding/json` for both paths. An unrelated malformed
Unicode surrogate, invalid UTF-8, deeply nested value or finite-to-Go but
serde-invalid f64 literal could be ignored by the native translator or
converted to replacement text/f64 by the Chat-compatible normalizer, then
forwarded as an apparently valid request with the original user message.

Fail-before tests exercise both translator variants with authentic model and
user input plus unrelated invalid values. Paired Unicode escapes and the
published long-mantissa near-f64-maximum positive control remain accepted;
lone surrogates, bad UTF-8, serde-only numeric overflow, `1e309` and
excess nesting are rejected **before** constructing a provider request.
The scoped fix centralizes the same tagged Unicode, container-depth and
number-token checks already used by the corrected Gemini response decoder,
while retaining the original request model/type validations and error
behavior for normal JSON. This semantic slice is established by source and
local tests; the pinned executable Gemini oracle provides an additional
transport no-regression check, not real-provider testing of these malformed
fixtures.

## DeepSeek Responses request translation requires complete serde-valid JSON

The official 0.438.4 `prodex-provider-core` DeepSeek request transformer
calls `serde_json::from_slice::<Value>` on the complete OpenAI Responses
input before inspecting its user message, tool calls, reasoning controls or
model identifier. Godex's `parseResponsesObjectWithoutValidation` previously
accepted all JSON that Go's `encoding/json` could decode, even when serde
would reject an otherwise unrelated unpaired Unicode surrogate, invalid
UTF-8, excessive container nesting, `1e309`, or the near-f64-maximum
`1.7976931348623158e308`. The invalid extra member was then silently
lost while Godex forwarded a valid-looking user request. The same decoder
was used to extract context/history, allowing the malformed request to
supply replayable user/tool content.

Fail-before tests cover the actual translated upstream request as well as
the lower-level history parser. A genuine paired surrogate and the pinned
serde-valid longer-mantissa float remain positive controls. The scoped
change runs the project's shared, source-matched serde Unicode/depth and
number-token validators before the usual Go decoder, preserving existing
model/policy checks and ordinary request bytes. No invalid provider request
is emitted and no malformed history is trusted. This is a local semantic
regression from the published Rust source; the official DeepSeek executable
matrix serves as an independent transport no-regression gate, not a claim
that each malformed payload reached a live provider.

## DeepSeek buffered and streamed response values require whole serde-valid JSON

The published `prodex-provider-core` DeepSeek 0.438.4
`response_transform.rs` calls `serde_json::from_slice::<Value>` before
projecting a buffered ChatCompletion into a Responses output. Its SSE
`stream.rs` likewise parses each complete `data:` chunk as a serde JSON
value before extracting deltas and tool calls. Godex previously used Go's
`encoding/json` without checking serde-specific Unicode, depth and numeric
boundaries on these response paths. Consequently a valid assistant message
could be emitted even when an unrelated member contained a lone surrogate,
invalid UTF-8, nested arrays beyond the serde recursion limit, or a
near-f64-maximum decimal that the tagged serde decoder rejects.

Fail-before tests cover buffered DeepSeek completion output, direct SSE
chunk projection and an actual translated live stream with a later `[DONE]`.
The scoped fix rejects those invalid complete values using the shared
source-matched serde Unicode/depth and numeric validators **before** emitting
any user-visible assistant text. A malformed stream now produces its
existing `provider_stream_error` terminal event, not a fabricated successful
completion; paired Unicode escapes and the serde-valid longer-mantissa
numeric control still complete normally. The helper for SSE validation is
separate from the 486-line stream-state file, with the source-size baseline
reduced to match the shorter implementation. Broader real-provider parity
and exact Rust error diagnostic strings are not inferred from the local
semantic tests or the official 30-case transport differential.

## DeepSeek native Anthropic Messages request translation enforces serde JSON

The published 0.438.4 DeepSeek native Messages route calls the shared
`translate_openai_chat_request_to_anthropic_messages` provider-core bridge.
`translators/anthropic/messages.rs` first deserializes its entire
ChatCompletion-shaped input with `serde_json::from_slice::<Value>`, then
passes the typed Value to Mojo request translation. Godex previously decoded
this path directly with `encoding/json`, which accepts unpaired surrogate
escapes and invalid UTF-8 and can also accept serde-invalid near-maximum
floating-point numbers or excessive nested JSON. An ignored malformed
member of an otherwise ordinary `messages[0]` could disappear and leave a
provider request containing apparently valid user text.

Fail-before regressions exercise both `parseAnthropicChatRequest` and the
actual native Messages request translator. They confirm invalid Unicode,
UTF-8, depth, `1e309` and the pinned serde f64 boundary are rejected before
upstream request construction. Legitimate user messages, paired Unicode,
and the valid longer-mantissa float remain accepted. The scoped correction
reuses the source-matched whole-document serde Unicode/depth and numeric
token checks already used by the DeepSeek Responses translator. It preserves
normal tool, policy, model and payload conversion behavior, without sending
real requests or accessing external credentials. This semantic test is
separate from the broader executable DeepSeek oracle.

## Gemini video metadata text must embed canonical serde JSON

Tagged `gemini_response_media.mojo::media_put_special_text` emits the
`videoMetadata` value as `js_raw_view(tree, metadata)` inside a descriptive
assistant text item. The parsed tree is built from the published
serde's canonical value representation. Godex previously used
`encoding/json.Marshal(videoMetadata)` instead, which HTML-escapes `<`, `>`
and `&`, escapes Unicode U+2028/U+2029, and preserves noncanonical numeric
spellings (for example `1.00000000` instead of `1.0`, or `1e-5` instead of
`0.00001`). The resulting visible text differed despite equal original
metadata. A pinned offline Rust serde_json probe confirmed the exact UTF-8
and HTML behavior, and fail-before Go regressions reproduced the mismatch
inside both a media text item and a complete Gemini Responses projection.

A shared `jsonutil.MarshalSerdeValue04384` writer now emits canonical
serde-like JSON for decoded values: sorted UTF-8 object keys, serde string
escaping without optional Go HTML/JavaScript escapes, and the already
verified default-serde f64 parsing/formatting (including negative zero and
u64 bounds). It rejects invalid UTF-8, non-finite numbers and excessive
nesting. The scoped Gemini video metadata projection uses this writer
without changing the original provider payload, media precedence, or valid
image/tool output. Unit tests compare canonical text for nested maps,
controls, Unicode separators, numeric spellings and full assistant output.

This change has **source- and dependency-pinned local coverage**. The
existing Gemini 0.438.4 executable differential tests Chat-compatible Super
fixtures; it is a broad no-regression gate, not a dedicated direct
GenerateContent `videoMetadata` binary comparison. No live Gemini account
or credentials were accessed.

## Gemini response thought-signature selection respects first PRESENT source

Tagged `prodex-provider-core` Gemini `response_tool_calls.rs` supplies the
response part's **camelCase** `thoughtSignature` and then the nested
`functionCall.thoughtSignature` to the compiled
`prodex_gemini_signature_choice_v1` policy. The Mojo selector stops on the
**first present** candidate, even if it is null, wrong-type or only
whitespace; those invalid values suppress the signature instead of allowing
a later field to override them. The source does **not** offer snake_case
`thought_signature` as an alternative response field. Godex previously
iterated both camelCase and snake_case fields and skipped empty or malformed
values, so an explicit null/blank response-part signature incorrectly
borrowed a nested signature; snake_case-only content also created an
unrecognized continuation token.

Fail-before regressions cover positive part/nested signatures; null, empty,
blank and non-string primary values; unsupported snake_case variants;
Unicode whitespace; and precedence within the complete Gemini Responses
tool-call history. The scoped correction stops at the first present
camelCase field, emitting only a nonblank string. It preserves canonical
signatures and the original upstream JSON, preventing an upstream tool
response from gaining a fabricated model signature. The rule is verified
against the tagged Rust and Mojo source and synthetic local tests; the
broader Gemini 0.438.4 executable differential is a no-regression gate,
not a dedicated live-provider signature-token experiment.

## Canonical Gemini function-call arguments and exact RTK tool-name matching

In the tagged `prodex-provider-core` Gemini `response_tool_calls.rs`, the
`functionCall.args` serde JSON value is serialized with
`serde_json::to_string` **before** being supplied as the text of a function
call. Godex previously used `encoding/json.Marshal`, which escaped HTML,
U+2028/U+2029, and preserved raw decimal number spellings. An otherwise
identical tool call could therefore carry different `arguments` text in the
continuation history (`1e-5` rather than the canonical `0.00001`, for
example). Fail-before regressions cover direct and complete Responses tool
items, Unicode, HTML, sorted nested arguments, and scientific floats. The
Gemini tool-call projector now shares the serde-compatible value writer
already validated for response video metadata.

A second mismatch appeared in RTK shell wrapping. The published Gemini
`response_tool_calls/rtk.rs` applies RTK **only** to the original flat tool
name `shell` or `exec_command`, not to `exec` and not to a shortened suffix
from a namespaced `mcp__...__shell` or `namespace.shell`. Godex previously
passed the display-shortened tool name to a generic wrapper that accepted
`exec`, causing unrelated tools to be silently rewritten. The adapter now
checks the full original name first; qualified aliases remain unchanged.
Rust's shared `tool_args::wrap_json_string_arg_with` also serializes a
rewritten command with `serde_json::to_string`, so Godex's general RTK
argument writer now validates complete serde JSON and uses the same
canonical serializer when it actually changes `cmd` or `command`.

Fail-before tests exercise canonical `shell`/`exec_command`, unrelated
`exec` and namespaced names, HTML/Unicode/numeric metadata during wrapping,
no-change source preservation, and serde-invalid JSON passthrough. This is
source/dependency-aligned local parity plus the official Gemini and
DeepSeek executable regression gates, not a claim that every synthetic
RTK/media variant was run through a live API-key provider.

## Gemini 3 tool-description and parameter enrichment

Prodex 0.438.4 includes a new compiled `gemini_tool_descriptions.mojo` policy.
The production `gemini_provider_core_function_tools_from_chat` builds normal
`functionDeclarations`, then invokes
`gemini_provider_core_apply_gemini3_tool_declaration_overrides(model, ... )`
before sending the GenerateContent request. On model IDs containing
`gemini-3` (case-insensitive ASCII) or the `auto` selector, declarations
such as `read_file`, `read_many_files`, `grep`, `exec_command`, `write_file`
and `apply_patch` are enriched with fixed source-defined instructions. The
published name policy trims Unicode whitespace, strips a dotted namespace,
converts ASCII uppercase and hyphens, then separately recognizes a narrow
set of MCP `__` suffix aliases. Qualified `mcp__repo__read_file` may match,
while `mcp__repo__read` intentionally does **not** inherit the short alias.

Godex's native Gemini adapter previously forwarded converted function tools
without any of these overrides. Fail-before tests reproduced missing tool
instructions and absent `start_line`, `end_line`, `grep.pattern` and
`apply_patch.properties.input.description` guidance. A scoped source-aligned
Gemini 3 declaration enrichment phase now injects those descriptions,
preserves pre-existing targeted property descriptions (even explicit null),
replaces only the patch input description as the Mojo policy requires, and
leaves unrelated parameters, original request schema and older Gemini 2.5
models unchanged. Tests cover canonical and namespaced aliases, ASCII case,
non-ASCII near-matches, `AUTO`, patch schema preservation, and complete
Responses-to-GenerateContent translation.

This is a finite source-derived Gemini 3 request-shape parity result. The
released two-executable Gemini differential is retained as a no-regression
gate; it does not certify every schema corner (notably compiled Mojo panic
behavior on invalid nested patch schema nodes) or a live Gemini API call.

## Gemini streamed function-call JSON uses canonical serde serialization

Prodex 0.438.4's `gemini_provider_core_stream_function_call_delta` serializes
upstream `serde_json::Value` to canonical JSON before the Mojo
`GEMINI_STREAM_FUNCTION_CALL_DELTA` projector selects the `args` member.
The projector distinguishes a missing `args` (default `"{}"`) from a
present `args:null` (literal `"null"`). The resulting argument string is
used both by the SSE `response.function_call_arguments.delta` and the
completed function-call history. The published Rust SSE event path also
serializes JSON through serde, without Go's optional HTML/Unicode escapes.

Godex's streaming adapter had a separate `json.Marshal` path even after
buffered Gemini function calls were corrected. It escaped `<>&` and
U+2028/U+2029, preserved original `1e-5` and `1.000000` spellings, and
collapsed an explicit null argument to `{}`. Fail-before tests reproduce
all three mismatches, including the exact original SSE event bytes and the
completed `response.output` tool item. The scoped correction shares the
already pinned serde-compatible writer for function-call arguments and
emitted SSE JSON, defaults to `{}` only when the `args` key is **absent**,
and preserves explicit JSON null and normal Unicode. Other stream IDs,
provider selection, and unrelated tool-call metadata remain unchanged.

This source/dependency-pinned slice covers synthetic streamed tool calls.
The official 0.438.4 Gemini executable differential remains an independent
regression gate, not a claim of live-provider coverage for every nested
argument/value type.

## Buffered Gemini arguments distinguish absent members from JSON null

The tagged `gemini_response_tool_call_item_with_call_id` and Chat-compatible
tool-call builder use `get("args").cloned().unwrap_or_else(|| json!({}))`.
Only a missing member defaults to `{}`; a present null serializes as `null`.
Godex's buffered projector instead treated both cases as absent, including
the special `tool_search` and `apply_patch` output paths. The correction
uses the map lookup's presence flag and preserves every present JSON value.

An overlay of the original `cfa3d1235` implementation fails the explicit-null
unit, HTTP envelope, and special-tool regressions. Missing arguments, empty
objects, arrays, booleans, empty strings, and zero remain positive controls.
HTTP translation tests cover wrapped/unwrapped responses and both direct
and Chat-compatible call-ID modes. All 462 Gemini package tests pass, and
`rtk go test ./internal/gateway/gemini ./tools/differential -count=1` passes
664 tests. `rtk proxy make verify` passes formatting, source size, TUI
framework, cross-build/test compilation, installer, vet, full race and build
checks with the four new executable fixtures included.

The `--gemini` executable oracle now includes `buffered-args-null`,
`buffered-args-absent`, `buffered-args-empty`, and
`buffered-args-wrapped-null`. It checks exact tool shape and argument text,
HTTP status, identity, usage, provider request and attempt count. Its
negative controls reject changed arguments or tool identity even if both
products return the same invalid output. Candidate executable results must
come from a clean committed build; local tests alone do not prove them.

The official 21-case Gemini executable run on clean commit
`880291b80d4988ebc9086937241e9f06f0ef2c42` passed every case and mutation
sentinel. The same extended oracle on clean baseline `cfa3d1235` failed
only `buffered-args-null` and `buffered-args-wrapped-null`; the other 19
cases passed. This is executable evidence for that finite presence contract,
not certification of all Gemini or overall provider parity.

## Buffered Gemini emits canonical serde JSON through the final envelope

The published buffered Gemini transformer serializes its Responses value
with `serde_json::to_vec`. Its custom `apply_patch` fallback independently
serializes unrecognized arguments with `Value::to_string`. Godex still used
`encoding/json.Marshal` for that fallback, the native response envelope,
and the final API-key identity rewrite. This escaped HTML and U+2028/U+2029
and retained noncanonical decimal spellings even after function-call
argument strings and SSE events had been corrected.

Fail-before tests on `880291b` reproduce those differences in both direct
and API-key buffered output, including canonical metadata floats, negative
zero, and custom-tool fallback text. ASCII, null, empty-object and explicit
patch aliases remain positive controls. The three emission sites now reuse
the existing serde value writer. The identity rewrite retains decoded
number tokens, generates the same UUIDv7 namespace, and preserves headers;
no new dependency, helper package, retry policy, or persistence format is
introduced. `rtk go test ./internal/gateway/gemini ./tools/differential -count=1`
passes 679 tests, `rtk go test ./...` passes 8,635 tests across 81
packages, and `rtk proxy make verify` passes its full local gate.

Two added executable cases, `buffered-args-canonical-wire` and
`buffered-args-patch-fallback`, compare every original client response byte
except the independently validated generated response UUID. They also retain
the ordinary response, request, attempt, identity and usage invariants.
Mutation sentinels distinguish semantically equal HTML/Unicode escapes and
decimal spellings, preventing a semantic JSON comparison from concealing
wire differences. Clean candidate executable validation follows the commit.

The 23-case pinned executable oracle fails against clean baseline `880291b`
only in these two new cases: the canonical-wire case exposes byte differences
without a semantic JSON mismatch, while the custom-tool fallback exposes
both byte and argument-text differences. The earlier 21 cases still pass.

Clean commit `7899defb40bcedc39390bea0c0b25b8bdb9a153d` passed all 23
Gemini executable cases, including both exact-byte cases and every mutation
sentinel, before its fast-forward push to `origin/main`.

## Gemini envelope presence and complete serde acquisition

Tagged `gemini_put_normalized_response_envelope` selects any present
`response` value, including null, arrays and scalars. Only object values
accept the optional string trace-ID override. Godex previously unwrapped
only objects, allowing candidates, model, usage and identity on the outer
envelope to surface when the selected inner value contained no response
fields. A present non-object envelope now exposes no native fields. Missing
envelopes and valid inner objects retain their visible content; outer
`choices` cannot override an explicitly present response envelope.

The tagged Rust acquisition is `serde_json::from_slice::<Value>`, not an
object-only decoder. The shared Gemini response reader now validates the
entire serde document before accepting any JSON value; valid non-objects
have no candidate/metadata fields. The Chat-compatible fallback uses this
same validation before examining `choices`, closing a path that previously
ignored invalid Unicode/UTF-8 or overflowing numbers in unrelated members.
API-key empty responses retain the provider transform's empty `usage` and
`metadata` objects. The downstream identity rewriter still requires an
object, so these accepted upstream scalars do not relax that internal
output boundary.

Fail-before tests exercise outer-field leakage, valid null/array/scalar
documents, non-object SSE values, outer-choices precedence, missing/default
metadata, and invalid ignored Chat fallback members. Paired Unicode and the
pinned valid longer-mantissa number remain controls. All changes stay in
`gateway/gemini`; no application policy, persistence, retry contract, helper
package or dependency is added. Focused Gemini/oracle coverage passes 683
tests. The extended executable oracle adds five envelope/value cases with
independent exact empty-response, identity, usage and mutation assertions.

The executable baseline run against clean `7899def` failed at
`buffered-value-array`: Godex rejected the valid array and made four
upstream attempts before the synthetic client deadline expired. This is a
recorded FAIL, not a completed 28-case comparison or a timeout counted as
PASS. Local regressions independently prove the parser/envelope mismatch;
clean candidate executable validation follows the commit. No live provider
account or credential is involved.

`rtk go test ./...` passes 8,639 tests across 81 packages. The final
`rtk proxy make verify` passes formatting, source size, framework,
cross-build/test compilation, installers, vet, full race and build checks.

## API-key Gemini typed fallback identity respects original string IDs

The preceding clean envelope commit
`b81f3e782aaa626fa47e1daecd8b16227a26cede` passed all 28 Gemini and all 30
DeepSeek executable scenarios, including negative controls, before its
fast-forward push to `origin/main`.

Prodex 0.438.4's `runtime_deepseek_chat_buffered_response_parts` first
accepts the Gemini provider transform, then calls
`runtime_deepseek_apply_typed_fallback_ids` with the original upstream value.
That function generates a `resp_deepseek_` UUIDv7 only when the original
top-level `id` is not a string. An explicit string, including an empty one,
preserves the provider transform's identity selected from `responseId` or
`traceId`; it does not make the original `id` a response-ID alias. Godex
previously generated a UUID unconditionally and discarded that distinction.

Fail-before HTTP regressions on `b81f3e7` cover string/empty original IDs,
with missing/null/numeric IDs as UUID-generation controls. Both direct and
wrapped native payloads are exercised through the API-key response path.
The existing identity rewriter now receives the original source map and
uses the same type-presence decision before UUID generation. It retains
timestamp omission, canonical JSON output, header handling, and strict
translated-object validation. Existing direct helper tests explicitly use
an absent original source and retain their full UUID assertions. The
protocol decision stays private to `gateway/gemini`; no account ownership,
continuation store, shared helper, or persistence contract changes.

The executable oracle adds four source-ID cases: normal string, empty
string, wrapped string and null. Their independent guards require the exact
preserved provider ID or a valid generated UUIDv7 as appropriate, while
retaining request, output, usage, status and attempt comparisons. Mutation
tests reject borrowing the original source ID or emitting a native
timestamp. Clean candidate executable validation follows local gates and
commit; source-derived unit coverage alone is not binary evidence.

`rtk go test ./internal/gateway/gemini ./tools/differential -count=1` passes
685 tests; `rtk go test ./...` passes 8,641 across 81 packages.
`rtk proxy make verify` passes its full local gate, including race tests.

The source-ID oracle failed against clean `b81f3e7` only for the three
string-ID cases; the other 29 cases passed. Clean commit
`40826d2a6e297f89403e7f0f8e580e4fed9d3010` then passed all 32 Gemini
executable cases before its fast-forward push to `origin/main`.

## Gemini tool names and call identities retain raw string semantics

Tagged `response_tool_calls.rs` uses `Value::as_str` for buffered function
names and IDs, preserving explicit empty or whitespace strings. Its Mojo
`gemini_tool_name_split` recognizes a nonblank double-hyphen split, scanning
backward for a valid separator, then the last valid double underscore in an
exact `mcp__` name. It preserves raw prefix/suffix whitespace and does not
recognize dots. Godex instead trimmed names, treated empty strings as
missing, split dotted names, rejected an empty MCP provider segment, and
did not recover an earlier valid separator after a trailing one.

The corrected local splitter and buffered type-presence checks follow the
tagged rules. Completed native SSE tools now use the same splitter, keeping
namespace and explicit empty-name semantics in the final stream history.
Stream call IDs keep their separate source-defined nonblank selection.
No tool is executed by these transformations; account selection, retries,
request bytes, and the independent generic Chat-compatible splitter remain
unchanged. The implementation stays private to `gateway/gemini` and creates
no shared helper, interface, dependency, configuration or background work.

Fail-before regressions cover ordinary names, valid namespace controls,
dots, Unicode/padded strings, empty names and IDs, empty MCP provider
segments, trailing separators and full completed SSE output. The 702 focused
Gemini/oracle tests and 8,658 tests across 81 packages pass; the final
`rtk proxy make verify` passes formatting, source size, framework, cross
build/test compilation, installers, vet, full race and build checks.
Five new executable cases independently assert the exact buffered name,
call ID, namespace and arguments, alongside existing request/response,
status, usage and attempt checks. Dedicated mutation tests reject changed
identity, namespace or arguments. Native streaming assertions remain
source-aligned local coverage; API-key executable cases do not certify
every native SSE lifecycle behavior.

The 37-case executable baseline run on clean `40826d2` failed exactly the
five new buffered tool-identity cases; all earlier 32 cases passed. The
independent WebSocket continuation executable oracle on that same commit
also passed 56 upstream attempts, with hard-owner, precommit retry,
no-replay, restart and quota-probe checks. That WebSocket result does not
replace the separate transport/deadline/Compact-lineage release phases.

## Gemini custom apply_patch converts unified diffs

The tagged `response_tool_calls_apply_patch.rs` first extracts an explicit
patch alias or assembles an edit, then normalizes an apply-patch block or
calls the compiled `gemini_unified_diff_to_apply_patch` policy. Godex
previously handled only the native block form and edit arguments; unified
diff strings were returned unchanged. It also normalized CRLF in literal
non-patch strings even though the tagged no-match path preserves the input.

The new private `gateway/gemini/native_patch_diff.go` follows the published
section/header and hunk rules: git or plain unified sections; quoted paths
and `/dev/null`; Add/Delete/Update/Move records; hunk context without line
counts; and explicit end-of-file markers. Incomplete or unchanged sections
do not create fabricated edits. Native blocks still take precedence, now
recognizing the same Unicode whitespace before a marker; unmatched literal
input retains its original bytes. The parser only transforms provider tool
input and never touches the filesystem or executes a patch.

Fail-before regressions cover update/add/delete/rename/EOF output, malformed
hunk labels, incomplete headers, unchanged sections, native-block controls,
Unicode markers, LF/CRLF and the complete API-key custom-tool envelope.
The implementation is a local protocol parser with no dependency, exported
API, helper package, application policy or durable-state change. Focused
Gemini/oracle coverage passes 719 tests, and `rtk go test ./...` passes
8,675 tests across 81 packages. Three executable fixtures independently
assert single-file conversion, a mixed update/move/add/delete diff, and
literal CRLF preservation; negative controls reject changed patch bytes.

These finite fixtures do not certify source panic behavior for oversized
native custom-tool arguments or every malformed diff/header combination.
The existing bounded upstream read remains in force. Full local and clean
candidate executable validation are required before this change is pushed.

`rtk proxy make verify` passes the complete local gate, including full race
tests and cross-build/test compilation. The preceding clean commit
`b512e8fdadb1b8bdf8784e750a89af6633a0d86d` passed all 37 Gemini executable
cases and mutation controls before its fast-forward push to `origin/main`.

The extended 40-case executable baseline on clean `b512e8f` failed only the
three new patch cases (single-file unified diff, mixed file operations,
and literal CRLF); its earlier 37 cases passed. Additional source/binary
oracles on clean `40826d2` passed both strict WebSocket Compact turn lineage
and HTTP conflicted-session/durable-ownership checks across both products.

## Native Gemini stream completion retains tool kinds and RTK policy

The tagged `gemini_provider_core_stream_completed_tool_call_item` re-parses
canonical arguments, applies RTK only to the supported original flat names,
and emits parsed `tool_search_call` or normalized `custom_tool_call` items
for special tools. Ordinary function-argument SSE deltas keep their upstream
argument text. Godex instead stored every completed tool as a generic
`function_call` without shell wrapping, even after buffered tool handling
was corrected.

The native stream now reuses the existing private Gemini tool projector
for its completed output. It supplies the stream's already selected call
ID, name, argument value and signature, preserving the distinct nonblank
stream-ID fallback rule. Ordinary argument deltas and their IDs remain unchanged;
only the completed item gets RTK or special-tool shaping. This deletes a
parallel projection implementation and adds no helper, interface, adapter,
dependency, configuration, process, persistence or retry behavior.

The runtime's preceding `gemini_provider_core_canonical_output_tool_name`
step also maps the normalized `run_shell_command` alias to `exec_command`.
The stream now applies the same Unicode outer trim, final dotted suffix,
ASCII fold and hyphen normalization solely for that alias. MCP suffixes,
leading suffix whitespace, and non-ASCII near-matches do not gain an alias.
Buffered provider-transform names keep their independent raw-name contract.

Fail-before tests drive actual HTTP SSE translation through stream
completion and the DONE sentinel. They assert exact raw deltas, one terminal
completion, matching ordinary delta/completed IDs, parsed search arguments including
null, normalized patch input, special-tool signature omission, and shell
wrapping. Plain `exec`, dotted/MCP names, ordinary null arguments and explicit
literal patches are negative controls; explicit/empty stream call IDs test
the separate fallback rule. Separate fail-before tests cover plain,
uppercase and dotted shell aliases plus MCP/Unicode near-matches. Focused
Gemini/oracle coverage passes 731 tests.

This is source-aligned native SSE coverage. The official API-key Gemini
differential remains a separate no-regression gate and does not establish
native OAuth transport, multi-chunk tool assembly, or full native SSE
item-added/item-done lifecycle parity. Those are independent audit areas.

The preceding clean patch commit
`e02a676b9dbe4aa9d360e0e1c2ce7d69a9e2d808` passed all 40 Gemini executable
cases and mutation controls before its fast-forward push to `origin/main`.

`rtk go test ./...` passes 8,687 tests across 81 packages. The final
`rtk proxy make verify` passes formatting, source size, framework, cross
build/test compilation, installers, vet, full race and build checks.

## Native special tools suppress generic function-argument deltas

The runtime `observe_function_call` invokes
`gemini_provider_core_stream_should_emit_function_call_arguments_delta`
after selecting a canonical output name. Its pinned Mojo
`GEMINI_STREAM_SHOULD_EMIT_ARGUMENTS_DELTA` returns false for exact
`tool_search` and `apply_patch`, while ordinary and namespaced function
names retain their argument deltas. Godex's native stream previously emitted
generic argument events for every tool, including these special kinds.

A local gate now retains the completed special-tool item while omitting
its generic function delta. Ordinary functions keep the exact canonical
argument bytes and stable delta/completed identity. The earlier completion
test assumed all tools emitted a function delta; tracing the complete
runtime pipeline disproved that assumption. The corrected regression
asserts zero such events for both special tools, one unchanged delta for
ordinary functions, and the correct final item for explicit/fallback IDs.
It fails against the preceding implementation only on special-tool cases.

This private Gemini protocol fix adds no helper, dependency, state format,
configuration or orchestration. Native SSE item-added/item-done lifecycle
and repeated-call assembly remain separate audit areas; the finite local
tests and public API-key oracle do not certify full native stream parity.

`rtk go test ./internal/gateway/gemini ./tools/differential -count=1` passes
731 tests and `rtk go test ./...` passes 8,687 across 81 packages. The final
`rtk proxy make verify` passes its full formatting, source-size, framework,
cross-build/test compilation, installer, vet, race and build gate.

On clean stream-projection commit
`f342b7e510a10bcd8ed85a29680e7e250747c177`, official executable checks pass:
40 Gemini cases, all 30 DeepSeek scenarios, 56 WebSocket model attempts,
strict WebSocket Compact lineage, and HTTP conflicted-session/durable
ownership. These gates remain finite regression evidence; full native SSE
and every release phase are not inferred from them.

## 2026-10-21 verified checkpoint

Eight focused production commits have been pushed by fast-forward to
`origin/main`, ending at `08523c87d2a4418499e49b7a8f9bac5aad83fbd0`:

| Commit | Closed behavior gap | Gemini executable cases |
| --- | --- | --- |
| `880291b80` | Missing versus null buffered arguments | 21 PASS |
| `7899defb4` | Canonical buffered JSON and fallback argument text | 23 PASS |
| `b81f3e782` | Envelope presence and complete serde acquisition | 28 PASS |
| `40826d2a6` | Original string-ID typed fallback | 32 PASS |
| `b512e8fda` | Raw tool identity and namespace splitting | 37 PASS |
| `e02a676b9` | Unified patch conversion and literal CRLF | 40 PASS |
| `f342b7e51` | Completed native tool kinds, RTK and shell aliases | 40 PASS |
| `08523c87d` | Suppress special-tool function deltas | 40 PASS |

Each executable report validates the pinned 0.438.4 source, published binary
and manifest digests, and its exact clean candidate VCS revision. Newly
added buffered fixtures fail against the preceding clean baseline; their
positive controls retain the earlier passing cases. Native stream-only
changes additionally have failing-before HTTP/SSE regressions; the public
API-key executable matrix remains an independent no-regression gate.

Final exact-commit validation at `08523c87d`:

- `rtk go test ./internal/gateway/gemini ./tools/differential -count=1`:
  PASS, 731 tests.
- `rtk go test ./...`: PASS, 8,687 tests across 81 packages.
- `rtk proxy make verify`: PASS, including formatting, source size, TUI
  framework, cross-build/test compilation, installers, vet, full race and
  build. Log: `/tmp/gdx-04384-stream-delta-verify.log`.
- `rtk go build -trimpath -o /tmp/gdx-04384-stream-delta-candidate-godex ./cmd/godex`
  and the matching `./tools/differential` build: PASS. Embedded VCS metadata
  identifies `08523c87d2a4418499e49b7a8f9bac5aad83fbd0`, modified=false.
- The exact-commit `--gemini` invocation: PASS, 40 cases and mutation
  controls. Report: `/tmp/gdx-04384-stream-delta-candidate-oracle.json`.

The last invocation was:

```bash
rtk proxy /tmp/gdx-04384-stream-delta-candidate-differential --prodex /tmp/godex-release-parity-04384-20261021/prodex-x86_64-unknown-linux-gnu --godex /tmp/gdx-04384-stream-delta-candidate-godex --prodex-source /tmp/godex-release-parity-04384-20261021/prodex-source --godex-source /tmp/gdx-04384-stream-delta-work --godex-commit 08523c87d2a4418499e49b7a8f9bac5aad83fbd0 --gemini
```

Broader exact-source executable gates at preceding stream commit `f342b7e51`
also passed all 30 DeepSeek scenarios, 56 WebSocket upstream attempts,
strict WebSocket Compact lineage, and HTTP conflicted-session/durable
ownership. Their reports are respectively
`/tmp/gdx-04384-stream-tools-deepseek-oracle.json`,
`/tmp/gdx-04384-stream-tools-websocket.log`,
`/tmp/gdx-04384-stream-tools-ws-compact.log`, and
`/tmp/gdx-04384-stream-tools-http-ownership.log`. They were not rerun at
`08523c87d` and must not be represented as results for that later commit.

Full 1:1 parity remains unproven. Next source-confirmed audit areas include
native streamed call indexing/replacement across chunks, native
item-added/item-done lifecycle and fallback-ID formats. The published
`gemini_sse_tool_calls.rs` tracks explicit call IDs and part positions,
replaces current arguments, and completes calls once; Godex still appends
one stored call per observed chunk. Oversized custom-tool panic/error
boundaries, every release phase, live provider paths, and native macOS or
Windows execution also remain unverified. No live credentials or private
conversation data were used; no tag or release was created. CI status was
neither awaited nor claimed. The parity goal remains active.

## Native Gemini repeated tool snapshots retain their slot and identity

Tagged runtime `gemini_sse_tool_calls.rs::observe_function_call` asks the
compiled `gemini_tool_call_index` policy for a slot, then replaces that
slot's arguments with the current snapshot. A known explicit ID wins even
if the part position changes. Anonymous calls reuse a compatible position,
then the earliest anonymous slot with the same canonical name. New explicit
IDs never take over an anonymous or differently owned call. Otherwise an
unused part position wins, falling back to the first free slot starting at
the number of existing slots. Final output follows the Rust BTreeMap order.

Godex previously appended a completed item per observed chunk and generated
a fresh UUIDv4 for every missing ID, so repeated partial arguments could
become several tool executions. Fail-before HTTP SSE tests reproduce these
errors and verify positive/negative controls for distinct explicit IDs,
anonymous ownership, shifted/sparse part positions, canonical shell aliases,
null/absent arguments and retained thought signatures. The private stream
record now keeps the selected slot and UUIDv7; later snapshots replace the
item without concatenating argument JSON or borrowing another tool's ID.
All ordinary raw argument deltas remain visible, and special-tool delta
suppression is unchanged. Completed streams ignore later chunks.

An unmodified copy of the pinned `provider_constraints.mojo` was compiled
with local Mojo 1.1.0 using:

```bash
rtk proxy mojo build --emit shared-lib -O0 -I mojo/prodex_core -o /tmp/gdx-04384-tool-index-policy.so mojo/prodex_core/provider_constraints.mojo
```

Calling its exported slot-selection ABI produced 48 deterministic goldens
in `native_stream_index_04384.json`, including sparse occupied-index
collisions. Godex matches every golden. This is compiled tagged-source
evidence, separate from the released CLI executable differential.

The stream's retained tool snapshots are bounded by the existing 8 MiB
buffered-response budget, and replacement releases the old snapshot's
budget. Reaching that bound returns an explicit stream error without adding
the rejected item. The tagged source does not establish the same aggregate
tool-state bound, so requests exceeding it remain a documented safety
boundary rather than a claim of byte-for-byte parity. No new option,
dependency, exported type, shared helper, persistence or account/retry policy
is introduced. Protocol state stays in `gateway/gemini`; the source-size
split gives tool acquisition/indexing its own cohesive private file.

`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 747 tests; `rtk go test ./...` passes 8,703 across 81 packages. Native tool
item-added/item-done lifecycle and behavior after a nonterminal finish
reason remain separate audit work; this slice does not certify complete
native streaming or live OAuth transport parity.

The final `rtk proxy make verify` passes formatting, source size, framework,
cross-build/test compilation, installers, vet, the full race suite and build.
Log: `/tmp/gdx-04384-stream-merge-final-verify.log`. Clean candidate binary
differential validation follows the commit.

Clean commit `27d31f0cd92b03dd278f09b33cc257960bc1cf1c` passed all 40
official Gemini executable cases and mutation controls before its
fast-forward push to `origin/main`. Native repeated-call semantics remain
covered by the tagged-source kernel and local HTTP SSE regressions.

## Native Gemini tool lifecycle and finish barriers

Tagged runtime `observe_function_call` emits an argument-free
`response.output_item.added` once for an ordinary tool, followed by raw
argument deltas. `complete_tool_call_events` emits one done event per
unfinished slot, in slot order. Search and patch tools omit both the generic
added event and argument deltas; their parsed/normalized item still gets a
done event. A source-bound explicit ID may revisit a finished slot without
another done event; anonymous calls cannot reuse a finished slot.

Godex lacked item-added/item-done events and used a nonblank finish reason
as the entire response's completion barrier. That dropped trailing usage
chunks and treated EOF after STOP as successful even without DONE. The
native stream now completes pending tool items at a normal finish reason,
retains subsequent metadata, and emits response completion only on DONE.
Premature EOF still fails. MAX_TOKENS emits `response.incomplete`, while
the existing native finish-error classifier emits `response.failed` for
malformed calls, safety failures and other recognized failures. The emitted
SSE frame delimiter now matches the tagged runtime's exact CRLF bytes.
Finish handling requires an actual candidate parts array, including an
empty array; missing, null or incorrectly typed parts retain finish metadata
without triggering a terminal event, matching the runtime's early return.

Fail-before regressions verify full tool event order and sequence numbers,
absence of arguments on added items, identical done/final items, special-tool
suppression, slot completion deduplication, trailing usage and EOF failure,
recognized finish statuses, and exact CRLF/serde event bytes. Existing
tool-name/null-argument tests now supply the required DONE marker, retaining
their complete field/value assertions. Raw finish metadata remains tested
with visible content plus a separate assertion that STOP cannot commit the
response early. No assertion or meaningful EOF coverage is removed.

The protocol record and finish classifier stay private to `gateway/gemini`.
The change adds no dependency, public model, helper, persisted field,
configuration, account selection, retry decision or extra goroutine.
`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 766 tests. `rtk go test ./...` passes 8,722 tests across 81 packages.
`rtk proxy make verify` passes formatting, source size, framework,
cross-build/test compilation, installers, vet, the full race suite and build.
Logs: `/tmp/gdx-04384-stream-lifecycle-final2-test-all.log` and
`/tmp/gdx-04384-stream-lifecycle-final2-verify.log`. Clean candidate binary
differential validation follows the commit.

Response-created/text/reasoning/media lifecycle, embedded provider errors
and prompt-block precedence remain separate audit areas; this slice is not
a claim of full native SSE parity.

Clean commit `cac81c0935ab939fe456b9bb32db155ce83f5ec7` passed all 40
official Gemini executable cases and mutation controls before its
fast-forward push to `origin/main`. Log:
`/tmp/gdx-04384-stream-lifecycle-oracle.json`. The native tool lifecycle
and finish barriers are covered by tagged-source regressions, separately
from that API-key executable differential.

## Native Gemini embedded errors and prompt-block precedence

The tagged runtime checks the presence of `error` before acquiring chunk
metadata or candidate content. Every present value, including null and
non-objects, is terminal. Within an error object, a present `status` takes
precedence over `code` even when null or incorrectly typed. Strings retain
their exact bytes, while numeric codes require serde's signed-i64 grammar;
floats, negative zero and integers beyond i64 use `provider_stream_error`.
Only a string message is eligible, including the empty string.

After metadata acquisition, a nonblank string `promptFeedback.blockReason`
must fail before any text or tool from the same chunk is emitted. Its raw
whitespace and Unicode remain in the diagnostic. Embedded errors win over
prompt blocking and cannot acquire a response ID from that rejected chunk.
Both failure kinds suppress all later chunks and DONE completion.

Godex previously ignored embedded errors and processed candidate content
before noticing prompt blocking. The native SSE boundary now follows that
ordering. Failed events carry the stream's creation epoch and reuse the
existing `helper/redact.Secrets` for provider-message credential shapes.
Prompt classification is shared privately with the existing buffered
classifier in `gateway/gemini`; no new helper package, dependency, public
model, persistence format, retry policy or goroutine is introduced.

Unmodified pinned `gemini_value_policy.mojo` was compiled with local
Mojo 1.1.0:

```bash
rtk proxy mojo build --emit shared-lib -O0 -I mojo/prodex_core -o /tmp/gdx-04384-gemini-value-policy.so mojo/prodex_core/gemini_value_policy.mojo
```

The source's exported ABI, fed values canonicalized with the pinned default
`serde_json 1.0.154` and `serde 1.0.229`, generated 32 deterministic fixtures in
`testdata/native_stream_error_04384.json`. They cover status presence,
signed limits, unsigned overflow, decimal/exponent tokens, negative zero,
duplicate keys, empty strings and Unicode. These are compiled tagged-source
goldens, not released-executable native-OAuth evidence. The fail-before
suite in `/tmp/gdx-04384-stream-error-before.log` also demonstrates blocked
candidate suppression and synthetic credential redaction failures.

The focused final
`rtk go test -race ./internal/gateway/gemini ./internal/helper/redact ./tools/differential -count=1`
passes 809 tests. An HTTP-response/SSE-pump regression also verifies that an
embedded failure after committed text preserves the original response ID,
emits exactly one failure and suppresses later text and completion. Broader
native lifecycle and redaction equivalence remain outside this slice.

`rtk go test ./...` passes 8,764 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including full race tests.
Logs: `/tmp/gdx-04384-stream-error-final-race.log`,
`/tmp/gdx-04384-stream-error-final-test-all.log` and
`/tmp/gdx-04384-stream-error-final-verify.log`. Clean candidate executable
differential validation follows the commit.

## Locked serde dependency audit

The tagged root lockfile pins `serde 1.0.229`, `serde_json 1.0.154`,
`itoa 1.0.18`, `memchr 2.8.3` and `zmij 1.0.23`. The feature graph was
checked without changing the reference source:

```bash
rtk proxy cargo tree --locked --offline -e features -i serde_json
```

Only `default` and `std` are enabled; `float_roundtrip`, `arbitrary_precision`
and `preserve_order` are absent. Source formatting uses zmij. Earlier
references to serde_json 1.0.151/Ryu were inherited probe descriptions,
not an exact description of the 0.438.4 lockfile.

A new offline Rust probe pinned to the actual lockfile versions was
compared with Godex's existing Unicode/depth admission, whole-value decode,
numeric admission and canonical writer. The deterministic seed-4384 corpus
contains 21,317 inputs spanning signed/unsigned limits, negative zero,
subnormals, decimal/exponent overflow and underflow, long mantissas,
non-ASCII strings, duplicate fields, nesting and malformed trailing values.
All validity decisions and accepted canonical bytes match. The temporary
Go probe initially omitted the caller's trailing-value check; three fixture
mismatches were corrected in that probe, with no production change.

The checked-in 48 numeric goldens in
`helper/jsonutil/testdata/serde_locked_04384.json` come from the locked Rust
parser and serializer. They exercise the existing shared writer and numeric
validator. Version comments now identify the actual dependency. No runtime
behavior, dependency, exported API, ownership boundary or persisted format
changes. Historical 0.438.3 comments keep their original version labels.

Comparison log: `/tmp/gdx-04384-serde154-comparison.json`; corpus SHA-256:
`85e4fc4909dc433265e060f22adfedb66cecc718fde422e117629f3d59d615d6`.
This is bounded locked-source evidence, not a released-executable or
whole-provider parity claim. Typed Kiro envelopes and every possible f64
remain outside this probe's coverage.

`rtk go test ./internal/helper/jsonutil ./internal/gateway/gemini -count=1`
passes 694 tests. `rtk go test ./...` passes 8,813 across 81 packages.
`rtk proxy make verify` passes all local gates, including the full race
suite. Logs: `/tmp/gdx-04384-serde154-focused.log`,
`/tmp/gdx-04384-serde154-final-test-all.log` and
`/tmp/gdx-04384-serde154-final-verify.log`.

Clean error-boundary commit `b86e982d0295b28cada3862fa5faf63d83ccdb0f`
passed all 40 official Gemini executable cases and mutation controls before
its fast-forward push to `origin/main`. Log:
`/tmp/gdx-04384-stream-error-oracle.json`. Test/comment-only commit
`52cde7f7e726c67084f2f10f8d65892b89510a98` then passed the locked-serde
local gates and was pushed; the executable differential was not rerun for
that test-only commit.

## Native Gemini stream metadata replaces provider snapshots

The tagged runtime merges response metadata with
`gemini_provider_core_merge_stream_metadata`. Its Mojo
`MergeValueObjects` operation merges the outer object and one immediate
object level. Because runtime metadata is wrapped in `{"gemini": ...}`,
each present provider field replaces the entire earlier field value.
It does not recursively retain children inside usage or grounding
snapshots. Missing or top-level null provider fields remain filtered by
the existing acquisition policy, leaving earlier values intact.

Godex used the request-metadata merge routine for stream updates. That
routine intentionally preserves existing scalar fields, so later finish
reasons, usage counters, empty ratings, false values and nested snapshots
could retain stale values. The stream now assigns every newly acquired
provider field directly. Request-metadata merging keeps its separate
ownership policy; no helper, package, type, dependency or format is added.

Fail-before regressions in
`/tmp/gdx-04384-stream-metadata-before.log` demonstrate eight replacement
cases and stale usage/prompt snapshots. Controls preserve absent/null
fields and unrelated metadata. HTTP-response/SSE-pump coverage verifies
that final provider usage metadata agrees with final usage and retains
the request's ID/client metadata. These are tagged-source regressions,
not released-executable native-OAuth evidence.

`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 821 tests. `rtk go test ./...` passes 8,826 across 81 packages.
`rtk proxy make verify` passes all local gates, including full race tests.
Logs: `/tmp/gdx-04384-stream-metadata-race.log`,
`/tmp/gdx-04384-stream-metadata-final-test-all.log` and
`/tmp/gdx-04384-stream-metadata-final-verify.log`. Clean candidate executable
differential validation follows the commit.

Clean commit `6dae0ff25ce3cec7fead6f6c4e9e582663269291` passed all 40
official Gemini executable cases and mutation controls before its
fast-forward push to `origin/main`. Log:
`/tmp/gdx-04384-stream-metadata-oracle.json`.

## API-key Gemini SSE canonical event bytes

The active API-key Gemini Chat Completions stream is emitted by the tagged
shared Chat-compatible runtime with `serde_json::to_string` and exact CRLF
frames. Godex's event builder used `json.Marshal`, escaping literal HTML
characters and U+2028/U+2029 in text deltas, completed items and final
output. Its intermediate decoders also converted provider integers to
float64, which cannot preserve usage counters above `2^53`.

The event builder now reuses the existing serde-compatible value writer.
Both intermediate event and usage decoding retain `json.Number`, so an
integer remains an integer and full-range u64 counters remain exact. No
dependency, public model, shared helper, configuration, persisted format,
new stream owner or account/retry policy is added. Protocol serialization
stays in `gateway/gemini`.

The existing `stream-chat` executable fixture now includes ASCII text,
`<>&`, a slash, non-ASCII text, Unicode line separators and exact usage
counts above `2^53`. Its oracle checks the entire six-event trace against
an independent static fixture using the validated response ID, item ID and
creation epoch. This retains every material payload byte, object key,
spacing and CRLF delimiter; generated identities and clocks still have
consistency checks. Negative controls prove that HTML/Unicode escapes,
bare LF, JSON whitespace and a changed `data:` prefix are rejected even
when the decoded events are otherwise equivalent. The static fixture does
not call the production SSE builder or its serde writer.

Local fail-before logs:
`/tmp/gdx-04384-chat-sse-wire-before.log` and
`/tmp/gdx-04384-chat-sse-wire-integer-before.log`. Plain ASCII and literal
escaped-text lookalikes remain positive controls. The focused final
`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 835 tests. Released-executable baseline and clean-candidate
validation are recorded separately after their runs complete.

The first released-executable fail-before run used the extended text with
ordinary 2/1/3 usage. Prodex passed the strict oracle; only Godex commit
`6dae0ff25ce3cec7fead6f6c4e9e582663269291` failed `stream-chat` (39/40
matching cases), while mutation controls passed. Log:
`/tmp/gdx-04384-chat-sse-wire-before-oracle.json`. The final fixture also
checks exact larger integer usage; its baseline run is separate.

`rtk go test ./...` passes 8,840 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including full race tests.
Logs: `/tmp/gdx-04384-chat-sse-wire-final2-race.log`,
`/tmp/gdx-04384-chat-sse-wire-final2-test-all.log` and
`/tmp/gdx-04384-chat-sse-wire-final2-verify.log`. Clean candidate binary
differential validation follows the commit.

The final large-usage baseline also failed only Godex's `stream-chat`
case, with the other 39 cases and mutation controls passing. Its client
trace shows Prodex sending integer `9007199254740993` and Godex sending
`9007199254740992`; Prodex retains literal HTML characters while Godex
escapes them. Log: `/tmp/gdx-04384-chat-sse-wire-final-before-oracle.json`.

Clean commit `ace591890358886b92ed9af119f858b5952c7c24` passed all 40
final Gemini executable cases, including literal event bytes and exact
large usage counters, plus all mutation controls. It was fast-forward
pushed to `origin/main`. Log: `/tmp/gdx-04384-chat-sse-wire-oracle.json`.

## API-key Gemini finish barriers retain trailing usage

Tagged `RuntimeDeepSeekSseState::observe_chat_chunk`, used by API-key
Gemini, records a nonblank finish reason without completing the response.
`RuntimeProviderSseReader` invokes completion on `[DONE]`; EOF before that
marker produces a stream failure. This preserves usage-only chunks that
follow a finish-only chunk.

Godex delegated chunk translation to the existing Chat-compatible codec,
then treated its finish-reason completion signal as the entire stream's
completion barrier. That discarded later usage and treated EOF after a
finish-only chunk as success. The Gemini adapter now commits assistant-text
item-done and response-completed only for the DONE marker. The codec's
other consumers keep their separate contracts; no shared helper, API, model, package,
dependency, persistence or retry policy changes.

Fail-before log: `/tmp/gdx-04384-chat-sse-finish-before.log`. Regressions
cover finish-only and finish-with-text chunks, repeated finish reasons,
trailing usage-only chunks, one DONE completion, ignored post-DONE chunks,
and premature EOF with committed text both with and without a finish
reason. The existing strict six-event `stream-chat` executable fixture now
places its exact large usage counters in a separate trailing chunk. Its
expected client trace remains unchanged, including raw payload bytes,
identities, sequence, metadata and final usage.

`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 839 tests. The released baseline and clean-candidate differential
results remain separate from these local regressions.

`rtk go test ./...` passes 8,844 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including full race tests.
Logs: `/tmp/gdx-04384-chat-sse-finish-race.log`,
`/tmp/gdx-04384-chat-sse-finish-final-test-all.log` and
`/tmp/gdx-04384-chat-sse-finish-final-verify.log`. Clean candidate executable
differential validation follows the commit.

The released-executable fail-before run for trailing usage failed only
Godex's `stream-chat` case on commit
`ace591890358886b92ed9af119f858b5952c7c24`: Prodex retained the final
usage-only chunk, while Godex's completed response omitted usage. The
other 39 cases and mutation controls passed. Log:
`/tmp/gdx-04384-chat-sse-finish-before-oracle.json`.

Clean commit `8527a718d6d17833c10116cc921e2ad6f4ca04ea` passed all 40
final Gemini executable cases and mutation controls before its
fast-forward push to `origin/main`. Log:
`/tmp/gdx-04384-chat-sse-finish-oracle.json`.

## API-key Gemini EOF failure carries the creation epoch

The tagged Chat-compatible runtime emits failure events through
`runtime_provider_sse_failed_event`, including the same `created_at` as
the response-created event. Godex's API-key event builder omitted that
field. The failure payload now includes the stream's existing creation
epoch; no new state, clock, helper, dependency, public API or retry policy
is introduced.

The exact-byte regression in
`/tmp/gdx-04384-chat-sse-eof-before.log` fails before the one-field fix and
also checks that a failure cannot emit another terminal event. The new
`stream-chat-eof` executable case retains committed text, sends a normal
finish and trailing usage, then ends without DONE. It requires HTTP 200,
one upstream attempt, four consistent created/item/text events followed by
one exact canonical failure frame, with no item-done or completed response.
The matrix now has 41 cases.

The independent EOF fixture and mutation controls verify creation epoch,
response/item identity, terminal kind, sequence, code, message and CRLF
bytes. Missing/mismatched failure timestamps, a success event, altered
errors and framing mutations are rejected. These controls join the
executable oracle's existing mutation gate.

`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 841 tests. Source-derived/local tests and released executable
evidence remain distinct; full parity is not established.

`rtk go test ./...` passes 8,846 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including full race tests.
Logs: `/tmp/gdx-04384-chat-sse-eof-race.log`,
`/tmp/gdx-04384-chat-sse-eof-final-test-all.log` and
`/tmp/gdx-04384-chat-sse-eof-final-verify.log`. Clean candidate executable
differential validation follows the commit.

The released EOF baseline on `8527a718d6d17833c10116cc921e2ad6f4ca04ea`
matched 40/41 cases and failed only Godex's missing failure timestamp.
Prodex and mutation controls passed. Clean commit
`eb15bee887f080663c6439ce21ea33718e7997c7` then passed all 41 cases and
controls before its fast-forward push to `origin/main`. Logs:
`/tmp/gdx-04384-chat-sse-eof-before-oracle.json` and
`/tmp/gdx-04384-chat-sse-eof-oracle.json`.

## DeepSeek embedded SSE errors preserve field presence and numeric kinds

Tagged `deepseek_provider_core_chat_stream_error` acquires the exact
`error` member and delegates to the compiled `StreamErrorClassification`
operation. A present `type` overrides `code`, even when empty, null or
incorrectly typed. A present `message` similarly overrides `detail`.
Strings retain empty values and whitespace. Numeric codes require signed
i64 values; integral floats, exponent tokens, negative zero and unsigned
overflow use the generic code. A root error string is a message, while
other present non-objects use the generic code/message.

Godex's parser borrowed secondary values after an ineligible primary and
converted error numbers through float64. Fail-before coverage in
`/tmp/gdx-04384-deepseek-sse-error-before.log` demonstrates incorrect
fallbacks, loss of the signed maximum and acceptance of floating codes.
The private error codec now preserves presence, exact case and numeric
type. Its additional bounded decode runs only when `error` is present;
normal chunk acquisition and other numeric fields keep their current
contracts. A mixed-case `Error` decoy cannot shadow the lowercase member.

Twenty-one packet cases assert the actual failed event, original response
ownership, no response-created or text event from a rejected chunk, and no
later DONE completion. A separate offline probe using the actual locked
`serde 1.0.229`/`serde_json 1.0.154` verifies all nine signed-number boundary
outcomes. Log: `/tmp/gdx-04384-deepseek-sse-error-i64-oracle.log`.
This is locked-dependency/source evidence, not a compiled-Mojo or released
executable claim for these new edge cases.

The classifier stays in a focused private `gateway/deepseek/stream_error.go`.
Tool-call accumulation and signature acquisition move unchanged beside the
existing tool formatter/validator in `stream_tools.go`. The state file
shrinks from 486 to 389 lines, allowing removal of its oversized-file
baseline exception while retaining the 400-line guard. No shared helper,
package, exported API, dependency, clock, goroutine or persisted format is
introduced; all ownership remains in the same outbound integration.

The released baseline additionally fails all three new executable cases
on `eb15bee887f080663c6439ce21ea33718e7997c7`: primary-field presence,
the signed maximum and an integral-float code. Only Godex fails their
strict code/message contracts; Prodex passes. Each report verifies the
original upstream request, client HTTP 200, one primary-key attempt,
zero retries with an alternate key configured, child exit and durable-state
integrity, with mutation controls. Logs:
`/tmp/gdx-04384-deepseek-sse-error-presence-before-oracle.json`,
`/tmp/gdx-04384-deepseek-sse-error-i64-before-oracle.json` and
`/tmp/gdx-04384-deepseek-sse-error-float-before-oracle.json`.
These executable results supersede the source-only limit for those three
specific packet shapes; the other local edge cases retain that limit.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 592 tests after adding independent error-presence mutation controls.

`rtk go test ./...` passes 8,872 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the full race
suite, formatting, source size, vet and build. The final local
verification log is `/tmp/gdx-04384-deepseek-sse-error-final2-verify.log`;
the full-test log is `/tmp/gdx-04384-deepseek-sse-error-final2-test-all.log`.
The exact clean-commit executable differential follows the commit.

Clean commit `dfb50c7fcdca02a63d1819be77902f19e992463a` passed all 33
DeepSeek executable scenarios and mutation controls before its fast-forward
push to `origin/main`. The full matrix includes the three new embedded-error
cases. Log: `/tmp/gdx-04384-deepseek-sse-error-oracle.json`.

## API-key Gemini embedded errors precede response creation

The tagged API-key Gemini stream uses the same Chat Completions error codec
as DeepSeek. `observe_chat_chunk` checks a present error before acquiring
metadata, emitting response-created, or processing any candidate content.
Failures retain field-presence semantics, numeric kinds and the owned
response ID, redact provider-message credentials and suppress later DONE.

Godex's Gemini adapter ignored embedded errors and eagerly emitted creation
before inspecting its first packet. The stream now checks errors before
creation/content and emits created once for the first valid non-error packet.
The established native-Gemini error codec remains separate because its
status/message rules differ from Chat Completions. Valid scalar/array
packets and the bounded framing/parser behavior retain their own contracts;
partial-JSON accumulation and broader tool/reasoning parity are separate work.

The existing DeepSeek Chat Completions classifier moves unchanged into
`gateway/chatcompat/stream_error.go`, with focused tests and two real
consumers. DeepSeek keeps its failure-path numeric acquisition; Gemini
reuses its complete serde-valid JSON acquisition and passes decoded metadata
directly, avoiding another decode. This is an existing protocol ownership
boundary, not a generic helper or application policy. No dependency,
persisted field, credential-selection policy or goroutine is added.

Fail-before log: `/tmp/gdx-04384-gemini-chat-error-before.log`. Local pump
coverage checks null/string errors, primary null fields, empty strings, the
signed maximum, integral floats, Unicode, no leaked created/text/completed
events, retained committed identity and synthetic bearer redaction. The
`stream-chat-error` executable fixture sends a primary-null error plus
conflicting fallback fields and text, followed by DONE. It requires a single
exact failure frame with sequence zero, an owned UUIDv7 and creation epoch,
HTTP 200, one upstream attempt and successful child exit. Its independent
mutation gate rejects secondary-field borrowing, creation/content leaks,
terminal/sequence changes and noncanonical framing. The Gemini matrix now
contains 42 cases.

`rtk go test -race ./internal/gateway/gemini ./internal/gateway/chatcompat ./internal/gateway/deepseek ./tools/differential -count=1`
passes 1,271 tests. `rtk go test ./...` passes 8,894 across 81 packages.
Logs: `/tmp/gdx-04384-gemini-chat-error-final-race.log` and
`/tmp/gdx-04384-gemini-chat-error-final-test-all.log`.
`rtk proxy make verify` passes all local gates, including the full race
suite. Gate log: `/tmp/gdx-04384-gemini-chat-error-final-verify.log`.
Exact clean-commit executable validation follows the commit.

The released baseline on `dfb50c7fcdca02a63d1819be77902f19e992463a`
matched 41/42 cases and failed only Godex's `stream-chat-error` case.
Prodex emitted the required single failure frame, while Godex leaked
creation/content and completion. Mutation controls passed. Log:
`/tmp/gdx-04384-gemini-chat-error-before-oracle.json`.

Clean commit `e4dca175ae040161e54cc311ad23822b2a8759a0` passed all 42
Gemini cases, all 33 DeepSeek scenarios and their mutation controls before
its fast-forward push to `origin/main`. Logs:
`/tmp/gdx-04384-gemini-chat-error-oracle.json` and
`/tmp/gdx-04384-gemini-chat-error-deepseek-oracle.json`.

## Gemini SSE events may contain separate JSON data lines

The tagged shared provider SSE reader first tries the joined data lines as
one serde JSON value. When that fails, it tries every nonblank data line as
a complete value, validates the full set, then observes them in order.
Godex previously accepted only joined JSON and rejected two complete
values in one event. Both native and API-key Gemini streams now implement
the separate-value fallback with the same whole-serde admission boundary.

The private `gateway/gemini/stream_data.go` parser has two real
consumers and performs no I/O. Ordinary joined JSON keeps its existing path;
fallback parsing cannot emit the first candidate when a later line is
invalid. Every returned line is complete JSON, so recursive observation
cannot re-enter the fallback. Empty lines are skipped. No new helper
package, dependency, model, option, persistence, retry rule or goroutine is
introduced. Accumulation across separate SSE events and broader framing
size/error behavior remain separate audit areas.

Fail-before log: `/tmp/gdx-04384-gemini-sse-lines-before.log`. HTTP/SSE-pump
regressions cover both adapters with two text values in one event. Controls
check valid pretty-printed joined JSON, blank lines, a malformed second
value, unpaired surrogates and floating overflow, with no partial output
from a rejected event. The `stream-chat-data-lines`
executable fixture sends the same canonical text/usage conversation using
two data lines for its first event. It must yield the existing exact
six-event client trace. The Gemini matrix now contains 43 cases.

Source-derived/local tests and released-executable evidence remain distinct.

The reader's earlier DONE gate checks every data line before JSON parsing.
A DONE marker in any position completes the already committed content and
suppresses every other line in that event, even an embedded error or malformed
JSON. Both adapters now apply this precedence. Fail-before evidence:
`/tmp/gdx-04384-gemini-sse-done-before.log`; the focused final
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiSSE' -count=1`
passes 16 tests. This remains separate from cross-event JSON accumulation.

The final
`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 864 tests. `rtk go test ./...` passes 8,903 across 81 packages.
`rtk proxy make verify` passes all local gates, including the full race
suite. Logs: `/tmp/gdx-04384-gemini-sse-lines-final2-race.log`,
`/tmp/gdx-04384-gemini-sse-lines-final2-test-all.log` and
`/tmp/gdx-04384-gemini-sse-lines-final2-verify.log`. Clean candidate
executable validation follows the commit.

The released separate-line baseline on
`e4dca175ae040161e54cc311ad23822b2a8759a0` matched 42/43 cases and
failed only Godex's `stream-chat-data-lines` case. Mutation controls
passed. Clean commit `c6091b1b318222212161e82792c455ee9d88c40b` then
passed all 43 Gemini executable cases and controls before its fast-forward
push to `origin/main`. Logs:
`/tmp/gdx-04384-gemini-sse-lines-before-oracle.json` and
`/tmp/gdx-04384-gemini-sse-lines-oracle.json`.

## Gemini SSE reassembles partial JSON across events

After joined and separate JSON parsing both fail, the tagged provider
reader appends the trimmed fragment to its retained JSON buffer and tries
the combined value. Complete incoming values keep priority and do not
consume an earlier partial fragment. The pinned source's buffer limit is
4 MiB (`core_constants.rs`), independent of Godex's other response/tool
budgets. DONE or EOF with an invalid retained segment fails with
`incomplete JSON segment`; an accumulation overflow fails explicitly and
leaves the bounded state intact. Invalid UTF-8 is rejected before buffering.

Godex previously failed on the first partial packet. Both Gemini adapters
now retain bounded partial JSON, emit only after complete serde-valid
reassembly, and retain joined/separate precedence. Their pump owners close
the upstream body after terminal completion/failure, releasing retained
state without reading another packet. Native EOF now uses the shared
reader's exact `unexpected end of stream` message when no fragment remains.

The state remains private to `gateway/gemini/stream_data.go` and belongs to
each stream. No shared helper package, dependency, public model, option,
persistence or goroutine is introduced. Source per-line/event framing
limits and broader native output lifecycle remain separate audit areas.

Fail-before logs: `/tmp/gdx-04384-gemini-sse-fragments-before.log` and
`/tmp/gdx-04384-gemini-sse-close-before.log`. Regressions prove reassembly
in both adapters, no output from a partial packet, one final completion,
partial EOF/DONE failure, exact 4 MiB admission/overflow, complete-packet
priority and one upstream read before terminal shutdown. The synchronized
read/close fixture uses no sleep. Existing invalid-serde tests now assert
no leaked candidate text and exactly one failed event by DONE, preserving
their Unicode/overflow checks under the reference reader's buffering contract.

The new `stream-chat-json-fragments` executable case splits text in the
middle of a JSON string across two SSE events. It must still produce the
existing exact six-event client trace and exact large usage counters.
The Gemini matrix now contains 44 cases. Local source evidence and released
executable evidence remain separate until that oracle finishes.

`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 871 tests. Log: `/tmp/gdx-04384-gemini-sse-fragments-final-race.log`.

`rtk go test ./...` passes 8,910 tests across 81 packages. Full-test log:
`/tmp/gdx-04384-gemini-sse-fragments-final-test-all.log`.
`rtk proxy make verify` passes all local gates, including the full race
suite. Gate log: `/tmp/gdx-04384-gemini-sse-fragments-final-verify.log`.
Exact clean-commit executable validation follows the commit.

The fragment baseline on `c6091b1b318222212161e82792c455ee9d88c40b`
matched 43/44 Gemini cases and failed only Godex's fragment case; Prodex
and mutation controls passed. Clean commit
`6c450b3788dc712fc2f6245093c7ba5689f60a23` then passed all 44 cases
and controls before its fast-forward push to `origin/main`. Logs:
`/tmp/gdx-04384-gemini-sse-fragments-before-oracle.json` and
`/tmp/gdx-04384-gemini-sse-fragments-oracle.json`.

## DeepSeek buffered and SSE usage preserves unsigned integers

The tagged `response.rs`, `stream/shaping.rs` and Mojo
`deepseek_put_response_usage` retain serde unsigned integer counters, including
u64::MAX. Missing/invalid totals use saturating prompt-plus-completion counts;
present maximum cache/reasoning counts remain present. Streaming creation
epochs are unsigned throughout the source pipeline.

Godex previously narrowed the shared scalar parser to signed i64, treated
u64::MAX as an absence sentinel, wrapped the missing total, and rounded counters
through float64 in translated buffered responses and SSE decoding/cloning.
Its stream clock also narrowed unsigned epochs through time.Time. The shared
Chat-compatible scalar parser now preserves unsigned presence and range;
DeepSeek keeps JSON numbers through response/stream serialization and privately
retains the usage snapshot without a lossy clone. The stream clock is u64.
The redundant error-only JSON parser is removed because the whole packet now
preserves number kinds. Existing exact-key/error-presence coverage remains.

Ownership remains in `gateway/chatcompat` for the protocol mapping and
`gateway/deepseek` for provider input and stream state. Claude is the other
real shared response consumer. No helper, public model, option, dependency,
new goroutine, persistence or layer-direction change is introduced.

Fail-before `/tmp/gdx-04384-deepseek-usage-u64-before.log` passes both
ordinary buffered/SSE controls and fails all eight large-counter/saturation
cases. Regressions also cover unsigned epochs and absent, negative, decimal,
exponent, overflow and non-finite scalar rejection. Two new released-oracle
scenarios independently check exact integer tokens with UseNumber before
applying the existing response and complete SSE contracts; rounding cannot
be hidden by semantic float equality. They also check normal request/auth,
attempt, exit, header and durable-state invariants. Broader metadata/tool
numeric canonicalization and other provider/transport parity remain open.

The released baseline on `6c450b3788dc712fc2f6245093c7ba5689f60a23`
fails both new scenarios only on Godex, while Prodex independently satisfies
their exact unsigned contracts. Logs:
`/tmp/gdx-04384-deepseek-usage-u64-buffered-before-oracle.json` and
`/tmp/gdx-04384-deepseek-usage-u64-sse-before-oracle.json`.

`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/chatcompat ./tools/differential -count=1`
passes 666 tests. Log:
`/tmp/gdx-04384-deepseek-usage-u64-oracle-race.log`.
After tightening the oracle's buffered epoch assertion,
`rtk go test ./tools/differential -run '^TestProdex04384DeepSeekUnsignedUsageOracle' -count=1`
passes all three tests, and `rtk go test ./...` passes 8,940 tests across
81 packages. Logs: `/tmp/gdx-04384-deepseek-usage-u64-oracle-selfcheck.log`
and `/tmp/gdx-04384-deepseek-usage-u64-verified-test-all.log`.

`rtk proxy make verify` passes all local gates, including cross-builds,
installer checks, vet and the complete shuffled race suite. Log:
`/tmp/gdx-04384-deepseek-usage-u64-verified-final.log`. Clean-commit
released-executable validation follows this commit.



Clean commit `8d675af9c0d671d2fb3d3c43485f60ecc14899c4` passed all
35 DeepSeek released-executable scenarios, including both new unsigned cases,
and runtime mutation controls. Report:
`/tmp/gdx-04384-deepseek-usage-u64-oracle.json`.

The same clean commit also passed all 44 Gemini released cases and controls:
`/tmp/gdx-04384-deepseek-usage-u64-gemini-oracle.json`. It was then
fast-forwarded and pushed to `origin/main`.

## Present DeepSeek usage replaces prior counters even when null

Tagged `deepseek/response.rs` converts every present usage Value with the
compiled mapper; non-object Values produce explicit zero counters. The Mojo
stream metadata projector retains field presence, and Rust maps a present
null to that zero object. Missing fields leave the prior stream snapshot
alone. Godex previously required an object and therefore dropped present
scalar/array/null usage or retained stale counters from an earlier packet.

The shared `gateway/chatcompat` response mapper now checks presence at its
root caller and maps non-objects to zeros. `gateway/deepseek` retains a
private presence bit alongside the raw stream snapshot, so a present null
replaces prior usage while a missing field preserves it. Ownership, public
models, dependency direction, persistence and helpers remain unchanged;
Claude's real shared response consumer is included in focused race coverage.

Fail-before `/tmp/gdx-04384-deepseek-usage-presence-before.log` passes
six missing/ordinary/empty-object controls and fails ten null/scalar/array
buffered/SSE cases, plus the stale-snapshot parent regression. Local tests
also verify a missing later snapshot preserves prior usage and a present
null/false clears it. The executable matrix adds two scenarios: buffered
null and a stream with nonzero usage followed by null. Independent assertions
require exactly three zero integer counters before checking the existing
response or complete five-event SSE contract. Negative controls reject
missing, null, decimal and stale output usage, changed text and trailing JSON.

Both new scenarios fail only Godex on clean baseline
`8d675af9c0d671d2fb3d3c43485f60ecc14899c4`; Prodex meets their independent
contracts. Logs:
`/tmp/gdx-04384-deepseek-usage-presence-buffered-before-oracle.json` and
`/tmp/gdx-04384-deepseek-usage-presence-sse-before-oracle.json`.

`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/chatcompat ./internal/gateway/claude -count=1`
passes 495 tests. `rtk go test ./tools/differential -run '^TestProdex04384DeepSeekNullUsageOracle' -count=1`
passes its mutation test. `rtk go test ./...` passes 8,959 tests across
81 packages. `rtk proxy make verify` passes all local gates, including the
complete shuffled race suite. Logs:
`/tmp/gdx-04384-deepseek-usage-presence-race.log`,
`/tmp/gdx-04384-deepseek-usage-presence-oracle-selfcheck.log`,
`/tmp/gdx-04384-deepseek-usage-presence-final-test-all.log` and
`/tmp/gdx-04384-deepseek-usage-presence-final-verify.log`. Clean-commit
released-executable validation follows this commit.

Clean commit `a05e4386b5eb1dd0730bd767bea613caa80aa97b` passed all
37 DeepSeek released scenarios and runtime mutation controls, including both
new null-usage cases, before its fast-forward push to `origin/main`.
Report: `/tmp/gdx-04384-deepseek-usage-presence-oracle.json`.

## DeepSeek keeps present response strings and stream model absence

Tagged `bridge/chat_compat/response.rs` and `tool_calls.rs` apply fallbacks
only when identity values are not strings. Empty strings are present values.
The provider-core stream ID policy accepts the first supplied string while
the current ID still has its generated fallback prefix; later IDs cannot
replace an already supplied empty or whitespace ID. Runtime metadata stores
model strings without trimming, including empty strings, and a stream with
no string model omits that member from its sparse completion response.

Godex's shared `gateway/chatcompat` scalar helper no longer replaces empty
strings. `gateway/deepseek` retains supplied ID/model bytes and tracks model
presence, so a missing/non-string model does not turn into a buffered default.
A later present empty model clears an earlier model, whereas null/missing
leaves the prior string. The existing chunk metadata acquisition and unsigned
epoch parser move together into private `stream_metadata.go`; stream state
remains below its source guard. This is a cohesive concern in the existing
gateway package. No new helper package, exported API, dependency, option,
persistence format, goroutine or layer-direction change is introduced.

Fail-before `/tmp/gdx-04384-deepseek-stream-identity-before.log` proves
empty shared response/model/tool-call ID replacement, trimmed stream strings,
invented models for missing/null input and stale models after present empty
input. Ordinary identity, whitespace shared values, non-string fallback and
late null model controls pass. Actual SSE regressions check identity on every
envelope and exactly one final completion, plus model presence separately.

The released baseline on `a05e4386b5eb1dd0730bd767bea613caa80aa97b`
fails both new empty-identity scenarios only on Godex. Prodex preserves empty
IDs/models and refuses the later ID. Logs:
`/tmp/gdx-04384-deepseek-stream-identity-buffered-before-oracle.json` and
`/tmp/gdx-04384-deepseek-stream-identity-sse-before-oracle.json`.
The executable fixture independently requires exact identity fields and all
five SSE events; missing/null/default/late-replacement/text mutations fail.
Whitespace and absent-model variations currently have source-aligned local
coverage; they are not presented as separate executable oracle cases.

`rtk go test -race ./internal/gateway/chatcompat ./internal/gateway/deepseek ./internal/gateway/claude -count=1`
passes 508 tests. `rtk go test ./tools/differential -run '^TestProdex04384DeepSeekEmptyIdentityOracle' -count=1`
passes its mutation test. `rtk go test ./...` passes 8,973 tests across
81 packages. Logs: `/tmp/gdx-04384-deepseek-stream-identity-race.log`,
`/tmp/gdx-04384-deepseek-stream-identity-oracle-selfcheck.log` and
`/tmp/gdx-04384-deepseek-stream-identity-final-test-all.log`.

`rtk proxy make verify` passes all local gates, including the complete
shuffled race suite. Log:
`/tmp/gdx-04384-deepseek-stream-identity-final-verify.log`. Clean-commit
released-executable validation follows this commit.

Clean commit `fd985be45f77222505f3f63453064c00a3f60672` passed all
39 DeepSeek released cases and runtime mutation controls before its
fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-deepseek-stream-identity-oracle.json`.

## DeepSeek releases the upstream reader after terminal output

The tagged shared provider SSE reader stops filling pending data after its
state reaches EOF. Godex's DeepSeek pump previously wrote completion/failure
but continued calling the upstream reader, so a connection held open after
DONE could prevent the client from receiving EOF indefinitely. Gemini's
corresponding pump ownership was corrected in an earlier section.

The DeepSeek pump now closes its pipe and returns immediately after writing
a terminal state. Its existing defer closes upstream. There is no new
goroutine, helper, public API, option, retry, persistence or layer change.
The owner remains `gateway/deepseek/stream.go`.

Fail-before `/tmp/gdx-04384-deepseek-terminal-close-before.log` proves
extra reads after same-packet DONE, an embedded error, invalid tool failure,
and a separate DONE packet. Premature EOF is a passing control because it
requires the extra read. Channel synchronization observes upstream Close
without sleeps; all terminal outputs occur once and trailing text stays
suppressed. `rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 642 tests. Log: `/tmp/gdx-04384-deepseek-terminal-close-race.log`.

The new `sse-terminal-close` released scenario flushes the existing complete
text/usage fixture then keeps the HTTP response open until request cancellation.
No timer supplies EOF. On clean baseline
`fd985be45f77222505f3f63453064c00a3f60672`, the invocation fails on
Godex's missing child response after the shim read deadline; the tool aborts
without a JSON matrix report. This is failed baseline evidence, not a passing
or completed differential. Log:
`/tmp/gdx-04384-deepseek-terminal-close-before-oracle.stderr`.
DeepSeek line/event size and cross-event JSON accumulation remain separate
unclosed reader parity areas.

`rtk go test ./...` passes 8,979 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the complete
shuffled race suite. Logs:
`/tmp/gdx-04384-deepseek-terminal-close-final-test-all.log` and
`/tmp/gdx-04384-deepseek-terminal-close-final-verify.log`. Clean-commit
released-executable validation follows this commit.

Clean commit `8e3b0cf9e06b7da1da1523e96ae5ae405f74fb84` passed the
held-connection `--scenario sse-terminal-close` oracle, then all 40 released
DeepSeek scenarios and runtime mutation controls before its fast-forward
push to `origin/main`. Reports:
`/tmp/gdx-04384-deepseek-terminal-close-focused-oracle.json` and
`/tmp/gdx-04384-deepseek-terminal-close-oracle.json`.
The successful held-connection run compares both pinned executables and
independently requires HTTP 200, the exact complete five-event client trace,
one authenticated upstream attempt, normal child exit and valid durable state.

This remains bounded parity evidence. Native OS/live-provider coverage and
the unclosed reader/framing/numeric areas above are not certified by these
mock-based executable matrices. The last Gemini executable regression remains
44/44 PASS on `8d675af9c0d671d2fb3d3c43485f60ecc14899c4`; the later
DeepSeek-only commits did not repeat that separate matrix.

## Shared provider SSE JSON framing reaches DeepSeek

The tagged DeepSeek and Gemini readers both delegate to the shared
`RuntimeProviderSseJsonReader`. DeepSeek still treated each SSE data value as
one complete object, failing on partial JSON, multiple complete data lines,
valid non-object values and DONE sharing an event with other values. The
reader's joined/separate/retained-fragment precedence was already implemented
privately for the two Gemini adapters.

That technical framing now belongs to the existing `helper/sse` package as
`JSONFrames` with an explicitly owned 4 MiB pending buffer. Both gateway
packages are real production consumers. The helper returns validated raw
values, performs no I/O or application policy, and uses the existing
`helper/jsonutil` serde boundary and number checks. DONE's UTF-8 gate precedes
marker recognition. Domain response events, metadata, errors and terminal
state remain in the gateways. The private Gemini implementation is removed;
its exact-limit test moves beside the helper. Architecture ownership is
documented in `docs/ARCHITECTURE.md`. No dependency, option, persistence,
goroutine or public application model is introduced.

DeepSeek now retains partial segments, processes only fully validated
separate values, preserves complete-packet priority over pending fragments,
and reports `incomplete JSON segment` at DONE/EOF. Valid arrays/scalars carry
no response fields but remain valid events. Existing invalid-serde tests now
assert no leaked text and exactly one failed terminal event by DONE, keeping
Unicode and floating-point coverage under the shared reader contract.

Fail-before `/tmp/gdx-04384-provider-sse-json-before.log` passes the joined
JSON control and fails the targeted reassembly/separate/scalar/DONE/error
contracts. Baseline `8e3b0cf9e06b7da1da1523e96ae5ae405f74fb84` also
fails both new released scenarios only on Godex; Prodex meets the independent
full stream contract. Logs:
`/tmp/gdx-04384-provider-sse-json-fragments-before-oracle.json` and
`/tmp/gdx-04384-provider-sse-json-lines-before-oracle.json`.

`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/helper/sse -count=1`
passes 1,132 tests. `rtk go test ./...` passes 8,993 tests across 81 packages.
Logs: `/tmp/gdx-04384-provider-sse-json-final-race.log` and
`/tmp/gdx-04384-provider-sse-json-final-test-all.log`.
Per-line/event size/error semantics remain a separate reader audit. The exact
4 MiB accumulator admission/overflow currently has focused local coverage,
not a separate large-payload released-executable scenario.

`rtk proxy make verify` passes all local gates, including the complete
shuffled race suite. Log: `/tmp/gdx-04384-provider-sse-json-final-verify.log`.
Clean-commit DeepSeek and Gemini executable validation follows this commit.

## DeepSeek client stream closure owns upstream cancellation

The tagged provider reader owns its upstream reader for the response lifetime.
Godex's Chat bridge returned a bare pipe reader: closing it unblocked writes
but did not interrupt an upstream Read already waiting for more data. The
Messages bridge had a source-closing wrapper, but its pump defer separately
closed that same source after completion, permitting duplicate Close calls.

Both adapters now use that existing private pipe/source ownership pattern in
`gateway/deepseek/stream_body.go`. The client Close and pump completion share
one source-closing `sync.Once`; client Close also releases the pipe. The two
existing pump goroutines retain their owners and now have an upstream
cancellation path. No shared helper, exported API, option, dependency,
persistence or layer-direction change is introduced. The wrapper stays local
because both consumers belong to this gateway and own the same lifecycle.

Fail-before `/tmp/gdx-04384-deepseek-stream-body-before.log` proves the
Chat source remains blocked after client Close and the Messages source closes
twice after completion/client Close. The channel-based fixture covers both
protocols without sleeps, releases the fail-before blocked read, and checks
one source close across completion and repeated client Close. These are local
lifecycle regressions, not a new released executable cancellation case.
The existing executable `cancel` scenario covers its earlier request phase
and is not presented as proof of this post-response Close contract.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 655 tests. `rtk go test ./...` passes 8,995 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the complete
shuffled race suite. Logs: `/tmp/gdx-04384-deepseek-stream-body-race.log`,
`/tmp/gdx-04384-deepseek-stream-body-final-test-all.log` and
`/tmp/gdx-04384-deepseek-stream-body-final-verify.log`. Clean-commit
released-executable regression follows this commit.

Clean JSON-framing commit `527b2ae0e04fdd77236958a7e6d0bc814c322952`
passed all 42 DeepSeek and 44 Gemini released cases and mutation controls
before its fast-forward push. Reports:
`/tmp/gdx-04384-provider-sse-json-oracle.json` and
`/tmp/gdx-04384-provider-sse-json-gemini-oracle.json`.
Clean body-lifecycle commit `361d385976c82cfeb092d002693a41bef4cf1269`
then passed the same 42 DeepSeek released scenarios and controls before its
fast-forward push. Report: `/tmp/gdx-04384-deepseek-stream-body-oracle.json`.
That regression does not replace the local post-response Close tests above.

## Provider SSE wire limits fail explicitly at the tagged bounds

Tagged `provider_sse_reader.rs` counts physical line bytes including CR/LF,
and counts data bytes plus one byte per data line for each event. Both bounds
are 4 MiB. Every physical line must be valid UTF-8, including ignored fields
and comments. An over-limit line/event or invalid UTF-8 is terminal; it cannot
be skipped to admit later text. Godex's generic skip/recover decoder instead
silently discarded packets above its 1 MiB provider limit and ignored unsafe
comment lines.

The existing `helper/sse.Decoder` gains a strict constructor and terminal error
accessor. It retains its prior default contract for other real consumers.
DeepSeek Chat and both Gemini pumps select strict 4 MiB bounds and convert
framing failures to one provider failure. Wire/event counts include original
data bytes before BOM normalization. EOF validates the last unterminated line.
Provider-specific output and lifecycle remain in the gateways. Architecture
ownership is documented; no new package, dependency, configuration, goroutine
or persistence format is introduced.

Once Gemini accepts larger upstream values, its internally generated single
Responses frame must also fit the intermediate decoder. That decoder now
uses the actual generated frame length rather than another unrelated 1 MiB
limit. Its input is already bounded by the admitted upstream event. The
regression checks that all 2 MiB of visible text reach the client exactly once.
Fail-before: `/tmp/gdx-04384-provider-sse-limits-large-text-before.log`.

`/tmp/gdx-04384-provider-sse-limits-before.log` passes three ordinary
gateway controls and fails twelve targeted large-packet, oversized line/event
and invalid-comment cases across DeepSeek, Gemini Chat and native Gemini.
Focused decoder tests check exact LF/CRLF/EOF line admission, exact event
admission, counted BOM bytes, invalid ignored fields, terminal no-recovery
and preservation of an earlier complete event before a later error. An
initial BOM fixture exceeded the physical line bound first; its length was
corrected to isolate the intended event-bound assertion. The corrected
`rtk go test ./internal/helper/sse -run '^TestStrictDecoder' -count=1`
passes 14 tests. Log: `/tmp/gdx-04384-provider-sse-limits-strict-unit.log`.

On clean baseline `361d385976c82cfeb092d002693a41bef4cf1269`, all four
new released wire scenarios fail only Godex; Prodex independently meets the
normal trace or exact failure contract. Logs:
`/tmp/gdx-04384-provider-sse-large-packet-before-oracle.json`,
`/tmp/gdx-04384-provider-sse-line-limit-before-oracle.json`,
`/tmp/gdx-04384-provider-sse-event-limit-before-oracle.json` and
`/tmp/gdx-04384-provider-sse-comment-utf8-before-oracle.json`.
Mutation checks reject wrong failure codes/text, shifted sequence and extra
partial output. Boundary byte-level variations and large visible Gemini text
have focused local coverage; the released large packet carries ordinary text
plus ignored padding. Native OS/live-provider and broader numeric/output
canonicalization remain open.

An intermediate `rtk proxy make verify` ran while the differential files
were being added and failed with undefined wire-oracle functions. That run
is failed, not completion evidence:
`/tmp/gdx-04384-provider-sse-limits-final-verify.log`. All final checks below
were rerun after the complete code and test files were present.

`rtk go test -race ./internal/helper/sse ./internal/gateway/deepseek ./internal/gateway/gemini ./tools/differential -count=1`
passes 1,392 tests. `rtk go test ./...` passes 9,028 tests across 81 packages.
`rtk proxy make verify` passes every local gate, including the complete
shuffled race suite. Logs:
`/tmp/gdx-04384-provider-sse-limits-complete-race.log`,
`/tmp/gdx-04384-provider-sse-limits-complete-test-all.log` and
`/tmp/gdx-04384-provider-sse-limits-complete-verify.log`. Clean-commit
DeepSeek and Gemini executable validation follows this commit.

## DeepSeek stream metadata retains exact integer snapshots

Tagged stream metadata clones serde Values and serializes them without
rounding integer variants. Godex still cloned logprobs and annotations through
JSON float64 decoding, both when acquiring the packet and constructing the
final source. Buffered responses already retained numbers after the unsigned
usage correction, but the stream snapshots were separately lossy.

The stream now keeps its privately decoded logprobs value and appends owned
annotation values directly. Completion reads those same snapshots. No caller
can mutate the per-packet decoded objects, and there is no concurrent access
outside the owning pump. The four lossy clone calls are removed; unrelated
request/conversation clone contracts remain unchanged. The owner stays in
`gateway/deepseek/stream_state.go` with no shared helper, exported identifier,
configuration, dependency, goroutine or persistence change.

Fail-before `/tmp/gdx-04384-deepseek-stream-metadata-before.log` passes
the ordinary integer control and fails both >2^53/u64::MAX metadata cases.
The regression observes final client SSE with UseNumber and verifies that a
later null logprobs packet preserves the prior snapshot. Baseline clean commit
`cfe27296b13e865558bc6810c77e8fddb49dd32e` also fails the new released
`sse-metadata-u64` scenario only on Godex. Prodex independently retains exact
logprobs and annotation integers. Report:
`/tmp/gdx-04384-deepseek-stream-metadata-before-oracle.json`.
The independent oracle checks exact metadata bytes before the full five-event
trace and rejects rounded/dropped values, text changes and extra events.
Broader metadata floating-point canonicalization, tool numeric payloads and
other provider/native OS/live-provider behavior remain open.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 666 tests before the new oracle mutation test was added; its final
coverage is included in the complete race gate below. The standalone
`rtk go test ./tools/differential -run '^TestProdex04384StreamMetadataOracle' -count=1`
passes its mutation test. `rtk go test ./...` passes 9,033 tests across
81 packages. `rtk proxy make verify` passes every local gate, including the
complete shuffled race suite. Logs:
`/tmp/gdx-04384-deepseek-stream-metadata-race.log`,
`/tmp/gdx-04384-deepseek-stream-metadata-oracle-selfcheck.log`,
`/tmp/gdx-04384-deepseek-stream-metadata-final-test-all.log` and
`/tmp/gdx-04384-deepseek-stream-metadata-final-verify.log`. Clean-commit
released-executable validation follows this commit.

Clean wire-limit commit `cfe27296b13e865558bc6810c77e8fddb49dd32e`
passed all 46 DeepSeek and 44 Gemini released cases and controls before its
fast-forward push. Reports:
`/tmp/gdx-04384-provider-sse-limits-oracle.json` and
`/tmp/gdx-04384-provider-sse-limits-gemini-oracle.json`.
Clean metadata commit `3fc9f37c0dc827f3a31409f8e1251c280b075ea0` then
passed all 47 DeepSeek released scenarios and controls before its fast-forward
push. Report: `/tmp/gdx-04384-deepseek-stream-metadata-oracle.json`.

## DeepSeek tool validation and item completion happen at finish

Tagged `deepseek_sse.rs::observe_finish` runs for every string finish reason,
including empty strings. It validates all accumulated tools immediately,
emits each pending tool's done item once, and keeps accepting later usage.
Final DONE retains every tool in the final response without repeating its
done event. A malformed tool fails at finish; a later upstream error cannot
replace that failure. Godex previously only stored nonblank finish strings
and waited for DONE to validate and emit tool completion.

`gateway/deepseek` now separates output-item snapshots from pending done
events, validates on each present finish string, and retains finish presence
for response metadata. The shared `gateway/chatcompat` metadata mapping also
preserves present empty finish strings, matching the tagged Mojo projection.
The new private `stream_tool_completion.go` owns validation and done-event
emission for finish/DONE; snapshots still belong to stream state. No shared
helper, exported model, option, dependency, goroutine, persistence format or
layer-direction change is introduced.

Fail-before `/tmp/gdx-04384-deepseek-finish-tools-before.log` passes the
missing/non-string finish control and fails immediate valid completion and
invalid-tool failure for ordinary, empty and whitespace finish strings.
Regressions assert per-packet behavior, one done item, trailing usage, all
tools retained in final output and suppression after terminal failure. The
older invalid-Unicode test now asserts the same exact failure at finish and
no duplicate at DONE; its validation/error assertions remain intact.

On clean baseline `3fc9f37c0dc827f3a31409f8e1251c280b075ea0`, both
new released scenarios fail only Godex. Prodex independently emits the original
four-event invalid-tool trace before the later error, including when the
finish string is empty. Reports:
`/tmp/gdx-04384-sse-tool-finish-error-before-oracle.json` and
`/tmp/gdx-04384-sse-tool-empty-finish-error-before-oracle.json`.
The fixture reuses the exact tool-error contract and its mutation controls.
Valid-tool packet timing and empty buffered finish metadata currently have
source-aligned local coverage rather than separate executable scenarios.
Broader tool payload/numeric and native lifecycle parity remain open.

`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/chatcompat ./internal/gateway/claude ./tools/differential -count=1`
passes 771 tests. `rtk go test ./...` passes 9,040 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the complete
shuffled race suite. Logs: `/tmp/gdx-04384-deepseek-finish-tools-race.log`,
`/tmp/gdx-04384-deepseek-finish-tools-final-test-all.log` and
`/tmp/gdx-04384-deepseek-finish-tools-final-verify.log`. Clean-commit
released-executable validation follows this commit.

Clean finish commit `8918d065a8504f398042905bdf47f60a0266fd63` passed
all 49 DeepSeek released scenarios and runtime mutation controls before its
fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-deepseek-finish-tools-oracle.json`. Production code is fully
committed; this subsequent checkpoint update changes documentation only.

## Execution checkpoint: provider SSE and metadata work

The last tested production commit is
`8918d065a8504f398042905bdf47f60a0266fd63`: 9,040 local tests across
81 packages, the full `make verify` gate, and 49/49 DeepSeek executable cases.
The separate latest Gemini executable matrix is 44/44 PASS on
`cfe27296b13e865558bc6810c77e8fddb49dd32e`; later commits change only
DeepSeek behavior and its shared response mapper, whose Claude consumer has
focused local race coverage. A successful provider subset does not establish
global parity or replace native OS/live-provider validation.

Exact final provider matrix commands (already executed successfully):

```bash
rtk proxy /tmp/gdx-04384-deepseek-finish-tools-candidate-differential --prodex /tmp/godex-release-parity-04384-20261021/prodex-x86_64-unknown-linux-gnu --godex /tmp/gdx-04384-deepseek-finish-tools-candidate-godex --prodex-source /tmp/godex-release-parity-04384-20261021/prodex-source --godex-source /tmp/gdx-04384-deepseek-finish-tools-work --godex-commit 8918d065a8504f398042905bdf47f60a0266fd63 --scenario all --scenario-workers 3
rtk proxy /tmp/gdx-04384-provider-sse-limits-candidate-differential --prodex /tmp/godex-release-parity-04384-20261021/prodex-x86_64-unknown-linux-gnu --godex /tmp/gdx-04384-provider-sse-limits-candidate-godex --prodex-source /tmp/godex-release-parity-04384-20261021/prodex-source --godex-source /tmp/gdx-04384-provider-sse-limits-work --godex-commit cfe27296b13e865558bc6810c77e8fddb49dd32e --gemini
```

The pinned reference source remains unmodified. Candidate worktrees are clean,
with no pending implementation patch. The independent editor checkout's
unrelated files were not changed. No tag, release, dependency or persisted
format change was made; commits were pushed directly without waiting for CI.
The complete multi-phase release parity script was not rerun in this provider
slice. Its broader transport/ownership coverage remains a separate gate.

Continue with targeted fail-before evidence for these still-unproven areas:

- DeepSeek structured tool arguments, floating-point output canonicalization,
  multi-tool error ordering and request/conversation metadata number handling.
- Native Gemini created/text/reasoning/media lifecycle and cancellation ownership.
- Remaining provider SSE BOM, Unicode whitespace and malformed framing semantics.
- Broader Kiro, account ownership, continuation, transport and native OS parity.

These are audit targets, not newly confirmed executable mismatches. Keep the
same official 0.438.4 source, binary, manifest and locked serde dependencies.

## Structured DeepSeek tool-search arguments retain serde number values

The pinned buffered tool-call kernel retains structured `tool_search`
arguments and returns a serde Value; streaming `stream_tool_call_item` also
parses/serializes that Value before embedding it. Godex used float64 decoding
in both adapters, rounding integers above 2^53 and serializing decimal and
negative-zero values with different numeric kinds.

`gateway/deepseek/tool_search.go` now owns the existing two adapters' private
structured argument conversion. It validates the complete serde boundary and
number range, decodes with UseNumber, and uses the existing locked-serde writer
to retain canonical JSON number tokens inside RawMessage. Buffered validation
errors remain provider failures; streaming retains the reference's empty
argument fallback after its owning validator. The stream argument validator
now checks serde number validity directly instead of parsing every scalar
through Go float64, allowing the pinned long-mantissa finite control.
Other function-tool argument strings and apply_patch behavior are unchanged.
No new helper package, exported interface, model, dependency, option,
goroutine or persistence format is introduced. Conversion stays private to
the owning integration; generic serde rules stay in `helper/jsonutil`.

Fail-before `/tmp/gdx-04384-deepseek-tool-search-u64-before.log` passes
six ordinary/null controls and fails eight buffered/SSE large or nested
integer cases. The completed regression additionally checks decimal/exponent,
negative zero, u64 overflow converted to serde f64, and the locked finite
long-mantissa value in both item-done and final response output. Invalid UTF-8,
surrogates, out-of-range float, trailing JSON and depth remain rejected.

On clean baseline `8918d065a8504f398042905bdf47f60a0266fd63`, both
new released cases fail only Godex. Prodex independently meets the entire
canonical byte contract, including nested u64::MAX, 100.0 and -0.0. Reports:
`/tmp/gdx-04384-tool-search-canonical-before-oracle.json` and
`/tmp/gdx-04384-sse-tool-search-canonical-before-oracle.json`.
The oracle compares all buffered/SSE bytes without opaque IDs or float
normalization. Independent invariants reject matching wrong outputs.
Malformed buffered argument diagnostic text and broader envelope string/
floating-point canonicalization remain separate audit targets; local rejection
tests are not presented as released-executable diagnostic equivalence.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 700 tests. `rtk go test ./...` passes 9,067 tests across 81 packages.
`rtk proxy make verify` passes every local gate, including the complete
shuffled race suite. Logs:
`/tmp/gdx-04384-deepseek-tool-search-final-race.log`,
`/tmp/gdx-04384-deepseek-tool-search-final-test-all.log` and
`/tmp/gdx-04384-deepseek-tool-search-final-verify.log`. Clean-commit
released-executable validation follows this commit.

## Gemini client cancellation closes its owned upstream stream

Both Gemini adapters still returned bare pipe readers. Closing the client
body released pending writes but could leave the owning pump blocked in
upstream Read. The tagged Rust provider SSE reader owns its upstream reader
for the response lifetime. DeepSeek already implemented explicit Go pipe/
source ownership, so the same invariant now has two real gateway consumers.

The existing DeepSeek wrapper moves into `helper/sse/pipe_body.go` with a
small explicit lifecycle API. `Close` cancels the reader and source;
`CloseSource` lets the producer close upstream after completion without
closing a reader that is still draining. Both share one `sync.Once`, including
the original source-close error. Four existing pumps across DeepSeek and
Gemini use it. The superseded private DeepSeek wrapper is removed. The helper
uses only standard-library I/O/synchronization and contains no provider,
request, retry, routing or persistence policy. Architecture ownership is
updated in `docs/ARCHITECTURE.md`. No new dependency, option, model, goroutine
or persistence format is introduced.

Fail-before `/tmp/gdx-04384-provider-stream-cancel-before.log` passes the
normal completion control and fails client cancellation for both Gemini
adapters. Channel-based actual-pump tests prove upstream closure without
sleeps and preserve one close after completion and repeated client Close.
DeepSeek's original lifecycle regressions remain unchanged. Helper tests race
20 producer/client closers, require one source close and the same returned
error, and ensure successful producer completion still drains to EOF.

This is source ownership and local lifecycle evidence. Existing released
Gemini/DeepSeek matrices are regression gates, not a new executable proof
that isolates post-response body Close from HTTP request-context cancellation.
Broader native Gemini lifecycle and transport ownership remain open.

Clean tool-argument commit `4e91bd488579f9f8584bdc26b20a0d83a1dd039f`
passed all 51 DeepSeek released scenarios and runtime mutation controls before
its fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-deepseek-tool-search-oracle.json`.

`rtk go test -race ./internal/helper/sse ./internal/gateway/deepseek ./internal/gateway/gemini ./tools/differential -count=1`
passes 1,434 tests. `rtk go test ./...` passes 9,071 tests across 81 packages.
`rtk proxy make verify` passes every local gate, including the complete
shuffled race suite. Logs: `/tmp/gdx-04384-provider-stream-cancel-race.log`,
`/tmp/gdx-04384-provider-stream-cancel-final-test-all.log` and
`/tmp/gdx-04384-provider-stream-cancel-final-verify.log`. Clean-commit
released-executable regression and the broader release gate follow this commit.

## Deterministic DeepSeek stream tool validation failures

Clean cancellation commit `9a7e8e9283244e3535da02fa07fac54653bb7eb1`
passed 51/51 DeepSeek and 44/44 Gemini released-executable cases, including
their mutation controls, before its fast-forward push to `origin/main`.
Reports: `/tmp/gdx-04384-provider-stream-cancel-oracle.json` and
`/tmp/gdx-04384-provider-stream-cancel-gemini-oracle.json`. The complete
`rtk proxy bash scripts/verify-release-parity.sh` gate is running against that
same clean commit; no final outcome is claimed here.

The tagged `deepseek_sse.rs` owns tools in `BTreeMap<usize, ...>` and
`tool_calls.rs::validate_tool_call_arguments` reports the first invalid
entry by ascending index. Godex iterated its unordered map and could report
a different tool. The existing private gateway validator now retains the
lowest invalid index in one bounded pass without a sort or another helper.
Both finish-string and DONE paths already share this validator. No new
package, exported API, dependency, goroutine, option or persisted format is
introduced; protocol validation stays in the owning DeepSeek gateway.

Fail-before `/tmp/gdx-04384-tool-validation-order-before.log` passes the
valid/single-failure control and fails all six multi-failure subcases.
Actual-pump regressions cover named malformed arguments, absent names,
ordinary/empty finish strings and DONE, with a valid lower-index tool and
u64::MAX. Repeated independent stream lifetimes require one stable failure,
no completed tools and no terminal success.

Both released-executable fail-before cases independently satisfy the full
36-event contract on Prodex. Godex reports index six at finish and index
eight at DONE; the reference reports index three in both. Reports:
`/tmp/gdx-04384-tool-validation-order-finish-before-oracle.json` and
`/tmp/gdx-04384-tool-validation-order-done-before-oracle.json`.
The extended differential includes a valid tool at index zero and sixteen
invalid tools arriving in reverse order, checks every response byte, and
rejects matching wrong errors or terminal sequences through mutation controls.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 709 tests. `rtk go test ./...` passes 9,080 tests across 81 packages.
`rtk proxy make verify` passes formatting, source-size, cross-platform build
and test compilation, installer checks, vet, the full shuffled race suite,
and the build. Logs: `/tmp/gdx-04384-tool-validation-order-race.log`,
`/tmp/gdx-04384-tool-validation-order-test-all.log` and
`/tmp/gdx-04384-tool-validation-order-verify.log`. Clean-commit executable
validation follows this commit. Buffered malformed-argument diagnostics,
broader apply-patch argument canonicalization and global parity remain open.

## Canonical numeric fallback input for streamed apply-patch tools

Clean tool-order commit `50170d3543a28e3537b5c6567164e002da612a6e`
passed 53/53 released DeepSeek scenarios and mutation controls before its
fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-tool-validation-order-oracle.json`.

The tagged stream shaper calls the Gemini custom-tool input converter, whose
fallback serializes a parsed `serde_json::Value`. Godex instead decoded through
float64 and ordinary Go JSON serialization, rounding integers and losing serde
float kinds. The existing private structured argument decoder now serves both
tool-search and apply-patch under `gateway/deepseek/tool_arguments.go`.
Apply-patch's fallback uses the existing locked-serde writer. Explicit patch
strings, aliases, edit paths and patch normalization retain their own existing
conversion. Architecture documentation records the private ownership; generic
number rules remain in `helper/jsonutil`. No new package, exported identifier,
dependency, goroutine, option or state format is introduced.

Fail-before `/tmp/gdx-04384-apply-patch-json-before.log` passes seven controls
and fails eight tests, including the actual-pump completion and malformed
surrogate fallback. The fallback rejection tests are local source-aligned
evidence, not a new executable diagnostic oracle. The released buffered
DeepSeek path still preserves original apply-patch argument strings and is
unchanged by this stream-only conversion.

On clean baseline `50170d3543a28e3537b5c6567164e002da612a6e`, Prodex
independently satisfies the entire four-event canonical contract. Godex emits
18446744073709552000 and 9007199254740992, and loses the reference's 100.0
and -0.0 spellings. Fail-before report:
`/tmp/gdx-04384-apply-patch-json-before-oracle.json`. The differential checks
the completed item and final output byte for byte, without decoding their
numbers through float64. Matching wrong numeric values, item types or event
sequences are rejected by independent mutation controls.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 725 tests. `rtk go test ./...` passes 9,096 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the full shuffled
race suite and cross-platform build/test compilation. Logs:
`/tmp/gdx-04384-apply-patch-json-race.log`,
`/tmp/gdx-04384-apply-patch-json-test-all.log` and
`/tmp/gdx-04384-apply-patch-json-verify.log`. Clean-commit executable
validation follows this commit. Buffered malformed diagnostics and broader
provider SSE BOM/whitespace semantics remain open audit areas.

## Preserve BOM bytes at provider SSE JSON boundaries

Clean apply-patch commit `71bbd547b7f65705124499a6ec729ed6dd7bdc88`
passed 54/54 DeepSeek released-executable scenarios and mutation controls
before its fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-apply-patch-json-oracle.json`.

The tagged `provider_sse_reader.rs` collects data lines and joins them in Rust
before serde validation; it never calls the generic Mojo payload joiner that
strips initial BOMs. Godex reused that generic normalization even in its
strict provider decoder. A provider's leading BOM therefore became acceptable
JSON and emitted text/completion where Prodex retained an incomplete fragment.
The existing strict decoder now preserves provider payload BOMs; the generic
decoder keeps its existing Mojo-compatible BOM behavior. No new decoder mode,
API, dependency or package is added. The three existing strict consumers are
DeepSeek Chat, Gemini Chat-compatible and native Gemini. Framing remains in
`helper/sse`, provider state/events remain in each gateway, and architecture
documentation describes this boundary. Limits, UTF-8 rejection, credential
handling, retry and persistence behavior are unchanged.

Fail-before `/tmp/gdx-04384-provider-bom-before.log` fails both strict-decoder
and actual DeepSeek pump regressions. The Gemini actual-pump regression also
fails before the fix for native and Chat-compatible single/repeated BOMs:
`/tmp/gdx-04384-provider-bom-gemini-before.log`. Normal streams and generic
BOM normalization retain their existing positive controls.

On clean baseline `71bbd547b7f65705124499a6ec729ed6dd7bdc88`, the pinned
Prodex executable independently satisfies one exact failure event with
`incomplete JSON segment`; Godex instead creates a response, emits the fixture
text, and completes successfully. Report:
`/tmp/gdx-04384-provider-bom-before-oracle.json`. New DeepSeek and Gemini
cases test one/repeated BOMs. They validate generated UUIDv7 and timestamp
before comparing every remaining byte, and reject changed failure text,
sequence numbers, frame endings, duplicate events and borrowed upstream IDs.
Native Gemini coverage is local/source-aligned; the released Gemini API-key
oracle exercises the Chat-compatible provider reader.

`rtk go test -race ./internal/helper/sse ./internal/gateway/deepseek ./internal/gateway/gemini ./tools/differential -count=1`
passes 1,463 tests. `rtk go test ./...` passes 9,100 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the shuffled race
suite and cross-platform build/test compilation. Logs:
`/tmp/gdx-04384-provider-bom-race.log`,
`/tmp/gdx-04384-provider-bom-test-all.log` and
`/tmp/gdx-04384-provider-bom-verify.log`. Clean-commit executable validation
follows this commit. Broader provider whitespace/EOF and malformed buffered
argument diagnostics remain unproven.

## Released full-gate checkpoint and streamed tool signature bytes

The complete default release gate finished successfully on clean commit
`9a7e8e9283244e3535da02fa07fac54653bb7eb1` with actual exit status zero:

```bash
rtk proxy bash scripts/verify-release-parity.sh > /tmp/gdx-04384-9a7e8e9-full-release-parity.log 2>&1
```

That run used the pinned source/binary/manifest checksums, synthetic accounts
and isolated loopback services. It completed all scripted phases, including
the then-current 51 DeepSeek and 44 Gemini scenarios, CLI/context/profile/
history, WebSocket transport/lifecycle/lineage, continuation, durable ownership,
HTTP keepalive/recovery, prompt-cache, raw 429 and profile-status policy gates.
The opt-in official Codex app-server executable checks were not enabled.
This checkpoint applies to that exact commit and scripted coverage, not a
claim of complete 1:1 parity or validation of native operating-system lanes.

On clean `71bbd547b7f65705124499a6ec729ed6dd7bdc88`, the extended Gemini
fail-before run finishes all 46 cases. Only the two new BOM cases fail Godex;
Prodex independently meets both exact failure contracts. Report:
`/tmp/gdx-04384-provider-bom-gemini-before-oracle.json`.

The tagged DeepSeek stream tool projection preserves the original signature
string. Its ThoughtSignature policy rejects only absent/blank values; accepted
strings flow unchanged through item completion and conversation snapshots.
Godex instead trimmed accepted signatures, changing opaque provider data.
The existing private gateway extractor now uses trimming only for the
blankness check and returns the original nonblank value. No model, shared
helper, API, dependency, option, goroutine or storage format is introduced.

Fail-before `/tmp/gdx-04384-tool-signature-before.log` passes the blank-update
control and fails raw-byte extraction and completed-item/snapshot tests.
The pinned executable separately preserves a tab/space/Unicode/newline
signature across its entire five-event response while baseline Godex trims
it. Report: `/tmp/gdx-04384-tool-signature-before-oracle.json`.
The extended oracle checks every event byte and rejects matching trimmed
signatures, renamed fields, incorrect sequences and changed tool identities.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 731 tests. `rtk go test ./...` passes 9,104 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the shuffled race
suite, cross-platform builds and test compilation. Logs:
`/tmp/gdx-04384-tool-signature-race.log`,
`/tmp/gdx-04384-tool-signature-test-all.log` and
`/tmp/gdx-04384-tool-signature-verify.log`. Clean-commit executable validation
follows this commit. Missing/present tool-call identity fields, padded names,
and buffered malformed-argument diagnostics remain separate audit targets.

## Closing HTTP connections retain complete captured bodies

While validating the tool-ID candidate, `rtk proxy make verify` failed the
existing `TestProxyForwardsCapturedBodyAfterClientHalfClose` regression with an
empty HTTP 200 instead of the upstream status/body. Failed gate log:
`/tmp/gdx-04384-stream-tool-id-verify.log`. The same race reproduces on clean
baseline `f859439626cfcd9c4e3dc45373fbae73b44dfc61`: 147/150 passes and
three failures under `rtk go test -race ./internal/delivery/http/proxy -run '^TestProxyForwardsCapturedBodyAfterClientHalfClose$' -count=150`.
Log: `/tmp/gdx-04384-half-close-repro-baseline.log`.

Go can cancel a closing server request's context on TCP read EOF while the
body-capture goroutine has completed but not delivered its result. Delivery
previously detached this cancellation only after capture, so the select could
discard an otherwise complete body. Closing server connections now detach
before capture while keeping the local request budget and any earlier parent
deadline. Ordinary cancellation and incomplete-body rejection retain their
existing controls. The existing private bounded-read implementation moves to
`delivery/http/proxy/request_body.go`; it has one transport-specific consumer,
so no shared helper is introduced. Gateway, use-case, routing and persistence
ownership are unchanged. No new production API, option, dependency or worker
is introduced. Architecture documentation records this transport boundary.

The unchanged half-close regression passes all 150 repetitions after the fix:
`/tmp/gdx-04384-half-close-repro-fixed.log`. Thirty repetitions of complete/
incomplete-body and closing/normal deadline controls pass 180 tests, and thirty
ordinary upstream/body-cancellation controls pass 60 tests. Logs:
`/tmp/gdx-04384-half-close-fixed-controls.log` and
`/tmp/gdx-04384-half-close-cancel-controls.log`.
The parent-deadline regression uses explicit cancellation and retained deadline
values without sleeps; closing and normal stalled capture still expire under
the owning watchdog.

An isolated released-executable prototype issued 150 independent client turns,
each with TCP CloseWrite before reading its response. Both binaries delivered
all expected responses with 150 upstream attempts and zero retries in the
updated probe, but Prodex also created durable state files not classified by
that prototype's ephemeral fixture validator. Report:
`/tmp/gdx-04384-half-close-baseline-oracle.json`. That FAIL is not an
executable half-close mismatch or a qualified completion gate; it requires a
separate persistence audit. The full prototype is retained in
`/tmp/gdx-04384-half-close-oracle-probe-work`, outside the production change.
No assertion is disabled and the state error is not treated as passing.
The initial full-test prototype also had an invalid one-product test fixture;
its failed log is retained at `/tmp/gdx-04384-half-close-test-all.log`.

The final production-only change passes `rtk go test ./...`: 9,107 tests across
81 packages, logged at `/tmp/gdx-04384-half-close-production-test-all.log`.
`rtk proxy make verify` passes all local gates, including the full shuffled race
suite and cross-platform build/test compilation:
`/tmp/gdx-04384-half-close-production-verify.log`. Clean-commit provider and
HTTP keepalive regression gates follow this commit. The prototype's additional
Prodex persistence files remain an open audit item.

## Preserve present empty and whitespace-bearing stream tool IDs

Clean BOM commit `12e94e87c9769ff4e736af292f9e3014fe4f4242` passed
56/56 DeepSeek and 46/46 Gemini released scenarios and mutation controls before
its fast-forward push to `origin/main`. Reports:
`/tmp/gdx-04384-provider-bom-oracle.json` and
`/tmp/gdx-04384-provider-bom-gemini-oracle.json`.
Clean signature commit `f859439626cfcd9c4e3dc45373fbae73b44dfc61` passed
57/57 DeepSeek scenarios and mutation controls before its fast-forward push.
Report: `/tmp/gdx-04384-tool-signature-oracle.json`.

The tagged stream tool projection preserves every present string ID, including
empty/whitespace values, and replaces an earlier ID when a later string arrives.
Godex ignored blank IDs and trimmed others. Its private DeepSeek gateway
accumulator now accepts the original string on type presence. Absent and
non-string IDs retain the existing UUIDv7 fallback or prior ID. No model,
helper, API, dependency, option, goroutine or storage format changes.

An initial fallback probe expected the lower-level provider-core helper's
deterministic request/index ID. The released executable rejected that expected
fixture: `RuntimeDeepSeekToolCall::default` already assigns a UUIDv7 before
the helper could be used. The initial missing-ID probe is not mismatch evidence.
The corrected missing-ID positive control validates the stable UUIDv7 namespace,
version and bytes across all five events and passes both unmodified binaries:
`/tmp/gdx-04384-stream-tool-id-missing-control-oracle.json`.
The existing generated-ID production path is unchanged.

Confirmed fail-before `/tmp/gdx-04384-stream-tool-id-confirmed-before.log`
passes five controls and fails five tests, including present empty/space/padded
IDs in actual pumps and later empty-ID replacement. Missing/null/numeric IDs
and maximum unsigned request/tool indices retain valid generated UUIDv7s.
The source-pinned empty and padded executable cases independently satisfy
the complete five-event contract on Prodex and fail only Godex. Reports:
`/tmp/gdx-04384-stream-tool-id-empty-before-oracle.json` and
`/tmp/gdx-04384-stream-tool-id-padded-before-oracle.json`. Independent mutation
controls reject altered identity, tool type, sequence and event presence.

This change is based on the separate half-close fix
`43cd08b931829db2086186aaa37ec1d58d0a8ffe`, whose clean candidate passed
57/57 DeepSeek executable cases and the HTTP keepalive phase before its push.
Reports: `/tmp/gdx-04384-half-close-production-oracle.json` and
`/tmp/gdx-04384-half-close-http-keepalive.log`. The initial tool-ID gate failure
is retained; it is not described as passing. After the transport fix,
`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 742 tests, `rtk go test ./...` passes 9,118 tests across 81 packages,
and `rtk proxy make verify` passes all local gates, including the full shuffled
race suite. Logs: `/tmp/gdx-04384-stream-tool-id-final-race.log`,
`/tmp/gdx-04384-stream-tool-id-final-test-all.log` and
`/tmp/gdx-04384-stream-tool-id-final-verify.log`. Clean-commit executable
tool-ID validation follows this commit.

The retained delayed half-close diagnostic now supplies executable fail-before
evidence as well: the baseline Godex stops after 82 client turns with an empty
HTTP 200 while Prodex completes all 150. On clean `43cd08b`, both binaries
complete 150 turns with matching status, headers, payload and attempt count.
Reports: `/tmp/gdx-04384-half-close-state-diagnostic.json` and
`/tmp/gdx-04384-half-close-state-diagnostic-fixed.json`. Both diagnostic reports
remain FAIL because of the unresolved persistence comparison. No diagnostic
assertion is bypassed and neither report is a full parity pass.
Synthetic artifact roots are `/tmp/godex-differential-2497901154` and
`/tmp/godex-differential-2065269940`. Shape-only inspection finds a nonempty
Prodex response binding with hashed credential/endpoint identity in state and
continuation sidecars; Godex does not write those files in this raw-key flow.
Restart behavior and the correct durable compatibility contract remain open.

## Stream tool names retain presence, classification and namespace bytes

The tagged stream projection stores every present string name without trimming.
Special tool kinds require exact name bytes. Flat namespace splitting checks
that both sides of the final double-hyphen are nonblank, then returns their
original bytes. Exact `mcp__` names also accept a final double-underscore
separator immediately after that prefix. Godex trimmed the whole name,
ignored later blank names, and used incomplete namespace-side checks.

The existing private DeepSeek accumulator and stream namespace converter now
follow those contracts. The invalid-argument diagnostic alone trims the name,
as the tagged formatter does. Buffered namespace conversion keeps its separate
tagged contract. No shared helper, public API, package, dependency, option,
goroutine or state format is added; protocol policy remains in the owning
DeepSeek gateway.

Fail-before `/tmp/gdx-04384-stream-tool-name-before.log` fails all four local
regressions: byte-preserving namespace conversion, padded special-name
classification, later blank-name replacement, and trimmed diagnostic names.
Additional namespace evidence is retained in
`/tmp/gdx-04384-stream-tool-name-namespace-before.log`. Ordinary names and
absent/non-string replacement controls remain valid.

On clean baseline `0ed4186b17f4ffca8ef4d2b68a6fe7c0664909ef`, all three
new released cases independently meet the exact full-wire contract on Prodex
and fail only Godex. Reports:
`/tmp/gdx-04384-stream-tool-name-padded-before-oracle.json`,
`/tmp/gdx-04384-stream-tool-name-blank-namespace-before-oracle.json` and
`/tmp/gdx-04384-stream-tool-name-blank-late-before-oracle.json`.
The first two keep the full five-event function-call trace; the last emits
four events ending in one missing-name failure without completed tools or
response success. Mutation controls reject matching altered names, event
sequences and tool identities. Broader MCP namespace edge coverage is local
and source-aligned, not an additional released-executable namespace matrix.

Clean tool-ID commit `0ed4186b17f4ffca8ef4d2b68a6fe7c0664909ef`
passed 60/60 released DeepSeek cases and mutation controls before its
fast-forward push to `origin/main`. Report:
`/tmp/gdx-04384-stream-tool-id-oracle.json`.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 747 tests. `rtk go test ./...` passes 9,123 tests across 81 packages.
`rtk proxy make verify` passes all local gates, including the shuffled race
suite and cross-platform build/test compilation. Logs:
`/tmp/gdx-04384-stream-tool-name-race.log`,
`/tmp/gdx-04384-stream-tool-name-test-all.log` and
`/tmp/gdx-04384-stream-tool-name-verify.log`. Clean-commit released DeepSeek
and Gemini validation follows this commit. Raw-key binding persistence and
restart identity matching remain the next separate audit target.


## Raw-key continuation persistence audit

Clean stream-name candidate `c18c799a829397708d67ff6493f9d9d86faed742`
passes 46/46 Gemini executable cases, but its 63-case DeepSeek gate fails
`sse-large-packet.prodex.state_integrity`: the reference background save can
write raw-key response bindings after a longer stream. Reports:
`/tmp/gdx-04384-stream-tool-name-oracle.json` and
`/tmp/gdx-04384-stream-tool-name-gemini-oracle.json`. The failed DeepSeek gate
was retained and the candidate was withheld from `origin/main`.

The short-run assumption that every raw-key binding is volatile was incomplete.
The pinned continuation journal rewrites the unavailable `local` profile as the
existing hard-binding conflict sentinel. Dispatch also compares a raw-key
binding with its selected local-profile identity before credential selection.
The released executable rejects this workflow even with unchanged keys and
endpoint; matching key material alone does not authorize continuation.

The differential now checks exact observed response IDs, credential-slot
identity digests and the canonical synthetic loopback endpoint digest in each
reference state/continuation/journal snapshot and recovery copy. Godex snapshots
may contain only the existing conflict sentinel for those observed response
hashes. Unknown owners, profiles, keys, providers, endpoints, fields and corrupt
snapshots remain failures. The original no-evidence audit still rejects every
nonempty affinity snapshot. Backoff, health, history, secret and file checks
remain enforced. These checks compare durable ownership semantics, not private
Rust/Go file-layout equality; the production normalization policy is not copied
into the test oracle.

Two synthetic executable scenarios cover same-process continuation and six
restart phases: rotated seed, same-key resume, changed-key resume,
changed-endpoint resume, fresh request and unknown-response discovery. Seed
waits for an observed durable binding, bounded at 15 seconds; a timeout is not
persistence evidence. The restart oracle requires that evidence, exact HTTP
503/code/message/exit 2 and zero attempts for all three known continuations,
plus successful fresh/discovery controls. Independent mutations reject altered
responses, ownership, attempt counts, state and missing persistence.

Fail-before reports on clean `c18c799`:
`/tmp/gdx-04384-raw-key-confirmed-before-oracle.json` and
`/tmp/gdx-04384-raw-key-in-process-confirmed-before-oracle.json`.
All reference phases satisfy the stricter state and behavior contracts. Godex
instead returns 200 and attempts a credential after every restart resume, and
makes a third upstream attempt within the original process where Prodex stops
after the two seed attempts. These are confirmed routing mismatches, beyond
file-layout differences. Production repair and clean-commit gates follow.


## Raw-key continuations fail before credential dispatch

`usecase/routing` now resolves raw-key response/turn/session bindings before
ordinary managed-owner recovery. Known bindings return the reference HTTP 503
message without retry or another credential attempt. Fresh and unknown-response
requests retain bounded discovery. Verified raw-key observations use the
existing conflict sentinel with hashed continuation keys in the existing
version-1 routing snapshot/backup; no raw credential, credential ID, endpoint,
new field or migration is introduced. Legacy synthetic-account owners are
still ignored without modifying their records, and managed-profile bindings
and private turn-state sidecars retain their existing boundaries.

The obsolete volatile-only observation function is removed. The new private
owner-resolution policy belongs to `usecase/routing`; HTTP JSON code/presentation
belongs to `delivery/http/proxy`, and the existing repository remains responsible
for locking, atomic snapshots and recovery. No shared helper, dependency,
exported API, option or background worker is introduced. Architecture, rotation
and user documentation record the compatibility behavior, and the historical
0.437.0 volatility assumption points to this re-audit.

Exact fail-before `rtk go test ./internal/usecase/routing ./internal/delivery/http/proxy -run '^TestProdex04384RawKeyContinuation' -count=1`
on isolated `c18c799` fails response/session/turn controls with an extra upstream
attempt and the HTTP integration with 200 instead of 503. Log:
`/tmp/gdx-04384-raw-key-continuation-confirmed-before.log`.
The updated tests also cover same-process/restarted routers, changed credential
IDs/endpoints, fresh and unknown-response requests, the exact HTTP error
code/body, durable markers, managed ownership and legacy-binding safety.

`rtk go test ./tools/differential -count=1` passes 243 tests, including strict
snapshot identity checks and independent oracle mutations. Log:
`/tmp/gdx-04384-raw-key-oracle-tests.log`. `rtk go test ./...` passes 9,137 tests
across 81 packages, logged at
`/tmp/gdx-04384-raw-key-continuation-final-test-all.log`.
The focused routing/repository/HTTP/oracle race suite passed 2,410 tests before
one additional oracle self-test was added. Log:
`/tmp/gdx-04384-raw-key-continuation-final-race.log`.
`rtk proxy make verify` passes formatting, source-size/TUI guards, vet, the
full shuffled race suite, cross-platform build/test compilation, installers
and build. Log: `/tmp/gdx-04384-raw-key-continuation-final-verify.log`.
Clean-commit released-executable validation follows this candidate.
No whole-project 1:1 parity claim is made.
