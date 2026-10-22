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


## DeepSeek raw-key continuations fail before credential dispatch

`usecase/routing` now resolves DeepSeek raw-key response/turn/session bindings before
ordinary managed-owner recovery. Known bindings return the reference HTTP 503
message without retry or another credential attempt. Fresh and unknown-response
requests retain bounded discovery. Verified DeepSeek raw-key observations use the
existing conflict sentinel with hashed continuation keys in the existing
version-1 routing snapshot/backup; no raw credential, credential ID, endpoint,
new field or migration is introduced. Legacy synthetic-account owners are
still ignored without modifying their records, and managed-profile bindings
and private turn-state sidecars retain their existing boundaries.

The initial candidate removed the volatile observation function; the final scope
correction below retains it for unaudited provider continuation workflows. The new private
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


## Audit accepted previous-response identities and shared file policy

The first clean `8edd81124c75ff7de4c65f45a531a41d0e638b17` full DeepSeek
run finishes 65 cases but remains FAIL on the unknown-response discovery
phase's state audit. All wire, status, attempt, credential and other scenario
contracts match. Report: `/tmp/gdx-04384-raw-key-continuation-oracle.json`.
The corresponding Gemini executable gate passes 46/46 cases and mutations:
`/tmp/gdx-04384-raw-key-continuation-gemini-oracle.json`.

The reference accepted-binding recorder also owns the successfully accepted
request's previous_response_id. That ID was omitted from the new oracle's
response-only evidence. The discovery probe now waits for the exact requested
ID to appear in durable state on both products, bounded at 15 seconds, and
requires that evidence. It authorizes that one request ID only after observing
the successful fixture response and its exact credential/endpoint owner.
Persistence polling also checks the exact Godex key hash; another response's
marker cannot satisfy it.

The first delayed proof confirms all twelve persistence/behavior observations,
but still fails the separate state comparison because its old duplicated file
allowlist rejects sidecars already validated by the strict content auditor.
Report: `/tmp/gdx-04384-accepted-previous-evidence-oracle.json`. The signature
now reuses the auditor's bounded filename policy, removing the duplicated list.
Unknown files, forged temp/lock names, payload corruption, credential/profile
owners and divergent recovery copies remain failures. No integrity assertion
or production routing behavior is removed or weakened.

The final delayed proof passes on the exact clean `8edd811` product binary:
`/tmp/gdx-04384-accepted-previous-final-evidence-oracle.json`. Both products
persist the accepted unknown request ID, reject all three known restart
continuations without upstream attempts, and accept fresh/discovery controls.
`rtk go test -race ./tools/differential -count=1` passes 244 tests, including
shared-filename spoofing and exact-key persistence controls. Log:
`/tmp/gdx-04384-accepted-previous-final-oracle-tests.log`.
The earlier failed reports remain failed evidence. `rtk go test ./...` passes
9,138 tests across 81 packages: `/tmp/gdx-04384-accepted-previous-test-all.log`.
Final local and clean-commit executable gates follow this oracle correction.


## Preserve local DeepSeek routes before binding validation

Clean `664ac9165861d12c2cc38486bb6e20cb85b91731` passes all 65 DeepSeek
scenarios and independent mutations. Report:
`/tmp/gdx-04384-accepted-previous-oracle.json`. Its full local gate also passed:
`/tmp/gdx-04384-accepted-previous-verify.log`. This was not pushed: an additional
pipeline audit found local Compact and Models dispatch occurs before provider
binding validation. The broader exact-output probe proves the reference accepts
bound context there, while the initial candidate returns 503. Reports:
`/tmp/gdx-04384-local-route-probe.json` and
`/tmp/gdx-04384-local-route-confirmed-before-oracle.json`.
All three new reference controls satisfy their exact payload contracts; only
Godex's Compact, Models list and single-model phases fail. Existing response
rejection, restart, fresh/discovery, ownership and sidecar assertions remain.

Routing now bypasses binding validation and observation only for the verified
DeepSeek raw-key local routes. They write no continuation keys or session
markers. Exact Compact output and all four degradation headers, the full
four-model list, and the single-model object are pinned as synthetic fixtures;
mutations reject altered status, exit, body bytes and headers. The shared
response binder receives the request model instead of separate transport flags,
keeping route policy in the owning routing use case. No public API, shared
helper, dependency, option or persisted field is added.

This repair is scoped to DeepSeek's raw-key contract. The existing credential-ID
cache retains provider kind, and other raw-key providers keep their previous
volatile observations. Gemini Compact uses its own upstream semantic workflow;
its complete continuation/identity pipeline is a separate audit target, not
covered by the DeepSeek local-route result. Managed-profile recovery, legacy
synthetic-owner filtering, retry quarantine and private turn-state rules retain
their existing controls.

Exact fail-before `rtk go test ./internal/delivery/http/proxy -run '^TestProdex04384RawKeyContinuationHTTPRejectionSurvivesRestart$' -count=1`
fails the added Compact control with 503. Log:
`/tmp/gdx-04384-raw-key-local-routes-before.log`.
Focused raw-key regression `rtk go test ./internal/usecase/routing ./internal/delivery/http/proxy -run '^TestProdex04384RawKeyContinuation' -count=1`
passes five tests after repair. Log:
`/tmp/gdx-04384-raw-key-local-routes-focused.log`.
`rtk go test -race ./internal/usecase/routing ./internal/delivery/http/proxy ./tools/differential -count=1`
passes 2,352 tests; `rtk go test ./...` passes 9,141 tests across 81 packages.
Logs: `/tmp/gdx-04384-raw-key-local-routes-final-race.log` and
`/tmp/gdx-04384-raw-key-local-routes-final-test-all.log`.
The final scope/legacy/HTTP/oracle control run passes twelve tests:
`/tmp/gdx-04384-raw-key-local-routes-final-controls.log`.
`rtk proxy make verify` passes every local gate, including the full shuffled
race suite, cross-platform build/test compilation, installers and build. Log:
`/tmp/gdx-04384-raw-key-local-routes-final-verify.log`.
Clean-commit release gates follow.

The next separate confirmed gap is buffered malformed-tool JSON diagnostics.
A released synthetic `{bad` response returns the pinned serde diagnostic on
Prodex but Go's decoder diagnostic on Godex, with one upstream attempt and
HTTP 200/failed Responses on both. Exploratory report:
`/tmp/gdx-04384-buffered-tool-invalid-confirmed-probe.json`. Its generic success
invariant is intentionally still FAIL; this is diagnostic mismatch evidence,
not a qualified parity gate or a production fix.


## Shared Compact wire compatibility

The default release gate on clean `bd08991a22594e8fcde7bd4bef383b93812014a3`
completed preflight tests/oracles but failed the extended DeepSeek matrix's
local Compact phase. It returns 200 with no upstream attempt on both products,
but Godex uses `X-Godex-Compact-*`, `Local Godex ...` summary text and Go map
field order, while the reference uses `X-Prodex-Compact-*`, `Local Prodex ...`
and the Mojo envelope order. Command:
`rtk proxy bash scripts/verify-release-parity.sh > /tmp/gdx-04384-raw-key-local-routes-full-release-parity.log 2>&1`.
Exit status is 1; this run is not a full release pass. Extracted matrix report:
`/tmp/gdx-04384-raw-key-local-routes-full-deepseek-report.json`.

The shared protocol owner `gateway/compact` now emits the reference header
namespace, fallback summary/malformed-input text and ordered envelope. Both
semantic and local responses use its existing private envelope builder, removing
the duplicated semantic map construction. String escaping reuses the existing
`helper/jsonutil` writer: raw valid UTF-8/HTML/U+2028/U+2029 and JSON control
escapes match `json_sink.mojo`; no new shared helper or dependency is introduced.
Provider gateways still own invocation and routing policy is unchanged.

Every current caller was traced: Anthropic, DeepSeek, Gemini and Kiro fallback,
plus Gemini/Kiro semantic Compact. Their integration expectations now require
the compatibility headers. A focused exact-wire regression covers all four
provider labels, all degradation headers and semantic Unicode/control/field
ordering. It fails all six tests before repair:
`/tmp/gdx-04384-compact-wire-before.log`. Focused
`rtk go test ./internal/gateway/compact ./internal/gateway/deepseek ./internal/gateway/claude ./internal/gateway/gemini ./internal/gateway/kiro -count=1`
passes 1,462 tests: `/tmp/gdx-04384-compact-wire-focused.log`.
The existing byte-exact local-route executable oracle remains unchanged.
`rtk go test -race ./internal/gateway/compact ./internal/gateway/deepseek ./internal/gateway/claude ./internal/gateway/gemini ./internal/gateway/kiro -count=1`
passes 1,462 tests; `rtk go test ./...` passes 9,147 tests across 81 packages.
Logs: `/tmp/gdx-04384-compact-wire-race.log` and
`/tmp/gdx-04384-compact-wire-test-all.log`.
`rtk proxy make verify` passes every local gate, including the full shuffled
race suite, cross-platform builds/test compilation, installers and build.
Log: `/tmp/gdx-04384-compact-wire-verify.log`.
Clean-commit executable gates follow.


## Buffered tool object-syntax errors use tagged serde diagnostics

The pinned provider-core buffered tool normalizer reports serde_json errors
when the authoritative Mojo kernel rejects malformed arguments. Godex instead
returned Go decoder text for ordinary tools and generic `invalid JSON` for
`tool_search`. Its private gateway error formatter now maps the three verified
object-syntax error contexts to serde's key/colon/separator/trailing-comma
messages and counts UTF-8 bytes for line/column locations. Accepted arguments
and stream validation are unchanged. Other diagnostic classes retain their
existing behavior and remain a separate audit target.

The formatter stays private to `gateway/deepseek`, where two existing buffered
argument-error paths consume it. It runs only after validation fails; there is
no extra valid-path parsing, shared helper, exported API, dependency, setting,
state format or worker. Architecture documentation records this protocol owner.

Before repair, `rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedToolObjectSyntax' -count=1`
passes the valid-argument control and fails all 21 ordinary/search/padded-name
object-error cases. Log: `/tmp/gdx-04384-buffered-tool-object-before.log`.
The expected messages, byte columns and Unicode/newline positions were also
checked with an offline Rust probe pinned to serde 1.0.229/serde_json 1.0.154.
This probe is source-aligned evidence, not a released-executable claim by itself.

Six independent executable fail-before cases use exact HTTP 200/failed
Responses envelopes, usage, metadata, exit 0, one upstream attempt, exact
credential/route/request, stdout and safe durable-state checks. All six meet
that contract on Prodex and fail only Godex on clean `bd08991`:
`/tmp/gdx-04384-buffered-tool-object-key-before-oracle.json`,
`/tmp/gdx-04384-buffered-tool-object-comma-before-oracle.json`,
`/tmp/gdx-04384-buffered-tool-object-colon-before-oracle.json`,
`/tmp/gdx-04384-buffered-tool-object-separator-before-oracle.json`,
`/tmp/gdx-04384-buffered-tool-object-search-key-before-oracle.json` and
`/tmp/gdx-04384-buffered-tool-object-padded-key-before-oracle.json`.
Mutations reject changed bodies, stdout, status, child exit, attempts, retries
and state even when both product traces are mutated alike. An envelope
regression retains prior text and valid tools, records one exact failure, and
suppresses later tools.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 783 tests. `rtk go test ./...` passes 9,166 tests across 81 packages.
Logs: `/tmp/gdx-04384-buffered-tool-object-race.log` and
`/tmp/gdx-04384-buffered-tool-object-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full
shuffled race tests, cross-platform builds/test compilation, installer checks
and build. Log: `/tmp/gdx-04384-buffered-tool-object-verify.log`.
A further same-line Unicode-byte-column control agrees with the locked Rust
probe and passes all 26 focused syntax cases:
`/tmp/gdx-04384-buffered-tool-object-unicode-controls.log`.
Final `rtk go test ./...` passes 9,169 tests across 81 packages after that
additional control: `/tmp/gdx-04384-buffered-tool-object-final-test-all.log`.
Clean-commit executable gates follow this candidate.


## Complete bounded response captures retain exact framing

Combined `869ca483c6ce83816fd264dc81311a5d2d5bf54c` passes 9,175 tests and
`rtk proxy make verify`. Logs:
`/tmp/gdx-04384-compact-buffered-combined-test-all.log` and
`/tmp/gdx-04384-compact-buffered-combined-verify.log`.
The extended restart probe now matches every Compact/model payload, but its
strict header comparison still fails Models list: Prodex emits Content-Length
2864, while Go switches to chunked framing beyond its automatic buffer size.
Report: `/tmp/gdx-04384-compact-buffered-raw-key-oracle.json` (FAIL).
The six buffered object-error cases on clean `a3466b9` all meet their exact
contracts; that candidate's complete 71-case report remains FAIL on its
unfixed Compact parent: `/tmp/gdx-04384-buffered-tool-object-oracle.json`.

HTTP delivery now derives Content-Length when its own existing bounded capture
proves EOF. The upstream framing value is still discarded. Prior prefixes,
partial captures, streams, bodyless statuses and trailer-bearing responses do
not gain an inferred length. No extra body read, buffer, gateway/model field,
shared helper, option or worker is introduced. Framing remains in
`delivery/http/proxy`; routing, credential selection and persistence are unchanged.

The existing framing regression now exercises the exact inspection boundary
with short and larger JSON bodies, plus an intentionally false upstream length.
Before repair, the short control passes and all three larger bodies report
length -1: `/tmp/gdx-04384-buffered-framing-before.log`.
`rtk go test -race ./internal/delivery/http/proxy -count=1` passes 343 tests,
including partial-body, trailer, stream and half-close controls. Log:
`/tmp/gdx-04384-buffered-framing-race.log`. `rtk go test ./...` passes 9,179
tests across 81 packages: `/tmp/gdx-04384-buffered-framing-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full
shuffled race tests, cross-platform build/test compilation, installers and build.
Log: `/tmp/gdx-04384-buffered-framing-verify.log`.
Clean-commit executable gates follow.


## Validated pushed checkpoint: 6695c9b

Clean `6695c9b0e05788201b7af5e04397a34e80a7bd33` passes 71/71 released
DeepSeek scenarios and independent mutations, including all eighteen restart,
identity-change, local-route and fresh/discovery observations. It also passes
46/46 Gemini cases and their independent mutations. Reports:
`/tmp/gdx-04384-buffered-framing-oracle.json` and
`/tmp/gdx-04384-buffered-framing-gemini-oracle.json`.

The exact clean-commit HTTP release phases also pass with exit zero:

```bash
rtk proxy bash scripts/verify-release-parity.sh --phase=http-keepalive > /tmp/gdx-04384-buffered-framing-http-keepalive.log 2>&1
rtk proxy bash scripts/verify-release-parity.sh --phase=http-compact > /tmp/gdx-04384-buffered-framing-http-compact.log 2>&1
```

The earlier default run on `869ca483` remains FAIL on the now-fixed Models-list
Content-Length mismatch:
`/tmp/gdx-04384-compact-buffered-full-release-parity.log`. It is not reported
as a complete release pass. Native macOS/Windows execution, live authentication,
other providers' raw-key continuation identities and additional serde diagnostic
classes remain outside this checkpoint's executable proof. No full 1:1 parity
claim is made.

After checking the primary worktree and unchanged remote, all eight validated
commits from `c18c799` through `6695c9b` were fast-forwarded and pushed directly
to `origin/main`. The primary worktree is clean at this checkpoint. No force
push, tag, release or unrelated editor-worktree change occurred.


## Validate DeepSeek continuation components before dispatch

A separate raw-key probe proves that an oversized nonblank previous_response_id
returns HTTP 503/exit 2 with zero upstream attempts on Prodex, while Godex
returns 200 after one attempt. Diagnostic report:
`/tmp/gdx-04384-raw-key-invalid-previous-probe.json` (FAIL, its generic success
invariant is not a qualified rejection gate). The pinned
`RuntimeHardBindingIdentity::new` rejects any invalid nonblank response, turn
or session component. The established bound is 1,024 UTF-8 bytes, with C0/C1
control characters forbidden.

The private DeepSeek raw-key resolver now applies the existing component
validator before owner lookup or credential selection. Invalid requests use
the same exact `bound_continuation_unavailable` rejection as unavailable owners.
Local Compact/Models still dispatch earlier. No new validator, helper, exported
API, dependency, setting, persisted field or worker is introduced. Policy stays
in `usecase/routing`; input/output models and repositories are unchanged.

Exact fail-before
`rtk go test ./internal/usecase/routing -run '^TestProdex04384RawKeyRejectsInvalidContinuationIdentity$' -count=1`
passes nine length/blank controls and fails nine invalid component cases plus
the parent test: `/tmp/gdx-04384-raw-key-identity-before.log`.
After repair all nineteen tests pass:
`/tmp/gdx-04384-raw-key-identity-focused.log`.
The cases cover response/session/turn, byte limits with ASCII and Unicode,
blank components and internal controls. Turn-header control cases are use-case
checks, not evidence that HTTP accepts illegal header bytes.

Eight strict executable cases independently verify exact client body/status,
Content-Type, stdout, child exit, credential slot, original request, zero retry,
upstream attempt count and safe durable state. On clean `6695c9b`, the 1,024-byte
ASCII/Unicode, blank and local-Compact controls all PASS:
`/tmp/gdx-04384-raw-key-identity-limit-before-oracle.json`,
`/tmp/gdx-04384-raw-key-identity-unicode-limit-before-oracle.json`,
`/tmp/gdx-04384-raw-key-identity-blank-before-oracle.json` and
`/tmp/gdx-04384-raw-key-identity-local-compact-before-oracle.json`.
The 1,025-byte ASCII, oversized Unicode, control-bearing response ID and mixed
valid-response/invalid-session cases all satisfy the reference rejection and
fail only Godex:
`/tmp/gdx-04384-raw-key-identity-over-limit-before-oracle.json`,
`/tmp/gdx-04384-raw-key-identity-unicode-over-before-oracle.json`,
`/tmp/gdx-04384-raw-key-identity-control-before-oracle.json` and
`/tmp/gdx-04384-raw-key-identity-mixed-before-oracle.json`.
The mixed case also prevents an invalid session from creating durable state.
Independent oracle mutations reject equal corrupt executions.
`rtk go test -race ./internal/usecase/routing ./internal/repository/routing ./internal/delivery/http/proxy ./tools/differential -count=1`
passes 2,440 tests. `rtk go test ./...` passes 9,199 tests across 81 packages.
Logs: `/tmp/gdx-04384-raw-key-identity-race.log` and
`/tmp/gdx-04384-raw-key-identity-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, the full
shuffled race suite, cross-platform build/test compilation, installers and build.
Log: `/tmp/gdx-04384-raw-key-identity-verify.log`.
Clean-commit executable gates follow.


## Buffered ordinary arguments follow the bounded structural view

Pinned `json_view.mojo` and buffered `response_tool_calls.rs` use Mojo's
structural view for ordinary/custom tool arguments. Primitive contents and
escaped bytes remain opaque. The root starts at depth zero with a 256 limit:
256 arrays around a scalar and 257 empty arrays pass; 257 arrays around a
scalar and 258 empty arrays fail. The Rust serde parser supplies diagnostics
only after the authoritative kernel rejects input. Structured `tool_search`
arguments cross an additional serde boundary and remain strict.

A private scanner in `gateway/deepseek/buffered_fragment.go` reproduces that
view without decoding JSON values. Its error path preserves the earlier serde
range/surrogate diagnostic before excessive nesting, or serde's recursion
failure at column 128. Existing object-syntax formatting is reused. No shared
helper, exported API, dependency, option, state change or worker is added.
Other lexical diagnostic classes remain a separate audit target.

The narrow fail-before regression passes structured-search controls and fails
ordinary opaque-value acceptance plus excessive-depth rejection:
`/tmp/gdx-04384-buffered-fragment-before.log`.
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384Buffered' -count=1`
then passes all 31 focused tests:
`/tmp/gdx-04384-buffered-fragment-focused.log`.
Expected composite surrogate/range/depth diagnostics also match an offline
Rust probe pinned to serde 1.0.229/serde_json 1.0.154; that is source-aligned
evidence, not an executable parity result by itself.

Ten strict released-executable cases were run against clean `9ed7963` using
`--scenario deepseek-buffered-view-<suffix>`. All independently require exact
client body/status/type, stdout, exit zero, one exact authenticated upstream
request, zero retries and safe durable state. The surrogate, depth-limit and
depth-empty controls PASS on both products. Literal, overflow, escape,
depth-over, early-surrogate, early-range and patch satisfy the reference
contract and FAIL only Godex. Reports:
`/tmp/gdx-04384-buffered-fragment-<suffix>-before-oracle.json`.
Independent mutations reject changed response bytes, stdout, retries and
attempts even when both traces are corrupted alike. The shared wire oracle
also retains its existing status, exit and state mutations.

`rtk go test ./tools/differential -count=1` passes 248 tests after correcting
a test-only mutation that previously targeted a nonexistent failed-call ID.
The retained earlier fixture failure is not reported as a product failure.
`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 792 tests: `/tmp/gdx-04384-buffered-fragment-race.log`.
`rtk go test ./...` passes 9,204 tests across 81 packages:
`/tmp/gdx-04384-buffered-fragment-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, the full
shuffled race suite, cross-platform build/test compilation, installers and build.
Log: `/tmp/gdx-04384-buffered-fragment-verify.log`.
Clean-commit executable gates follow.


## Validated pushed checkpoint: e9ccedc

Clean `e9ccedcdfe82afd01707a37260d79a1db6ebc1e1` passes all 89 released
DeepSeek scenarios and their independent mutation controls, including the ten
new bounded-view cases. Report:
`/tmp/gdx-04384-buffered-fragment-oracle.json`.
The primary worktree was clean and `origin/main` unchanged before fast-forward
integration and direct push. No force push, tag, release or unrelated editor
change occurred. The default full release gate is being rerun separately; this
89-case result is not a complete release or global parity claim.


## Buffered array and trailing-input syntax diagnostics

The next pinned serde probe and released-executable comparison identify three
more diagnostic contexts: array trailing comma, missing array separator and
trailing input after a complete value. Both ordinary and structured-search
tool arguments fail with the reference's exact syntax messages; Godex retained
Go decoder text or a generic structured-search error.

The existing private formatter gains three mappings. Its file is renamed from
`gateway/deepseek/tool_object_error.go` to `tool_argument_error.go` to describe
its broader syntax ownership. Accepted inputs, raw-view depth, streaming,
routing and state are unchanged. No new parser, helper, exported API,
dependency, option or worker is introduced.

`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedArrayAndTrailingSyntaxDiagnostics$' -count=1`
fails all eighteen tool/argument cases and their parent before repair:
`/tmp/gdx-04384-buffered-array-before.log`.
The cases cover ordinary lookup, apply-patch and structured search, mismatched
closers, Unicode byte columns, newline positions and string-valued elements.
`rtk go test ./internal/gateway/deepseek ./tools/differential -run '^TestProdex04384Buffered' -count=1`
then passes 52 tests, retaining accepted opaque/depth/search controls:
`/tmp/gdx-04384-buffered-array-focused.log`.

Six strict executable cases use array-comma, array-separator, trailing,
search-array-comma, search-array-separator and search-trailing suffixes. On
clean `e9ccedc`, every reference trace meets the exact expected failed envelope,
status/type, stdout, exit, authenticated original request, single attempt,
zero retries and safe durable-state contract; every Godex trace fails only its
response bytes and corresponding framing. Reports:
`/tmp/gdx-04384-buffered-<suffix>-before-oracle.json`.
The existing independent corruption tests automatically cover these fixtures.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 811 tests: `/tmp/gdx-04384-buffered-array-race.log`.
`rtk go test ./...` passes 9,223 tests across 81 packages:
`/tmp/gdx-04384-buffered-array-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, the full
shuffled race suite, cross-platform build/test compilation, installers and build.
Log: `/tmp/gdx-04384-buffered-array-verify.log`.
Clean-commit executable gates follow.


## Validated pushed checkpoint: d43855b

Clean `d43855b0225d4e08a38d27e03949ec8c2ecb07f4` passes all 95 released
DeepSeek scenarios and their independent mutation controls, including all six
array/trailing-input cases and the earlier bounded-view controls. Report:
`/tmp/gdx-04384-buffered-array-oracle.json`.
After checking the clean primary worktree and unchanged remote, it was
fast-forwarded and pushed directly to `origin/main`. No force push, tag,
release or unrelated editor change occurred.


## Gemini raw-key continuation component validation

The pinned local-rewrite dispatch validates nonblank response, turn and
session components before provider selection for Gemini as well as DeepSeek.
The same 1,024-byte UTF-8/C0/C1 invariant applies. Godex applied that guard only
to DeepSeek; invalid Gemini previous/session components could enter the normal
routing path, and invalid turns received the generic unknown-owner response.

The existing private raw-key pool predicate now takes its concrete provider
name. `usecase/routing` applies the existing validator to both supported pools
before owner lookup on provider routes. The second predicate consumer still
exempts only DeepSeek local-route response binding. No gateway, transport,
persistence, valid-continuation policy, shared helper, interface, dependency,
setting or worker is added. Gemini Compact/Models retain their existing route
order and are outside this validation change.

The corrected bounded fail-before test overlays only the expanded test file
onto clean `d43855b`, leaving that product source and binary untouched:
`rtk go test -overlay /tmp/gdx-04384-gemini-identity-before-overlay.json ./internal/usecase/routing -run '^TestProdex04384RawKeyRejectsInvalidContinuationIdentity$' -count=1`.
It passes 25 tests and fails nine invalid Gemini cases plus two parents:
`/tmp/gdx-04384-gemini-identity-bounded-before.log`.
The initial broader diagnostic log also exposed unbound valid Gemini turn
policy differences: `/tmp/gdx-04384-gemini-identity-before.log`. Those valid-turn
expectations were not treated as part of this component-validation regression.
After repair, the focused test passes all 36 tests:
`/tmp/gdx-04384-gemini-identity-focused.log`.

Five Gemini executable cases add ASCII and Unicode session byte-limit controls,
ASCII/Unicode overflow and an internal C0 control. They independently require
exact status/type/exit/stdout, the precise 503 JSON for invalid sessions, zero
upstream attempts on rejection, the exact authenticated request on acceptance
and safe durable state. Equal status, exit, type, body, stdout, state or attempt
corruption is rejected by self-checks also invoked by the executable oracle.
`rtk go test ./tools/differential -count=1` passes 249 tests.

`rtk go test -race ./internal/usecase/routing ./internal/repository/routing ./internal/delivery/http/proxy ./tools/differential -count=1`
passes 2,459 tests: `/tmp/gdx-04384-gemini-identity-race.log`.
`rtk go test ./...` passes 9,241 tests across 81 packages:
`/tmp/gdx-04384-gemini-identity-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, the full
shuffled race suite, cross-platform build/test compilation, installers and build.
Log: `/tmp/gdx-04384-gemini-identity-verify.log`.
The 249 oracle self-tests also pass after wiring identity mutations into the
executable negative controls: `/tmp/gdx-04384-gemini-identity-oracle-final-tests.log`.
Clean-commit executable gates follow.

Exploratory native and Chat-shaped Gemini probes also expose valid previous-ID
translation timeouts and Compact unknown-owner rejection. Reports:
`/tmp/gdx-04384-gemini-continuation-exploratory.json` and
`/tmp/gdx-04384-gemini-continuation-native-exploratory.json`.
They are diagnostic observations, not qualified parity passes. Native valid
continuation/replay and Compact ownership remain separate audit targets.


## Complete default release gate passes on e9ccedc

The unmodified default gate now exits zero on clean
`e9ccedcdfe82afd01707a37260d79a1db6ebc1e1`:

```bash
rtk proxy bash scripts/verify-release-parity.sh > /tmp/gdx-04384-buffered-fragment-full-release-parity.log 2>&1
```

The final HTTP profile/status and raw-429 tests after the script's all-phase
marker also complete successfully. This is a complete default-script PASS,
not an inferred combination of phase results. The earlier failed default
runs remain retained. This pass covers that pinned commit and the script's
synthetic/native Linux test surface; later Gemini identity cases and global
1:1 parity are not implied.


## Recognize the existing private conversation-selection lock

The first 51-case Gemini baseline report is FAIL:
`/tmp/gdx-04384-gemini-identity-before-oracle.json`. All five new cases flag
Godex's `state/routing-conversation.guard` as unknown. The two accepted
byte-limit controls otherwise match; the three invalid sessions additionally
show the expected product mismatch. Reference traces meet every invariant.
The 51-case candidate run on `0287578` used the same incomplete allowlist;
it fails only those two accepted controls and is retained as FAIL:
`/tmp/gdx-04384-gemini-identity-oracle.json`.

`repository/routing.Store.AcquireConversation` owns this first-owner lock.
`helper/lockfile` creates it empty with mode 0600 and never writes payload data.
The differential auditor now accepts this exact path only while it remains an
empty private regular file. Populated and public guards still fail, with the
existing Windows mode-bit exception; other file, secret and binding checks
are unchanged. No production policy or persistence format is changed.

The guard regression passes both rejection controls and fails the empty-lock
control before repair:
`/tmp/gdx-04384-gemini-guard-before.log`.
`rtk go test ./tools/differential -count=1` then passes 254 tests:
`/tmp/gdx-04384-gemini-guard-focused.log`.
The existing `--scenario` flag can now select one known Gemini case, while
unknown names fail before launching products. The default remains the full
matrix; model catalogs still require all cases and forbid DeepSeek workers.
No assertion is disabled for focused execution.

Five corrected strict baseline invocations on clean `d43855b` use
`--gemini --scenario raw-key-session-<suffix>`. ASCII and Unicode limit
controls PASS; over-limit, unicode-over and control satisfy the reference
contract and FAIL only Godex. Reports:
`/tmp/gdx-04384-gemini-session-<suffix>-before-oracle.json`.
The candidate `0287578` separately passes all 95 DeepSeek cases with independent
mutations: `/tmp/gdx-04384-gemini-identity-deepseek-oracle.json`.

`rtk go test ./...` passes 9,246 tests across 81 packages:
`/tmp/gdx-04384-gemini-guard-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-gemini-guard-verify.log`.
Clean-commit executable gates follow.


## Validated Gemini identity checkpoint: 625fac5

Clean `625fac5f7c88ebf8b05ed224592b2c589c8eae9f` passes all 51 Gemini
cases and all 95 DeepSeek scenarios. Both executable negative-control sets
PASS, including the new independent raw-key identity/stdout/state mutations.
The corrected auditor accepts both legitimate byte-limit controls and rejects
all three invalid-session requests exactly before any upstream attempt.
Reports:
`/tmp/gdx-04384-gemini-guard-oracle.json` and
`/tmp/gdx-04384-gemini-guard-deepseek-oracle.json`.
Both binaries were built from the clean commit and checked against the same
pinned 0.438.4 source, binary digest and exact Go VCS provenance.

The complete default-script PASS reported above applies to `e9ccedc`, not a
new default run on this checkpoint. Latest local verification is 9,246 tests
and `rtk proxy make verify` PASS. The retained intermediate FAIL reports
remain FAIL; no mismatch was silently discarded or converted into a pass.
Native macOS/Windows execution, live authentication, valid Gemini continuation
replay/Compact ownership and additional buffered lexical diagnostics remain
open. Global 1:1 parity is not claimed.

After checking the primary worktree and unchanged remote, the Gemini policy
and oracle commits were fast-forwarded and pushed directly to `origin/main`.
No force push, tag, release or unrelated editor-worktree change occurred.


## Buffered container/value EOF retains the first serde error

The pinned Rust diagnostic probe distinguishes EOF in a list, object or
value, and counts the complete consumed UTF-8 byte position including trailing
ASCII whitespace. A range/surrogate error encountered earlier remains the
reported failure. Godex instead emitted Go EOF text for ordinary arguments and
a generic error for structured search.

The existing private buffered serde token walk now retains its bounded
container stack and reports these EOF contexts. Structured search uses that
same first-error walk after admission fails. A private byte-position formatter
is reused by both buffered error paths and the existing object/array syntax
mapper. All accepted ordinary opaque fragments and strict-search admission
remain unchanged. There is no new parser on valid ordinary inputs, shared
helper, exported API, dependency, setting, state field or worker. Quoted-string
EOF and other lexical classes remain separate audit targets.

The shared first-error walk is explicitly gated on structural kernel rejection.
A review control preserves the caller's outer error cause for kernel-accepted
overflow, surrogate and deep arguments. It rejects the initial unguarded
prototype: `/tmp/gdx-04384-buffered-eof-outer-before.log`.

`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedContainerEOFUsesFirstSerdeDiagnostic$' -count=1`
before repair passes four prior-error controls and fails 26 tool/argument
cases plus their parent: `/tmp/gdx-04384-buffered-eof-before.log`.
The locked Rust probe and Go token observations are retained at
`/tmp/gdx-04384-buffered-eof-diagnostic-probe.jsonl` and
`/tmp/gdx-04384-buffered-eof-go-token-probe.log`.
They establish source semantics, not released-executable parity by themselves.
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384Buffered' -count=1`
after repair passes 81 tests:
`/tmp/gdx-04384-buffered-eof-focused.log`.

Seven strict executable baseline cases use eof-list, eof-object, eof-value,
eof-array-value, search-eof-value, eof-early-range and eof-early-surrogate
suffixes on clean `625fac5`. Every reference trace satisfies the exact failed
Responses envelope, status/type, stdout, child exit, original authenticated
request, single attempt, zero retry and safe durable-state contract. The two
early-error controls PASS on both products; the other five fail only Godex.
Reports: `/tmp/gdx-04384-buffered-<suffix>-before-oracle.json`.
The shared independent corruption tests cover every new fixture.

`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 848 tests: `/tmp/gdx-04384-buffered-eof-race.log`.
`rtk go test ./...` passes 9,277 tests across 81 packages:
`/tmp/gdx-04384-buffered-eof-test-all.log`.
After adding and satisfying the outer-cause control, the same focused race
command passes 849 tests and the full suite passes 9,278 tests. Logs:
`/tmp/gdx-04384-buffered-eof-final-race.log` and
`/tmp/gdx-04384-buffered-eof-final-test-all.log`.
`rtk proxy make verify` on the final tree passes formatting, source/TUI guards,
vet, full shuffled race tests, cross-platform build/test compilation, installers
and build: `/tmp/gdx-04384-buffered-eof-final-verify.log`.
Clean-commit executable gates follow.


## Validated buffered EOF checkpoint: 138ce84

Clean `138ce847179a9f203542296a174ee251b5e5b145` passes all 102 released
DeepSeek scenarios and independent status/header/event/state/retry/cancellation/
stdout mutations: `/tmp/gdx-04384-buffered-eof-oracle.json`.
The exact clean-commit binary/source provenance and pinned 0.438.4 checks are
retained in that report. Latest local checks are 849 focused race tests,
9,278 full-suite tests and final `rtk proxy make verify`, all PASS.
The most recent Gemini 51-case PASS remains on `625fac5`; the complete default
release gate PASS remains on `e9ccedc`. Neither is presented as a new run on
this EOF commit.

After checking the primary worktree and unchanged remote, this commit was
fast-forwarded and pushed directly to `origin/main`. No force push, tag,
release or unrelated editor-worktree change occurred.

A separate exploratory `tool_search` overflow probe records another boundary:
Prodex emits `DeepSeek Mojo returned invalid response tool-call JSON: number
out of range at line 1 column 95`, while Godex retains the generic malformed
argument message. The locked serde probe confirms column 95 for the kernel's
emitted envelope. Artifacts:
`/tmp/gdx-04384-search-output-overflow-exploratory.json` (FAIL with deliberately
unsuitable generic success expectations) and
`/tmp/gdx-04384-search-output-serde-probe.jsonl`.
This is diagnostic evidence, not a qualified parity pass or a production fix.

Before implementing that follow-up, account for the full pinned admission flow:
ResponseToolCallItem sums all optional input string byte lengths with a 16 MiB
aggregate bound. Call ID/name/signature each have a 4 MiB view bound; arguments
have a 16 MiB view bound. Its output allocation is six times input bytes plus
2,048. Kernel rejection uses the inner argument's serde diagnostic, while a
successful kernel's invalid structured output uses the outer envelope's
position and different prefix. The next strict oracle must distinguish those
paths and preserve ordinary opaque argument acceptance. Audit notes are saved
at `/tmp/gdx-04384-search-output-audit.txt`.

Global 1:1 parity remains unproven. Quoted-string/literal EOF, other lexical
classes, valid Gemini continuation/Compact ownership, native macOS/Windows
execution and live authentication remain open.


## Buffered runtime body gate and provider-core tool admission

The pinned `core_constants.rs` sets the public buffered response cap to 4 MiB.
`runtime_launch/proxy_startup/deepseek_rewrite/response.rs` reads the complete
body under that cap before parsing or provider-core translation. Godex instead
allowed 20 MiB. A body above 4 MiB could therefore produce a successful Godex
tool response where the release returns the terminal plain HTTP 502
`provider response could not be processed`.

Provider-core has a separate boundary. The pinned
`translators/deepseek/tooling/response_tool_calls.rs`, rich Rust bridge and
`deepseek_input_valid` Mojo contract admit name, raw call ID and raw signature
up to 4 MiB each, arguments up to 16 MiB, and aggregate input up to 16 MiB.
RTK wrapping happens before aggregate admission. An absent ID contributes zero
bytes, while a whitespace-only signature counts before being omitted from
output. Name size validation precedes blank-name rejection. Godex now follows
these checks in its private DeepSeek gateway without a new interface, shared
helper, dependency, configuration, state format or background worker.

Kernel rejection retains the existing inner-argument serde diagnostic; valid
JSON rejected only by bounds uses the exact bounded-kernel error. The distinct
kernel-accepted structured-output error remains an open follow-up. Existing
9–17 MiB metadata, RTK and tool-field assertions now exercise provider-core
conversion directly. The large-response regression separately requires public
runtime HTTP 502. No meaningful large-field assertion was removed.

The private fail-before tests catch oversized call ID/signature, aggregate
overflow with present or absent ID, wrapped RTK growth, earlier numeric error
precedence and oversized blank-name priority. Logs:
`/tmp/gdx-04384-tool-kernel-before.log` and
`/tmp/gdx-04384-tool-kernel-name-before.log`.
The public body regression also fails before repair:
`/tmp/gdx-04384-buffered-body-before.log`.
These larger provider-core tests establish source-aligned behavior, not
released-executable coverage beyond the public 4 MiB gate.

Four strict executable fixtures use ignored ASCII padding so the complete
serialized upstream body is 4 MiB minus one, exactly 4 MiB, or 4 MiB plus one.
Under/limit require the exact successful tool response; over/over-rotation
require plain HTTP 502, exit 2, exact stdout and content type, one original
authenticated request, zero retry and safe durable state. The last case
configures two synthetic keys and requires only the primary attempt. Fixture
self-tests assert exact wire size and unchanged original content. Independent
matching corruptions of status, body, content type, stdout, exit, state,
attempts, authentication, selected key and original request must fail.

Qualified baseline runs on clean `138ce847179a9f203542296a174ee251b5e5b145`
PASS under/limit and FAIL only Godex for over/over-rotation. Every reference run
meets the strict contract, including the two-key no-rotation check. Reports:
`/tmp/gdx-04384-buffered-body-<under|limit|over|over-rotation>-before-oracle.json`.
Earlier signature/aggregate executable prototypes assumed that the larger
provider-core limits were reachable through this public route; all failed
because the release's body gate fired first. Those reports remain FAIL:
`/tmp/gdx-04384-tool-kernel-<signature-limit|signature-over|aggregate-limit|aggregate-over>-before-oracle.json`.
They are not qualified evidence of provider-core executable behavior.

Before the final fixture size/authentication controls, the commands
`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
and `rtk go test ./...` pass 864 focused race tests and 9,293 tests across 81
packages. Logs: `/tmp/gdx-04384-tool-kernel-final-race.log` and
`/tmp/gdx-04384-tool-kernel-final-test-all.log`.
The first settled production-tree `rtk proxy make verify` also passes:
`/tmp/gdx-04384-tool-kernel-verify.log`.
After adding exact-size and authentication/key/request corruption controls,
`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 865 tests: `/tmp/gdx-04384-tool-kernel-reviewed-race.log`.
Final `rtk proxy make verify` passes formatting, source/TUI guards, vet, full
shuffled race tests, cross-platform build/test compilation, installers and
build: `/tmp/gdx-04384-tool-kernel-reviewed-verify.log`.
Clean-commit executable verification follows; no new global parity claim is made.


## Local buffered processing failures must not rotate provider credentials

The first 106-case run on clean `11203a0cd621da119c97af06e1fcdb694b816caa`
does not PASS. It aborts in `deepseek-buffered-body-over-rotation` because
Godex's Codex shim times out before receiving HTTP headers and writes no child
exchange. The report is empty, not a completed parity result:
`/tmp/gdx-04384-admission-oracle.json` and
`/tmp/gdx-04384-admission-oracle.stderr`.
An isolated repeat fails the same way:
`/tmp/gdx-04384-admission-rotation-isolated.stderr`.
A diagnostic harness retains the redacted child stderr and confirms the HTTP
client timeout: `/tmp/gdx-04384-admission-rotation-diagnostic.stderr`.
No timeout was counted as PASS or dismissed as a fixture problem.

The gateway correctly constructed the local processing HTTP 502 but omitted
its terminal routing evidence. With two keys, generic external-provider
classification treated it like an upstream 502, rotated, then waited after
repeated failures. The release's `local_rewrite_response.rs` constructs this
terminal translation response after upstream selection; it cannot retry it.

The private DeepSeek failure constructor now supplies the existing
`PrecommitFailure` model with code `provider_response_translation_failed`.
The existing use-case classification treats that non-retryable code as a
failed terminal response. It does not pretend bytes were committed, change
upstream-error retry policy or introduce a new model field/interface. Gateway
owns the translation-failure evidence; routing still owns the retry decision.
Helpers stay private in the owning gateway. State formats and credentials
remain unchanged.

A real HTTP gateway/pool/router regression uses two synthetic credentials.
It cancels on an unexpected extra attempt, without sleeps. Before repair,
malformed and oversized successful bodies fail by attempting another key;
the actual-upstream-502 rotation control passes. After repair all four tests
pass, preserving exact 502 text/type and failed outcome while the upstream
control still uses two attempts. Exact command:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedTranslationFailureDoesNotRotateCredentials$' -count=1`.
Qualified before/after logs: `/tmp/gdx-04384-body-terminal-qualified-before.log`
and `/tmp/gdx-04384-body-terminal-focused.log`.
The initial fixture omitted required account homes and failed all controls;
`/tmp/gdx-04384-body-terminal-before.log` is retained as an unqualified fixture
failure, not bug evidence.

`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,673 tests: `/tmp/gdx-04384-body-terminal-race.log`.
`rtk go test ./...` passes 9,298 tests across 81 packages:
`/tmp/gdx-04384-body-terminal-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-body-terminal-verify.log`.
Clean-commit executable verification follows before pushing either admission
commit; the failed 11203a0 matrix remains incomplete.


## Validated buffered admission checkpoint: 2a36859

Clean `2a36859568bf5c54ae9409c42f20cb64099100af` passes all 106 DeepSeek
executable cases and independent status/header/event/state/retry/cancellation/
stdout mutations: `/tmp/gdx-04384-terminal-oracle.json`.
All four exact-size buffered-body scenarios satisfy the reference and candidate
contracts, including the two-key terminal 502 with one primary attempt and no
retry. The isolated two-key run also PASSes:
`/tmp/gdx-04384-terminal-rotation-oracle.json`.
The earlier aborted 11203a0 run remains incomplete; it is not a parity pass.

The binaries were built from that clean commit with
`rtk go build -trimpath -o /tmp/gdx-04384-terminal-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-terminal-differential ./tools/differential`.
The full oracle uses `--scenario all --scenario-workers 3`, the pinned released
0.438.4 binary/source/digest and exact clean Godex VCS provenance retained in
the report. Latest local checks are 2,673 focused race tests, 9,298 full-suite
tests and `rtk proxy make verify`, all PASS.

This admission stage is ready for direct fast-forward push after checking the
primary worktree and remote. No force push, tag, release, dependency change or
editor-worktree modification is part of this stage. Latest Gemini 51-case PASS
remains on `625fac5`; complete default-script PASS remains on `e9ccedc`.
Structured search output-envelope diagnostics and runtime depth are in a
separate follow-up. Global 1:1 parity is not claimed.


## Structured search parses the kernel item and public response envelopes

The buffered admission commits and checkpoint were fast-forwarded and pushed
directly to `origin/main` at `bec259f6c33f1a0f22887c6e98e2c5958acd40c4`
after checking the clean primary worktree and unchanged remote. No CI wait,
force push, tag, release or editor-worktree modification occurred.

The pinned `response_tool_calls.rs` first admits the whole kernel input, then
parses the kernel's emitted item as serde JSON. For structured `tool_search`,
Mojo embeds arguments verbatim inside that item. Godex instead parsed only the
inner argument and either reported a generic malformed-argument error or
accepted nesting that the real outer parse rejects. The error prefix, byte
position and recursion boundary were therefore wrong.

Private `gateway/deepseek/search_output.go` now constructs that exact item
envelope and reuses the existing serde value decoder and canonical writer.
Its call ID uses Mojo's `call_0` fallback for missing/non-string IDs during
this parse; successful public output still uses the existing typed fallback.
The existing private error walk runs only after decoding fails, preserving
earlier numeric/surrogate diagnostics and the envelope's depth/byte position.
The private byte-position formatter has two real callers. Structural kernel
rejection remains on the inner-argument path. Ordinary opaque tools, streamed
search conversion, field/aggregate bounds and RTK admission stay unchanged.
No shared helper package, exported API, interface, dependency, configuration,
state format or worker was added. Other output lexical diagnostics retain an
explicit shortcut until their own pinned regressions are available.

The full public flow has another parse:
`provider_core_rewritten_json_value` reparses the serialized Responses envelope.
An item with 126 nested arrays is valid in isolation, but 125 or 126 arrays
exceed the public envelope's 127-container serde limit. The release's fallback
path also fails its required response parse, producing empty HTTP 500. Godex
now checks this runtime boundary before storing conversation ownership and
returns that exact empty terminal response without retry/rotation. It does not
panic. The accepted public control has 124 arrays; the separate provider-core
control still accepts 126 arrays. HTTP 502 body-size admission remains distinct.

The locked offline serde probe establishes overflow column 95, multiline
overflow line 2 column 5, surrogate column 98, early-surrogate column 103 and
item recursion column 217 for the fixed synthetic call ID. Separate probes
cover missing, empty, escaped and UTF-8 IDs. Artifacts:
`/tmp/gdx-04384-search-envelope-serde-probe.jsonl` and
`/tmp/gdx-04384-search-envelope-id-serde-probe.jsonl`.
They establish serde semantics, not executable parity by themselves.

Qualified fail-before command:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedSearchOutput' -count=1`.
It passes the boundary controls and fails all 11 diagnostic cases plus earlier
output retention: `/tmp/gdx-04384-search-envelope-qualified-before.log`.
The initial ID-column guesses were corrected against the locked probe before
that qualified run; `/tmp/gdx-04384-search-envelope-before.log` remains an
unqualified earlier fixture result. The runtime-depth regression separately
fails before its gate:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384BufferedSearchRuntimeDepthBoundary$' -count=1`;
log: `/tmp/gdx-04384-search-runtime-depth-before.log`.

The initial executable depth control used 126 arrays and failed the reference
because it assumed that item acceptance implied runtime acceptance. Its report
remains FAIL and is not a qualified control:
`/tmp/gdx-04384-search-envelope-depth-limit-before-oracle.json`.
Nine corrected strict baseline invocations on clean
`2a36859568bf5c54ae9409c42f20cb64099100af` use the output-overflow,
output-overflow-line, output-surrogate, output-depth-limit, output-depth-over,
output-early-surrogate, depth-kernel-reject, runtime-depth-over and
runtime-depth-over-rotation suffixes. All reference traces satisfy the complete
contract. The 124-array success and structural kernel-rejection controls PASS
both products; the other seven FAIL only Godex. Reports:
`/tmp/gdx-04384-search-envelope-qualified-<suffix>-before-oracle.json`.
The exact two-executable invocations are retained in
`/tmp/gdx-04384-search-envelope-qualified-baseline.py`.

The nine fixtures require exact successful/failed Responses or empty HTTP 500,
status/type, stdout, child exit, original authenticated request, selected key,
one attempt, zero retry and safe durable state. Existing independent mutation
tests include the new cases. Terminal wire checks are reused by actual HTTP 502
body rejection and HTTP 500 depth rejection, and enforce absent Content-Type
and the final stdout newline where required. Missing-child diagnostics now
retain the child exit and redacted stderr instead of hiding timeout evidence.

`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384Buffered' -count=1`
passes 114 tests: `/tmp/gdx-04384-search-envelope-all-focused.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,688 tests: `/tmp/gdx-04384-search-envelope-race.log`.
`rtk go test ./...` passes 9,313 tests across 81 packages:
`/tmp/gdx-04384-search-envelope-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-search-envelope-verify.log`.
Clean-commit executable verification follows. Global 1:1 parity remains unproven.


## Validated structured search envelope checkpoint: fca1a1a

Clean `fca1a1a4d229fbf8f295277f9af78f868956e687` passes all 115 DeepSeek
executable scenarios and the independent corruption controls:
`/tmp/gdx-04384-search-envelope-oracle.json`.
The new output-envelope failures retain exact serde prefixes and positions;
the 124-array public control succeeds, while the two 125-array runtime cases
return empty HTTP 500, exit 2, one authenticated attempt and no retry/rotation.
The focused overflow and two-key runtime-depth probes also PASS:
`/tmp/gdx-04384-search-envelope-overflow-oracle.json` and
`/tmp/gdx-04384-search-envelope-runtime-rotation-oracle.json`.

Both binaries were built from the clean commit with
`rtk go build -trimpath -o /tmp/gdx-04384-search-envelope-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-search-envelope-final-differential ./tools/differential`.
The full run uses `--scenario all --scenario-workers 3` and retains the exact
pinned reference source/binary digest and clean Godex VCS evidence in the report.
Latest local checks are 2,688 focused race tests, 9,313 full-suite tests and
`rtk proxy make verify`, all PASS. The prior 106-case checkpoint remains valid
on `2a36859`; no broader Gemini/default-script rerun is implied here.

This envelope stage is ready for direct fast-forward push after checking the
primary worktree and remote. Quoted-string/literal EOF and second-surrogate
escape positions are the next isolated follow-up. Other lexical diagnostics,
valid Gemini continuation/Compact ownership, native macOS/Windows and live
authentication remain open. Global 1:1 parity is not claimed.


## Validated Gemini control admission checkpoint: 293b1b6

Clean `293b1b6cb6d543875847b6aeec0a5e8750a1360b` passes all 89 Gemini cases
and all 125 DeepSeek scenarios, with independent mutation controls:
`/tmp/gdx-04384-gemini-control-types-oracle.json` and
`/tmp/gdx-04384-gemini-control-types-deepseek-oracle.json`.
Invalid stream/instructions/previous-response types remain terminal before
normalization or upstream dispatch. The stream-false control and all earlier
request/response, ownership and retry cases remain matched.

Clean builds use
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-control-types-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-control-types-final-differential ./tools/differential`.
Commands use the same pinned reference, `--gemini`, or
`--scenario all --scenario-workers 3`. After checking the primary worktree and
unchanged remote, this commit was fast-forwarded and pushed directly to main.
A full default `rtk proxy scripts/verify-release-parity.sh` invocation started
on an isolated clean 293b1b6 checkout at 2026-10-22 04:47:18 UTC:
`/tmp/gdx-04384-release-293b1b6.log`. Until its complete exit, including stages
after phase-all, the complete-default PASS remains scoped to clean `8730116`.


## Validated Gemini complete-body checkpoint: 3398cd9

Clean `3398cd9cb33aa5af28dccf7358c5882f8eb76dc2` passes all 102 Gemini cases
and all 125 DeepSeek scenarios, including independent matching corruptions:
`/tmp/gdx-04384-gemini-json-admission-oracle.json` and
`/tmp/gdx-04384-gemini-json-admission-deepseek-oracle.json`.
Three initial targeted clean-commit invocations also PASS:
`/tmp/gdx-04384-gemini-json-<root-null|root-null-rotation|syntax>-targeted-oracle.json`.
The exact empty 500 survives the HTTP delivery layer without a content type;
parse failures retain the terminal plain 502. Both stop before any upstream
attempt, including two-key rejection, and the valid Unicode control remains
an exact authenticated successful request with safe durable state.

Clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-json-admission-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-json-admission-final-differential ./tools/differential`.
Commands use the unchanged pinned reference, `--gemini`, or
`--scenario all --scenario-workers 3`. The checked clean primary worktree and
unchanged remote were fast-forwarded and pushed directly to `origin/main` at
`3398cd9`. No CI wait, force push, tag, release or editor-worktree change
occurred. The separate full default script on 293b1b6 remains in progress;
it is not evidence for later commits. Other controls, combined-error ordering,
Gemini Compact, session/turn executable continuation, native macOS/Windows
and live authentication remain open. Global 1:1 parity is not claimed.


## DeepSeek translation errors share one terminal presentation boundary

The complete pinned runtime propagates translation failures from
`runtime_deepseek_prepare_model_request` through `runtime_deepseek_response_attempts`
with `?`. `local_rewrite_pipeline_dispatch.rs:272–307,402–404` presents any
failed provider preparation result as plain HTTP 502 `upstream request failed`.
The policy's details are not a per-field HTTP-status distinction.

Godex's earlier targeted repairs used a private error marker for known fields;
unmarked rejected controls still produced ordinary 400 errors and entered
recovery. The request-attempt boundary now maps every Responses translation
error to that exact terminal 502, preserving the distinct empty-500 valid-root
case. The now-unnecessary marker, its wrappers and a parser forwarding function
are removed. Detailed internal validation errors remain available through
`ResponsesRequest`; protocol validation is not removed or weakened. Unit
regressions assert rejection/diagnostics, and the actual transport table retains
the terminal status/message and zero-upstream assertions for old and new fields.

This fixes the common cause once in `gateway/deepseek`, reduces production code
and keeps retry policy in `usecase/routing`. Native Messages conversion failures,
provider responses and passthrough behavior retain their independent paths.
No new production helper, package, exported API, interface, dependency, config,
worker or persisted format is added. This supersedes the earlier field-specific
marker implementation recorded in historical checkpoints without changing its
already matched public outcomes.

Before repair 42 controls pass and thirteen tests fail, including the parent:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04380DeepSeekInvalidInputIsTerminalBeforeUpstream$' -count=1`;
`/tmp/gdx-04384-deepseek-preparation-errors-before.log`.
The original twelve new cases pass after repair, 55 tests including controls:
`/tmp/gdx-04384-deepseek-preparation-errors-focused.log`.
The first complete gateway run has 824 passes and two failures:
`rtk go test ./internal/gateway/deepseek -count=1`;
`/tmp/gdx-04384-deepseek-preparation-errors-gateway-tests.log`.
Those older web-search transport tests required 400 or detailed internal text.
They now require the tagged generic terminal 502 while retaining zero-attempt
guards. Malformed search context is also added to the executable fixture set
and transport table. The complete gateway rerun passes all 827 tests:
`/tmp/gdx-04384-deepseek-preparation-errors-final-gateway-tests.log`.

Thirteen executable baseline reports pin unchanged clean
`587f04a08cd252fd44c8d231a344b4b06498d265` and the unchanged reference:
`/tmp/gdx-04384-deepseek-preparation-<suffix>-before-oracle.json`.
All FAIL only Godex; every reference and independent mutation invariant passes.
The original twelve use `/tmp/gdx-04384-deepseek-preparation-errors-differential`;
the additional search case uses
`/tmp/gdx-04384-deepseek-preparation-errors-web-search-differential`.
The existing strict primitive runner checks exact plain response/status/type,
exit/stdout, zero attempts/retries and safe durable state. Prior accepted
controls remain in the same complete executable matrix.

`rtk go test ./tools/differential -count=1` passes 281 tests before the final
web-search fixture addition:
`/tmp/gdx-04384-deepseek-preparation-errors-oracle-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,938 tests: `/tmp/gdx-04384-deepseek-preparation-errors-race.log`.
`rtk proxy go test -json ./...` passes 9,747 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-deepseek-preparation-errors-test-all.jsonl`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-deepseek-preparation-errors-verify.log`.
Clean-commit 213-case DeepSeek qualification follows. The separate 185-case
Gemini PASS remains scoped to clean `8d618e0`; Gemini production is unchanged.
The completed default gate remains `293b1b6` while the full protected
`8d618e0` invocation runs. Global 1:1 parity is not claimed.


## Complete default release gate checkpoint: 8d618e0

The unchanged default command completes every stage and exits 0 from protected
clean `8d618e097c8549015785bce1e23c89999198d42f`:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-8d618e0.log 2>&1`.
The final HTTP profile-status and raw-429 checks after the phase marker also
PASS. The protected worktree remains clean at the same exact revision.
This advances the completed default-gate checkpoint from `293b1b6` to
`8d618e0`, not to subsequent DeepSeek or Gemini changes. Global parity is unproven.


## Validated DeepSeek JSON and identity checkpoints

Clean `09790dca9375f389578f5f921998c77a515dc6e5` passes all 190 DeepSeek
scenarios with independent mutation controls:
`/tmp/gdx-04384-deepseek-json-admission-oracle.json`.
The exact null-root, malformed-syntax and valid-Unicode targeted cases PASS:
`/tmp/gdx-04384-deepseek-json-<root-null|syntax|valid-unicode>-targeted-oracle.json`.
The clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-json-admission-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-json-admission-final-differential ./tools/differential`.

Clean `587f04a08cd252fd44c8d231a344b4b06498d265` passes all 200 DeepSeek
scenarios with independent mutations:
`/tmp/gdx-04384-deepseek-user-id-oracle.json`.
Canonical and null-identity targeted reports PASS:
`/tmp/gdx-04384-deepseek-user-id-<canonical|null>-targeted-oracle.json`.
The binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-user-id-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-user-id-final-differential ./tools/differential`.
Both full matrices use `--scenario all --scenario-workers 3`, the unchanged
pinned reference and matching clean candidate source/commit pins.
Each completed candidate was checked, fast-forwarded through the clean primary
worktree and pushed directly to `origin/main` without waiting for CI.
No force push, tag, release or editor-checkout change occurred.
The newer `18f7ad3` 213-case matrix remains pending:
`/tmp/gdx-04384-deepseek-preparation-errors-oracle.json`.
Its targeted frequency-penalty and malformed-web-search reports PASS:
`/tmp/gdx-04384-deepseek-preparation-<frequency|web-search>-targeted-oracle.json`.


## Gemini preparation errors preserve the tool-declaration exception

The pinned `local_rewrite_gemini_send.rs:147–158` first runs
`runtime_gemini_request_validation_error`, returning structured 400
`invalid_request_error` for malformed tool declarations. Later request
translation errors propagate to the plain terminal 502 pipeline boundary.
These are distinct public outcomes. Godex's tool errors already matched;
invalid reasoning or input instead entered recovery through an ordinary 400.

The API-key gateway now uses the existing generic terminal 502 for its request
translation/normalization failures, preserving the indexed tool validator's
400 presentation and the valid nonobject empty-500 path. The native adapter,
credential routing and provider response paths are unchanged. Four existing
error returns change; no production helper, package, exported API, interface,
dependency, config, worker or persisted format is added. This stage does not
claim correct precedence for every combination of invalid fields.

The first regression/oracle draft incorrectly expected plain 502 for all seven
cases. Its eight failing unit tests and seven FAIL executable reports remain
unqualified evidence:
`/tmp/gdx-04384-gemini-preparation-errors-before.log` and
`/tmp/gdx-04384-gemini-preparation-<suffix>-before-oracle.json`.
Five reference tool-error invariants correctly reject that expectation. The
raw executable reports show that both products already return the same exact
structured 400, indexed text, JSON type, no-store/nosniff, exit and no attempt.
The overbroad draft change and its altered legacy test were undone before any
commit. The original detailed tool regression remains unchanged.

The corrected regression is run against isolated `18f7ad3` source with only
the new test added: five controls PASS and three tests FAIL, including parent:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiRequestPreparationFailuresAreTerminal$' -count=1`;
`/tmp/gdx-04384-gemini-preparation-errors-qualified-before.log`.
The corrected new regression and existing valid/invalid tool tests pass 16 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex(04384GeminiRequestPreparationFailuresAreTerminal|04380GeminiAPIKeyChat(RejectsInvalidFunctionToolsBeforeUpstream|KeepsValidFunctionTools))$' -count=1`;
`/tmp/gdx-04384-gemini-preparation-errors-qualified-focused.log`.

The first corrected oracle self-check caught a preexisting fixed mutation that
set Content-Type to `application/json`, which is the valid value for the newly
added 400 fixtures. Three oracle tests FAILed:
`/tmp/gdx-04384-gemini-preparation-errors-qualified-oracle-tests.log`.
The corresponding qualified executable attempts stop on the negative-control
guard and produce no valid reports:
`/tmp/gdx-04384-gemini-preparation-<suffix>-qualified-before-oracle.json`.
They are not parity evidence. The mutation now uses a distinct corrupt media
type, preserving every assertion and strengthening the separate no-store/
nosniff mutations. `rtk go test ./tools/differential -count=1` then passes all
281 tests: `/tmp/gdx-04384-gemini-preparation-errors-final-oracle-tests.log`.

Seven final qualified baselines pin clean
`18f7ad39733e8a5a033a5766d6d14ecd934401bd`, the unchanged reference and
`/tmp/gdx-04384-gemini-preparation-errors-final-qualified-differential`:
`/tmp/gdx-04384-gemini-preparation-<suffix>-final-qualified-before-oracle.json`.
All five tool-declaration controls PASS; the two input/reasoning cases FAIL
only Godex. Every reference and independent mutation invariant passes.
The existing strict primitive fixture family retains exact error bodies/types,
exit/stdout, attempts, authenticated valid controls and safe durable state.

Earlier draft/invalid-control checks are not final qualification.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,968 tests: `/tmp/gdx-04384-gemini-preparation-errors-final-race.log`.
`rtk proxy go test -json ./...` passes 9,755 tests across 81 packages
(71 passing and ten with no tests):
`/tmp/gdx-04384-gemini-preparation-errors-final-test-all.jsonl`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-preparation-errors-final-verify.log`.
Clean-commit 192-case Gemini qualification follows, with 213 DeepSeek scenarios
separately qualified on unchanged DeepSeek production in clean `18f7ad3`.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Validated common DeepSeek preparation boundary: 18f7ad3

Clean `18f7ad39733e8a5a033a5766d6d14ecd934401bd` passes all 213 DeepSeek
scenarios and independent mutations:
`/tmp/gdx-04384-deepseek-preparation-errors-oracle.json`.
Frequency-penalty and malformed-search targeted reports also PASS:
`/tmp/gdx-04384-deepseek-preparation-<frequency|web-search>-targeted-oracle.json`.
The exact binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-preparation-errors-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-preparation-errors-final-differential ./tools/differential`.
The full matrix uses `--scenario all --scenario-workers 3`, the unchanged
pinned reference and matching clean candidate/source pins. The clean primary
worktree and remote were checked and fast-forwarded; `18f7ad3` was pushed
directly to `origin/main` without CI wait, force push, tag, release or
editor-checkout changes.

The clean next candidate `a7fabe987e50e30330599c534367323254db8f00` passes
targeted invalid-input, invalid-reasoning and malformed-tool cases:
`/tmp/gdx-04384-gemini-preparation-<input|reasoning|tool-params>-targeted-oracle.json`.
Its full 192-case Gemini and 213-case DeepSeek matrices are pending:
`/tmp/gdx-04384-gemini-preparation-errors-oracle.json` and
`/tmp/gdx-04384-gemini-preparation-errors-deepseek-oracle.json`.
The completed default gate remains scoped to `8d618e0`, not later candidates.


## Gemini validates malformed tools before other request-shape errors

The pinned runtime's tool/candidate-count gate runs before model selection and
chat-compatible shape validation. Complete serde parsing precedes that gate.
Thus malformed tools combined with invalid stream, temperature, tokens, user
identity, input or reasoning retain the indexed structured 400. A malformed
Unicode value anywhere in the body instead prevents the gate from parsing and
retains the later plain terminal 502 outcome.

Godex ran its existing tool validator only after control normalization and
shared Chat conversion. Six combined-error cases returned the later 502
instead of the earlier tool 400. The API-key adapter now invokes that same
private validator after complete JSON/root admission but before normalization,
removing the later duplicate location. Flat-tool projection and indexed
diagnostics are unchanged. Valid tool schemas, request numbers, alias handling,
empty-500 roots and malformed-JSON admission retain their existing checks.
All production logic remains in `gateway/gemini`; no helper, package, exported
API, interface, dependency, config, worker or persisted format is added.
Candidate-count precedence and other combined errors remain separate audits.

Before repair the malformed-serde control PASSes and seven tests fail,
including the parent:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiToolValidationPrecedesOtherShapeErrors$' -count=1`;
`/tmp/gdx-04384-gemini-tool-priority-before.log`.
The same command passes all eight tests after repair:
`/tmp/gdx-04384-gemini-tool-priority-focused.log`.

Seven executable baselines pin clean `a7fabe9`, the unchanged reference and
`/tmp/gdx-04384-gemini-tool-priority-differential`:
`/tmp/gdx-04384-gemini-tool-priority-<suffix>-before-oracle.json`.
Six combined shape errors FAIL only Godex; the invalid-surrogate control PASSes.
Every reference and independent mutation invariant passes. The existing strict
fixture runner retains the exact 400/502 body/status/type, terminal headers,
exit/stdout, zero attempts and safe durable state. A focused private tool-priority
fixture file owns the raw combinations without another production abstraction.

`rtk go test ./tools/differential -count=1` passes 281 tests before repair:
`/tmp/gdx-04384-gemini-tool-priority-oracle-tests.log`.
`rtk go test ./internal/gateway/gemini -count=1` passes 865 tests:
`/tmp/gdx-04384-gemini-tool-priority-gateway-tests.log`.
`rtk proxy go test -json ./...` passes 9,763 tests across 81 packages
(71 passing and ten with no tests):
`/tmp/gdx-04384-gemini-tool-priority-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,976 tests: `/tmp/gdx-04384-gemini-tool-priority-race.log`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-tool-priority-verify.log`.
Clean-commit 199-case Gemini qualification follows, with 213-case DeepSeek
coverage separately pinned to its unchanged production implementation.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Validated model-input and DeepSeek control-type checkpoints

Clean `8d618e097c8549015785bce1e23c89999198d42f` passes all 185 Gemini cases
and 169 DeepSeek scenarios with independent mutation controls:
`/tmp/gdx-04384-gemini-model-input-oracle.json` and
`/tmp/gdx-04384-gemini-model-input-deepseek-oracle.json`.
The clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-model-input-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-model-input-final-differential ./tools/differential`.
The three targeted model reports recorded above PASS on that exact candidate.

Clean `d0b4f56999e0de585f4ffd9fe325b6bc88878986` passes all 178 DeepSeek
scenarios with independent mutations:
`/tmp/gdx-04384-deepseek-control-types-oracle.json`.
The binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-control-types-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-deepseek-control-types-final-differential ./tools/differential`.
Null-stream and previous-array targeted reports PASS:
`/tmp/gdx-04384-deepseek-control-stream-null-targeted-oracle.json` and
`/tmp/gdx-04384-deepseek-control-previous-array-targeted-oracle.json`.
Commands use `--gemini`, or `--scenario all --scenario-workers 3`, with the
matching clean candidate/source and unchanged pinned reference.
The clean primary worktree and remote were checked and fast-forwarded;
`8d618e0` and `d0b4f56` were pushed directly to `origin/main` without CI wait.
No force push, tag, release or editor-checkout change occurred.
The full default gate on protected clean `8d618e0` remains pending;
the last completed full default gate is still `293b1b6`.


## DeepSeek complete-JSON admission and valid nonobject outcomes

Released-executable probes distinguish complete parse/serde failures from
valid nonobject JSON. Null, array, number, boolean and string roots return
an empty HTTP 500 with no Content-Type; malformed/trailing JSON, invalid
Unicode/UTF-8, numeric overflow and excess depth return exact plain HTTP 502
`upstream request failed`. Both have exit 2 and zero upstream attempts.
The valid-Unicode control produces one authenticated request and the exact
successful DeepSeek Responses output.

Godex already rejected invalid JSON in its complete decoder, but classified
that rejection as an ordinary error that entered routing recovery. Valid
nonobject roots likewise entered recovery, leaving no child exchange in
executable observations. The gateway now classifies parse failures through
its existing terminal preparation error and preserves valid nonobject roots
as a distinct private error until runtime presentation. The latter becomes
the exact empty 500 with existing `invalid_request_error` precommit evidence,
preventing both model and credential retries. It does not reproduce a panic
or weaken ordinary provider-500 policy.

The complete decoder and its two admission classes now live together in
`gateway/deepseek/request_json_admission_04384.go`, reducing the broad request
file. Existing history parsing shares that decoder, so rejected bytes cannot
contribute conversation context. All code remains in the concrete integration
gateway; no new production package, shared helper, exported API, interface,
dependency, configuration, worker or persisted format is introduced.

Three preliminary executable reports record the observed outcomes, all
failing only Godex:
`/tmp/gdx-04384-deepseek-json-<root-null|root-array|syntax>-unqualified-before-oracle.json`.
The twelve qualified baseline reports pin clean `d0b4f56` and the unchanged
reference, using `/tmp/gdx-04384-deepseek-json-admission-unqualified-differential`:
`/tmp/gdx-04384-deepseek-json-<suffix>-qualified-before-oracle.json`.
One Unicode control PASSes; all eleven rejection cases FAIL only Godex.
Every reference and independent mutation invariant passes. The original raw
fixture bytes remain intact, including malformed UTF-8; accepted control wire,
response/status/type/stdout/exit, auth/slot, attempts and durable state remain
strict. An explicitly present empty Content-Type is rejected for empty-500
outcomes rather than treated as an absent header.

Before repair one regression control passes and sixteen tests fail, including
parents and the two-key no-retry/no-wait checks:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384DeepSeekJSONAdmission(ReturnsTerminalOutcome|DoesNotRetryModelOrCredential)$' -count=1 -timeout=30s`;
`/tmp/gdx-04384-deepseek-json-admission-before.log`.
The same command passes 17 tests after repair:
`/tmp/gdx-04384-deepseek-json-admission-focused.log`.
`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-deepseek-json-admission-unqualified-oracle-tests.log`.

The first full gateway run correctly exposes an older 400 expectation:
`rtk go test ./internal/gateway/deepseek -count=1`;
`/tmp/gdx-04384-deepseek-json-admission-gateway-tests.log` has 800 passing tests
and one failing `TestProdex04373DeepSeekLoopbackToolRequestAndMalformedGuard`.
The concurrent first full suite likewise records 9,720 passes and one failure
in `/tmp/gdx-04384-deepseek-json-admission-test-all.jsonl`.
The first race run also FAILs that old expectation:
`/tmp/gdx-04384-deepseek-json-admission-race.log`.
That regression now requires the independently observed plain terminal 502
message while preserving its upstream-attempt guard.
`rtk go test ./internal/gateway/deepseek -count=1` passes all 801 tests:
`/tmp/gdx-04384-deepseek-json-admission-final-gateway-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,912 tests: `/tmp/gdx-04384-deepseek-json-admission-final-race.log`.
`rtk proxy go test -json ./...` passes 9,721 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-deepseek-json-admission-final-test-all.jsonl`.
The first `rtk proxy make verify` invocation completes successfully after the
corrected expectation is on disk:
`/tmp/gdx-04384-deepseek-json-admission-verify.log`.
The final unchanged-source `rtk proxy make verify` rerun also PASSes:
`/tmp/gdx-04384-deepseek-json-admission-final-verify.log`.
Clean-commit 190-case DeepSeek qualification follows. The separate 185-case
Gemini PASS remains scoped to clean `8d618e0`; Gemini production is unchanged.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## DeepSeek invalid user identities are terminal preparation failures

The pinned shared optional-field path calls
`deepseek_provider_core_user_id_from_responses_request`: first-present
`user_id`, `user`, then `safety_identifier`, requiring a string and compiled
Mojo normalization. Empty selected values omit the field; invalid types or
nonempty values outside the 512-byte ASCII letters/digits/underscore/dash
contract fail preparation. The released runtime presents plain terminal 502
before any provider dispatch.

Godex already normalized and validated identities correctly, but returned an
ordinary error that could enter routing recovery. `gateway/deepseek` now wraps
that error in its existing terminal preparation class at `copyUserField`.
The identity rules and detailed internal error remain unchanged. No new
production helper, file, package, exported API, interface, dependency, config,
worker or persisted format is added. This one-line boundary correction leaves
account routing and retry decisions in the use case.

Before repair 42 controls pass and ten tests fail, including parents:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex(04384DeepSeekUserIDUsesAliasesAndTerminalValidation|04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-deepseek-user-id-before.log`.
The same command passes 52 tests after repair:
`/tmp/gdx-04384-deepseek-user-id-focused.log`.
The transport table proves invalid identity input produces no upstream attempt.
Positive controls require exact canonical request bytes, first-alias priority,
blank suppression and Unicode trimming at the 512-byte limit.

Ten executable baseline reports pin unchanged clean
`09790dca9375f389578f5f921998c77a515dc6e5`, the unchanged reference and
`/tmp/gdx-04384-deepseek-user-id-differential`:
`/tmp/gdx-04384-deepseek-user-id-<suffix>-before-oracle.json`.
Six valid controls PASS; four invalid identity cases FAIL only Godex.
Every reference and independent mutation invariant passes. The existing strict
primitive runner reuses the prior Gemini identity fixture data, with exact
DeepSeek output/auth/slot/request bytes and safe state; it adds no production
cross-gateway dependency and no duplicate Gemini executable cases.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-deepseek-user-id-oracle-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,925 tests: `/tmp/gdx-04384-deepseek-user-id-race.log`.
`rtk proxy go test -json ./...` passes 9,734 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-deepseek-user-id-test-all.jsonl`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-deepseek-user-id-verify.log`.
Clean-commit 200-case DeepSeek qualification follows. The separate 185-case
Gemini PASS remains scoped to clean `8d618e0`; Gemini production is unchanged.
The completed default release gate remains `293b1b6` while the protected
full `8d618e0` invocation is pending. Other controls, combined-error ordering,
Gemini Compact, executable session/turn continuation, native macOS/Windows
and live authentication remain open. Global 1:1 parity is not claimed.


## Primitive request oracle: token aliases, uint64 and ignored parallel input

The pinned Gemini and DeepSeek Chat bridges share the primitive-field kernel.
`mojo/prodex_core/deepseek.mojo:1177–1213` scans maximum-token fields in output,
maximum, completion order, with each later present value replacing the prior
selection. Its policy at 2153–2187 validates every present alias first. Positive
integer digits are checked at 1722–1734 after serde Value parsing, so uint64
maximum remains valid but a decimal/exponent value or uint64 overflow does not.
The kernel omits `parallel_tool_calls`; DeepSeek separately rejects false,
but tolerates and ignores true. Gemini's ignored-control policy was already
qualified in the preceding stage.

New developer-only fixtures use the same technical input data in the existing
Gemini and DeepSeek harnesses. Cases pin canonical raw request bytes, original
credential slot, authenticated attempt count, exact response/type/exit/stdout,
zero retries and safe durable state. Invalid aliases must return terminal
plain 502 with no upstream attempt. Accepted cases require one original
request, including exact uint64 bytes. Missing exchanges are retained explicitly
and fail. Matching wire, auth, slot, response, retry, stdout and state corruptions
are rejected independently; no state allowlist or assertion is weakened.
The exact Gemini request/result matcher now has two real fixture consumers
within this developer package; no production helper or provider abstraction
was extracted.

Eighteen baseline runs use clean
`3398cd9cb33aa5af28dccf7358c5882f8eb76dc2`, the unchanged pinned reference and
`/tmp/gdx-04384-request-token-limits-differential`.
Six controls PASS: output/all for both products, plus Gemini u64/parallel-true.
Five Gemini and seven DeepSeek cases FAIL only Godex, while all reference and
independent invariants pass:
`/tmp/gdx-04384-request-primitive-<gemini|deepseek>-<suffix>-before-oracle.json`.
The suffixes are output, precedence, all, shadow-null, shadow-zero,
shadow-fraction, u64, overflow and parallel-true. Gemini runs use
`--gemini --scenario request-primitive-<suffix>`; DeepSeek uses
`--scenario deepseek-request-primitive-<suffix>`. The candidate/source are the
clean JSON-admission binary/worktree, with the corresponding pinned commit.
Both adapters choose the wrong output/maximum precedence and mask invalid
lower-priority aliases. DeepSeek additionally rejects uint64 maximum and leaks
the tolerated parallel control. Its overflow case times out after a local 400;
the reference returns the exact terminal 502.

`rtk go test ./tools/differential -count=1` passes 276 oracle tests before
production repair: `/tmp/gdx-04384-request-token-limits-final-oracle-tests.log`.
This commit establishes the checked fail-before contract. The gateway repairs
and their focused regression/local completion evidence are the next stage;
these fixtures alone do not claim repaired executable parity.


## Primitive token-limit repair in the concrete gateways

The Gemini gateway now validates all three maximum-token aliases as positive
serde integer values through uint64 maximum and reduces them to canonical
`max_tokens` before the shared Chat converter runs. Its private normalizer
stays in `gateway/gemini`. DeepSeek's owning `copyMaxTokenField` validates every
present alias in tagged order, retains unsigned 64-bit values and gives the
completion alias highest priority. Invalid token input uses the existing
`invalidResponsesInputError`, so transport presents the exact terminal 502
instead of an ordinary 400 that can enter routing recovery. The unrelated
web-search integer helper keeps its existing separate contract.

DeepSeek's obsolete parallel-field copier is removed: validated true remains
tolerated but never reaches upstream. The advanced-control test now asserts
absence instead of preserving its old incorrect expectation; all other
advanced-control assertions remain. There is no alternate implementation,
cross-gateway import, new production shared helper/package/interface, exported
identifier, dependency, configuration, persistent format or worker. Each
integration owns its protocol normalization and terminal error presentation.

Before repair,
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini -run '^TestProdex04384(DeepSeekTokenLimitPrecedenceAndAdmission|GeminiTokenLimitPrecedenceAndAdmission|DeepSeekParallelTrueIsIgnored)$' -count=1`
passes 13 controls and fails 16 tests including parents:
`/tmp/gdx-04384-request-token-limits-before.log`.
After only the value/precedence repair, the expanded terminal-error test still
fails five invalid token cases plus parent while its eight older controls pass:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04380DeepSeekInvalidInputIsTerminalBeforeUpstream$' -count=1`;
`/tmp/gdx-04384-request-token-limits-deepseek-terminal-before.log`.
This catches the entire transport error path rather than merely a parser result.
After error mapping, the focused command passes 44 tests across both gateways:
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini -run '^Test(Prodex04384(DeepSeekTokenLimitPrecedenceAndAdmission|GeminiTokenLimitPrecedenceAndAdmission|DeepSeekParallelTrueIsIgnored)|DeepSeekResponsesRequestMapsAdvancedControls|Prodex04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-request-token-limits-reviewed-focused.log`.

`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,562 tests: `/tmp/gdx-04384-request-token-limits-race.log`.
`rtk proxy go test -json ./...` passes 9,522 tests across 81 packages:
`/tmp/gdx-04384-request-token-limits-test-all.jsonl`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-request-token-limits-verify.log`.
The preceding oracle-only commit and this gateway repair are separate useful,
compiling stages. Clean-commit 111-case Gemini and 134-case DeepSeek executable
matrices follow before either is pushed. Other primitive fields, combined-error
ordering, Gemini Compact, session/turn executable continuation, native macOS/
Windows and live authentication remain open. Global 1:1 parity is not claimed.


## Validated string EOF checkpoint: c1f0e5a

Clean `c1f0e5a45b7705d21d95e6181663da3a9ffa5895` passes all 125 DeepSeek
executable scenarios and independent status/header/event/state/retry/
cancellation/stdout mutations: `/tmp/gdx-04384-string-eof-oracle.json`.
The nine corrected EOF/surrogate cases and the earlier-range control satisfy
the exact reference contract. All prior body, kernel admission, output-envelope
and terminal no-rotation cases remain in this full run.

The binaries were built from that clean commit with
`rtk go build -trimpath -o /tmp/gdx-04384-string-eof-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-string-eof-final-differential ./tools/differential`.
The full run uses `--scenario all --scenario-workers 3`, the same pinned 0.438.4
reference and verified clean Godex VCS evidence retained in the report.
Latest local checks are 937 focused race tests, 9,366 full-suite tests and
`rtk proxy make verify`, all PASS. This checkpoint is ready for direct
fast-forward push after checking the primary worktree and remote.

The latest Gemini 51-case PASS remains on `625fac5`, and the complete default
release-script PASS remains on `e9ccedc`; neither was rerun here. The next
source audit follows the shared 4 MiB buffered runtime gate into Gemini's
API-key chat-compatible path. Valid Gemini continuation/Compact ownership,
other lexical diagnostics, native macOS/Windows and live authentication remain
open. Global 1:1 parity is not claimed.


## Quoted-string EOF preserves earlier errors and consumed escape bytes

The structured envelope stage was fast-forwarded and pushed directly to
`origin/main` at `50b5f47787bb25a8d5a2be134c3e5955779e8c7c` after checking
the clean primary worktree and unchanged remote. No CI wait, force push, tag,
release or editor-worktree modification occurred.

Pinned serde reports `EOF while parsing a string` for truncated quoted values
or keys, dangling escapes and incomplete Unicode escapes. It reports value EOF
for a truncated literal in a rejected container. Go's token decoder returns
`io.ErrUnexpectedEOF` for both classes. The private DeepSeek error walk now
distinguishes the incomplete token and counts all consumed UTF-8 bytes.
The walk remains behind the existing kernel/output error gates; valid ordinary
opaque fragments are not reinterpreted or rejected.

A string may contain an invalid surrogate before it ends. The existing private
surrogate check now accepts a token ending at EOF and retains that earlier
failure. It also counts the second consumed escape byte when a high surrogate
is followed by an escape other than `u`: the structured item's locked serde
diagnostic is column 99, not 98, for the synthetic escaped-quote fixture.
Completed valid pairs and ordinary opaque tool strings keep their behavior.
This is one focused gateway file change, with no new parser on valid ordinary
inputs, shared helper, exported API, interface, dependency, state format,
configuration or background work.

The locked Rust and independent Go token probes are retained at:
`/tmp/gdx-04384-string-eof-audit-probe.jsonl`,
`/tmp/gdx-04384-string-eof-extended-serde-probe.jsonl`,
`/tmp/gdx-04384-string-eof-priority-serde-probe.jsonl` and
`/tmp/gdx-04384-string-eof-go-token-probe.jsonl`.
They establish diagnostics and byte offsets, not released-executable parity
by themselves.

Before repair,
`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384Buffered(StringEOF|SearchSurrogateEscape)' -count=1`
passes six earlier-error controls and fails 47 tests including parents:
`/tmp/gdx-04384-string-eof-before.log`.
The regressions exercise ordinary, structured search and apply-patch tools,
partial keys/values, UTF-8/newline columns, dangling and Unicode escapes,
earlier numeric/surrogate errors and the outer item position.

Ten strict baseline invocations on clean
`fca1a1a4d229fbf8f295277f9af78f868956e687` use eof-string, eof-key,
eof-string-unicode, eof-string-escape, search-eof-string,
eof-string-surrogate, eof-string-surrogate-pair, eof-literal-value,
eof-string-early-range and search-output-surrogate-quote suffixes.
All reference traces satisfy exact failed Responses, status/type, stdout,
child exit, original authenticated request, one attempt, no retry and safe
durable-state invariants. The early-range control PASSes both products; the
other nine FAIL only Godex. Reports:
`/tmp/gdx-04384-string-eof-<suffix>-before-oracle.json`.
Exact invocations are retained in `/tmp/gdx-04384-string-eof-baseline.py`.
The existing independent mutation tests cover all ten new fixtures.

`rtk go test ./internal/gateway/deepseek -run '^TestProdex04384Buffered' -count=1`
passes 167 tests: `/tmp/gdx-04384-string-eof-focused.log`.
`rtk go test -race ./internal/gateway/deepseek ./tools/differential -count=1`
passes 937 tests: `/tmp/gdx-04384-string-eof-race.log`.
`rtk go test ./...` passes 9,366 tests across 81 packages:
`/tmp/gdx-04384-string-eof-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-string-eof-verify.log`.
Clean-commit executable verification follows. Other lexical classes, valid
Gemini continuation/Compact ownership, native macOS/Windows and live
authentication remain open. Global 1:1 parity is not claimed.


## Gemini API-key buffered body admission and terminal retry evidence

The string EOF checkpoint was fast-forwarded and pushed directly to
`origin/main` at `f4aca3f4b69ca70c1fa8f573a11fbc46fa410b7a` after checking
the clean primary worktree and unchanged remote. No CI wait, force push, tag,
release or editor-worktree modification occurred.

The pinned API-key Gemini chat-compatible response path calls
`runtime_deepseek_chat_buffered_response_parts` through
`local_rewrite_response_chat_compatible.rs`. It therefore shares the same
4 MiB complete-body gate before provider normalization. Godex's Gemini bridge
instead read 8 MiB and accepted oversized success. Its translation errors also
escaped as ordinary gateway errors, allowing inappropriate credential retry.

`gateway/gemini/compatibility_043711.go` now has a separate 4 MiB successful
buffered chat-body cap and returns the exact local plain HTTP 502 processing
failure with the existing non-retryable precommit code. `usecase/routing` now
respects that code's classification before retrying a Gemini model. Without
that use-case change, even a correctly marked local 502 was retried on the
next model before credential routing could recognize its terminal evidence.
The gateway owns input admission/failure evidence; the use case owns retry
policy and consumes existing models/entities. No dependency direction changes,
new interface, shared production helper, configuration, state format, worker
or external dependency were introduced. Native direct-provider transports and
Compact remain separate audit surfaces.

A two-credential HTTP pool/router regression cancels unexpected extra attempts
without sleeps. Malformed and oversized successes must terminate after one
attempt; an actual upstream 502 remains retryable. Before repair, the focused
command passes three controls and fails five tests including parents:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiBuffered(BodyBoundary|FailureDoesNotRetry)' -count=1`;
log: `/tmp/gdx-04384-gemini-body-before.log`.
With only the gateway repair, five tests pass and three still fail due to model
fallback: `/tmp/gdx-04384-gemini-body-gateway-only.log`.
After the routing repair all eight pass:
`/tmp/gdx-04384-gemini-body-focused.log`.

Four new Gemini executable fixtures pad complete native bodies to 4 MiB minus
one, exactly 4 MiB and 4 MiB plus one. The same technical padding function now
has two real fixture consumers within `tools/differential`; it has not moved
into production or a cross-domain helper. Accepted responses match every byte
except an independently validated UUIDv7 value. Oversized responses require
exact 502 text/type, stdout and exit 2. All cases require the original exact
authenticated request, correct key slot, one attempt, no model/key replay and
safe durable state. The rotation case configures two synthetic keys.
Independent matching corruptions of status, type, body, exit, stdout, state,
attempts, auth, slot and request must fail.

Initial executable fixtures used `gemini-child-exchange.json`, which the common
state auditor correctly rejected as unknown on both products. These reports
remain unqualified FAIL results:
`/tmp/gdx-04384-gemini-body-<under|limit|over|over-rotation>-before-oracle.json`.
The fixture now uses the existing standard private `child-exchange.json` path;
no allowlist or state assertion was weakened. Corrected strict baseline runs on
clean `c1f0e5a45b7705d21d95e6181663da3a9ffa5895` PASS under/limit and FAIL
only Godex for over/over-rotation. Every reference run meets the entire contract,
including one primary-key attempt in the two-key case. Reports:
`/tmp/gdx-04384-gemini-body-qualified-<suffix>-before-oracle.json`.
Exact invocations are retained in `/tmp/gdx-04384-gemini-body-baseline.py`.

`rtk go test ./tools/differential -count=1` passes 257 tests before the production
repair: `/tmp/gdx-04384-gemini-body-oracle-tests.log`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,734 tests: `/tmp/gdx-04384-gemini-body-race.log`.
`rtk go test ./...` passes 9,375 tests across 81 packages:
`/tmp/gdx-04384-gemini-body-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-gemini-body-verify.log`.
Clean-commit executable verification follows. Global 1:1 parity remains unproven.


## Validated Gemini buffered admission checkpoint: 8730116

Clean `8730116c9ecafb6e6d9db06f5a76055ed868913f` passes all 55 Gemini cases
and all 125 DeepSeek scenarios. Both independent negative-control sets PASS.
Reports: `/tmp/gdx-04384-gemini-body-oracle.json` and
`/tmp/gdx-04384-gemini-body-deepseek-oracle.json`.
All four Gemini body boundaries satisfy exact output/type/exit/stdout,
authenticated original request, selected key, single attempt and durable-state
invariants. Both over-limit runs return the exact local 502, including one
primary attempt with two configured synthetic keys and no model/key replay.

Both binaries were built from that clean commit with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-body-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-body-final-differential ./tools/differential`.
The Gemini command uses `--gemini`; DeepSeek uses
`--scenario all --scenario-workers 3`. Entry gates verify the same pinned
0.438.4 source/binary digest and clean Godex VCS provenance. Latest local checks
are 2,734 focused race tests, 9,375 full-suite tests and
`rtk proxy make verify`, all PASS.

The stage and documentation checkpoint were fast-forwarded and pushed directly
to `origin/main` at `7e93289e6ac6d6b77fa94b0b15de81d02c201ad6`.
The complete default `rtk proxy scripts/verify-release-parity.sh` invocation
on isolated clean `8730116` finished with exit 0, including the HTTP profile
status and raw HTTP 429 stages after its phase-all marker:
`/tmp/gdx-04384-release-8730116.log`. The complete-default milestone therefore
advances from historical `e9ccedc` to `8730116`; it does not cover subsequent
continuation changes. Other lexical classes, Gemini restart/Compact ownership,
native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Gemini raw-key bound rejection and unbound fresh-input fallback

The pinned `local_rewrite_pipeline_dispatch.rs:328–374` projects the selected
dispatch identity through `from_profile`, while
`local_rewrite_gemini_openai.rs` records accepted raw-key work through
`from_raw_key`. The exact identity comparison in
`mojo/prodex_core/runtime_lineage.mojo:656–682` rejects a known response binding
before another upstream attempt. A successful seed therefore does not imply
successful replay. `chat_compatible_request.rs:44–137` rebuilds an unbound
request from the current input when no history is found.

`usecase/routing/raw_key_continuation.go` reuses the existing DeepSeek
dispatch-binding rejection for non-local Gemini raw-key routes. Known response,
session and turn owners return the existing terminal HTTP 503 before gateway
dispatch. `gateway/gemini/compatibility_043711.go` removes an unbound
`previous_response_id` only after complete serde/JSON validation so the
stateless request uses the current input. Models/Compact and managed profiles
retain their separate policies. The use case owns routing policy; the gateway
owns provider request normalization. No production helper, package, interface,
dependency, configuration, goroutine or persistence format was added.

Qualified baseline executable runs on clean
`8730116c9ecafb6e6d9db06f5a76055ed868913f` FAIL only Godex:
`/tmp/gdx-04384-gemini-bound-response-qualified-before-oracle.json` and
`/tmp/gdx-04384-gemini-unbound-response-qualified-before-oracle.json`.
The bound case has a successful native seed followed by exact reference 503
JSON, exit 2 and one total upstream request; old Godex returns plain 502.
The unknown-ID case has exact reference fresh-input 200, exit 0 and one
authenticated original request; old Godex times out without an upstream
request or client exchange. Missing exchanges are explicitly retained as
`client_error` and always fail independent invariants.
The harness is `/tmp/gdx-04384-gemini-binding-qualified-differential`, with
`/tmp/gdx-04384-gemini-body-godex`, the isolated release-gate source worktree,
and `--gemini --scenario raw-key-bound-response-same-process` or
`--gemini --scenario raw-key-unbound-response`.

Both new fixtures independently require exact client body/type/exit/stdout,
one original authenticated request, selected key slot and safe durable state.
The owner auditor now pins provider as well as credential and endpoint digests;
Gemini aliases derive only from an observed authenticated successful response.
Matching mutations must fail, including a foreign provider. The shared
same-process shim now rejects a failed seed before reporting any continuation
result. No state allowlist or assertion was weakened.

The first successful-replay expectation was disproved by the released binary;
`/tmp/gdx-04384-gemini-history-before-oracle.json` is an unqualified FAIL.
The first unknown-ID report also FAILed because its auditor understood only
DeepSeek ownership, before the explicit provider pin and observed alias fix:
`/tmp/gdx-04384-gemini-unbound-response-before-oracle.json`.
An unverified Gemini restart test extension timed out after 15 seconds:
`/tmp/gdx-04384-gemini-binding-focused-diagnostic.log`.
Its original unbounded run was stopped with exit 143, not PASS. The extension
was removed; DeepSeek restart/key/endpoint coverage remains, while Gemini
restart requires its own released-executable fixture before extension.

The focused regression command passes 52 tests across three packages:
`rtk go test ./internal/usecase/routing ./internal/gateway/gemini ./tools/differential -run '^TestProdex04384((Gemini)?RawKeyContinuationFailsBeforeDispatch|RawKeyRejectsInvalidContinuationIdentity|GeminiUnboundPreviousResponseUsesFreshInput|GeminiContinuationOracleRejectsMatchingMutations|GeminiContinuationOwnerPinsProvider|SameProcessProbeRequiresSuccessfulSeed)$' -count=1 -timeout=30s`;
log: `/tmp/gdx-04384-gemini-binding-reviewed-focused.log`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,746 tests: `/tmp/gdx-04384-gemini-binding-race.log`.
`rtk go test ./...` passes 9,387 tests across 81 packages:
`/tmp/gdx-04384-gemini-binding-test-all.log`.
`rtk proxy make verify` passes formatting, source/TUI guards, vet, full shuffled
race tests, cross-platform build/test compilation, installers and build:
`/tmp/gdx-04384-gemini-binding-verify.log`.
Clean-commit executable verification follows. The new unit session/turn checks
do not claim executable coverage. Gemini restart/Compact, broader lexical
classes, native macOS/Windows and live authentication remain open.
Global 1:1 parity remains unproven.


## Validated Gemini continuation checkpoint: c2895f0

Clean `c2895f00f2d44f2c9deaef88eed51d43ee79f29b` passes all 57 Gemini cases:
`/tmp/gdx-04384-gemini-binding-oracle.json`.
The first 125-case DeepSeek invocation FAILs only the reference's unknown-ID
durable-state audit: a `state.json.last-good.<pid>.<time>.<sequence>.tmp` file
remained and the backup differed from the primary. This report remains FAIL:
`/tmp/gdx-04384-gemini-binding-deepseek-oracle.json`.
No path allowlist, backup check or state invariant was relaxed.
The exact single-scenario rerun PASSes:
`/tmp/gdx-04384-gemini-binding-deepseek-probe-rerun.json`.
A second full invocation PASSes all 125 cases and independent mutations:
`/tmp/gdx-04384-gemini-binding-deepseek-rerun-oracle.json`.
The first reference teardown artifact is retained as an observed limitation,
not reclassified as a passing run or a Godex regression.

Both binaries were built from that clean commit using
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-binding-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-binding-final-differential ./tools/differential`.
The executable commands use the unchanged pinned reference, `--gemini`, or
`--scenario all --scenario-workers 3`; the isolated rerun uses
`--scenario raw-key-continuation-probe`. Source/binary provenance gates pass.
The complete default-script milestone remains scoped to clean `8730116`.
The verified continuation checkpoint was fast-forwarded and pushed directly
to `origin/main` at `c2895f00f2d44f2c9deaef88eed51d43ee79f29b`.


## Gemini raw-key durable restart rejection

The continuation audit found a separate confirmed restart gap. Godex's
`usecase/routing/ephemeral_keys.go` persisted the existing conflict sentinel
only for DeepSeek. Gemini's accepted response ownership disappeared on exit,
so a subsequent launch treated a known response ID as fresh work and sent it
upstream again. The pinned released executable instead retains ownership and
rejects the request before another attempt, even after the key or endpoint
changes. The same profile-projected dispatch identity described above applies
after restart; this does not establish successful replay.

Three strict executable baseline fixtures use clean `c2895f0` and
`/tmp/gdx-04384-gemini-restart-differential` with
`--gemini --scenario raw-key-bound-response-restart`,
`--gemini --scenario raw-key-bound-response-restart-key`, and
`--gemini --scenario raw-key-bound-response-restart-endpoint`.
All three FAIL only Godex, while reference and independent mutation controls
PASS. Each reference seed is exact native HTTP 200 with verified durable
ownership; the next process returns exact 503 JSON and exit 2, with one total
authenticated upstream request. Old Godex has no durable seed binding and
returns 200, exit 0 and a second upstream request. Reports:
`/tmp/gdx-04384-gemini-restart-before-oracle.json`,
`/tmp/gdx-04384-gemini-restart-key-before-oracle.json`, and
`/tmp/gdx-04384-gemini-restart-endpoint-before-oracle.json`.

The routing use case now applies the existing hashed conflict-sentinel policy
to verified Gemini raw-key work. No credential owner, endpoint or token enters
durable metadata; the version-1 repository format and atomic snapshot/backup
remain unchanged. Models and Compact local dispatch occurs before ordinary
binding lookup and does not create continuation identities, as in the pinned
`local_rewrite_pipeline_dispatch.rs` and `local_rewrite_gemini_compact.rs`.
This local-route policy has source-aligned routing regression coverage; it
does not claim Gemini Compact payload or executable ownership parity.
No production helper, package, interface, dependency, configuration or
background worker was introduced. The earlier restart test shortcut is removed
after the released-executable evidence; unit response/session/turn checks now
include restart, changed account ID and changed endpoint.

The extended persistence regression fails only the Gemini branch before repair:
`rtk go test ./internal/usecase/routing -run '^TestProdex04384RawKeyResponsePersistsRejectionWithoutCredentialOwner$' -count=1 -timeout=30s`;
`/tmp/gdx-04384-gemini-restart-before.log`.
The first local-route fixture lacked a profile home and failed the DeepSeek
controls as well; that report is unqualified:
`/tmp/gdx-04384-gemini-local-binding-before.log`.
With an isolated profile home and the three original production files restored
through a Go overlay, the qualified local test passes all three DeepSeek
controls and fails the three Gemini cases plus parent:
`rtk go test -overlay /tmp/gdx-04384-gemini-restart-before-overlay.json ./internal/usecase/routing -run '^TestProdex04384RawKeyLocalRoutesDoNotConsumeOrCreateBindings$' -count=1 -timeout=30s`;
`/tmp/gdx-04384-gemini-local-binding-qualified-before.log`.

The oracle retains the exact successful seed, its stdout and durable-persistence
evidence, removes the seed exchange before the next launch, and requires one
total authenticated original request. Equal corruptions of the seed,
persistence, request, final body/type/status/exit/stdout and state all fail.
Its first self-test exposed a shared synthetic header map between seed and
final results; cloning that map fixes the fixture without weakening assertions.
`rtk go test ./tools/differential -count=1` passes 266 tests:
`/tmp/gdx-04384-gemini-restart-all-oracle-tests.log`.
The focused routing/oracle command passes 26 tests:
`/tmp/gdx-04384-gemini-restart-reviewed-focused.log`.
`rtk go test -race ./internal/usecase/routing ./tools/differential -count=1`
passes 2,081 tests: `/tmp/gdx-04384-gemini-restart-race.log`.
`rtk go test ./...` passes 9,398 tests across 81 packages:
`/tmp/gdx-04384-gemini-restart-test-all.log`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-restart-verify.log`.
Clean-commit executable verification follows. Native macOS/Windows execution,
live authentication, Gemini Compact, session/turn executable continuation and
broader lexical classes remain open. Global 1:1 parity is not claimed.


## Gemini durable-state oracle qualification after restart repair

Clean `25efbbf197dbb64974cd8ae8f70d60016ea5fb97` passes the three exact restart
fixtures, including changed-key and changed-endpoint controls. Reports:
`/tmp/gdx-04384-gemini-restart-raw-key-bound-response-restart-initial-oracle.json`,
`/tmp/gdx-04384-gemini-restart-raw-key-bound-response-restart-key-initial-oracle.json`,
`/tmp/gdx-04384-gemini-restart-raw-key-bound-response-restart-endpoint-initial-oracle.json`.
The same initial harness FAILs the existing `buffered-body-under` and
`raw-key-session-limit` controls because it previously audited those workflows
without any expected durable owner. Their new synchronous hashed conflict
bindings were therefore correctly flagged as unexpected by that old contract:
`/tmp/gdx-04384-gemini-restart-buffered-body-under-initial-oracle.json` and
`/tmp/gdx-04384-gemini-restart-raw-key-session-limit-initial-oracle.json`.
These FAIL reports are retained; they are not passing restart-stage gates.

The developer-only auditor now derives response owners for all audited Gemini
successes, not only continuation cases. Accepted session-boundary fixtures
add only their exact original session value, with a distinct session-kind
hash namespace. The reference's `session_id_bindings` still require exact
provider, credential and endpoint identity plus valid timestamps. Godex still
requires the existing conflict sentinel and no credential owner. Unknown IDs,
wrong kinds, foreign identities, failed or unauthenticated responses, corrupt
backups, secret-bearing files and unknown paths remain rejected. Legacy
session maps, turn maps and status maps remain required to be empty.
No production code, schema, shared production helper or dependency changes.

Before this auditor repair,
`rtk go test ./tools/differential -run '^TestProdex04384GeminiState' -count=1`
passes the response control and fails the session, parent and unauthenticated
observation checks: `/tmp/gdx-04384-gemini-state-oracle-before.log`.
After repair, the two executable controls PASS on the same clean `25efbbf`
candidate with `/tmp/gdx-04384-gemini-state-evidence-differential`:
`/tmp/gdx-04384-gemini-state-body-reviewed-oracle.json` and
`/tmp/gdx-04384-gemini-state-session-reviewed-oracle.json`.
The candidate source remains `/tmp/gdx-04384-gemini-restart-work`; both commands
use the unchanged pinned reference and `--gemini --scenario <name>`.
This is a newly qualified harness, not a changed candidate executable.

`rtk go test ./tools/differential -count=1` passes 270 tests:
`/tmp/gdx-04384-gemini-state-oracle-tests.log`.
`rtk go test -race ./tools/differential -count=1` passes 270 tests:
`/tmp/gdx-04384-gemini-state-oracle-race.log`.
`rtk go test ./...` passes 9,402 tests across 81 packages:
`/tmp/gdx-04384-gemini-state-oracle-test-all.log`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-state-oracle-verify.log`.
Clean-commit 60-case Gemini/125-case DeepSeek matrices follow.
Gemini Compact and session/turn executable continuation,
other lexical classes, native macOS/Windows execution and live authentication
remain separate audits. Global 1:1 parity is not claimed.


## Same-process Gemini seed evidence in the durable-state oracle

The same-process fixture on clean
`ec806250e53c3dd646e4d946e8479a29113943f5` returned the exact required 503
for both products but FAILed the old auditor's Godex state check:
`/tmp/gdx-04384-gemini-durable-same-process-initial-oracle.json`.
Its final exchange alone could not authorize the durable binding established
by the successful first exchange. Inferring success from the final rejection
would weaken the independent evidence.

The shared synthetic Codex shim now retains the actual first successful
exchange in its existing private result file. The Gemini fixture independently
checks that seed's exact body, status and content type before deriving owner
evidence. A missing, failed, mutated or nested seed is rejected; matching
corruptions must fail for both products. The ordinary client stdout and
final exchange remain unchanged, and no new persistent path is permitted.
This is developer-only evidence; no Godex production code changes.

An initial regression draft did not compile and is not behavioral evidence.
The corrected regression passes the failed-seed control and fails the
successful-seed case plus parent before repair:
`rtk go test ./tools/differential -run '^TestProdex04384SameProcessProbeRequiresSuccessfulSeed$' -count=1`;
`/tmp/gdx-04384-gemini-same-process-seed-qualified-before.log`.
With the strengthened harness, the same clean ec80625 executable PASSes:
`/tmp/gdx-04384-gemini-durable-same-process-reviewed-oracle.json`.
Its immutable source is `/tmp/gdx-04384-gemini-durable-candidate-work`, the
harness is `/tmp/gdx-04384-gemini-seed-evidence-differential`, and the command
uses the unchanged pinned reference with
`--gemini --scenario raw-key-bound-response-same-process`.

`rtk go test ./tools/differential -count=1` passes 270 tests:
`/tmp/gdx-04384-gemini-same-process-seed-oracle-tests.log`.
`rtk go test -race ./tools/differential -count=1` passes 270 tests:
`/tmp/gdx-04384-gemini-seed-evidence-race.log`.
`rtk go test ./...` passes 9,402 tests across 81 packages:
`/tmp/gdx-04384-gemini-seed-evidence-test-all.log`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-seed-evidence-verify.log`.
Clean-commit full matrices follow; global 1:1 parity remains unproven.


## Validated Gemini durable continuation checkpoint: 82fe6c2

Clean `82fe6c2dfb04ceb7c8961ef40c870991a1ea4415` passes all 60 Gemini cases
and all 125 DeepSeek scenarios, including both independent mutation sets:
`/tmp/gdx-04384-gemini-durable-oracle.json` and
`/tmp/gdx-04384-gemini-durable-deepseek-oracle.json`.
The successful same-process seed is checked exactly, the three restart cases
retain durable ownership across launches, and known work is rejected before
another upstream attempt even after key/endpoint changes. The fresh/unbound
and buffered/session boundary controls also pass the qualified state auditor.

The clean binaries were built using
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-durable-final-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-durable-final-differential ./tools/differential`.
The executable commands use the unchanged pinned reference, `--gemini`, or
`--scenario all --scenario-workers 3`. The checked local/remote main was
fast-forwarded and pushed directly to `origin/main` at `82fe6c2`, including
the production restart repair and two focused oracle qualification commits.
No CI wait, force push, tag, release or editor-worktree change occurred.
The complete default-script milestone remains scoped to clean `8730116`.


## Gemini chat request integer preservation and serde numeric bytes

The pinned `chat_compatible_request.rs:44–137` parses a complete serde JSON
Value and serializes the constructed request with `serde_json::to_vec`.
Tool-schema numeric values pass through that pipeline. Godex instead decoded
the same request into float64 values three times: input normalization,
message adjacency and stream-option rewriting. An integer such as
9007199254740993 became 9007199254740992; signed/unsigned limits and the
wire spellings of integral f64 values, negative zero and unsigned overflow
also changed before reaching upstream.

The private Gemini request decoder now uses `UseNumber` and still rejects
trailing JSON and non-object roots. Its three actual consumers stay within
`gateway/gemini`. Adjacency and stream-option serialization reuse the existing
`helper/jsonutil.MarshalSerdeValue04384`, preserving integer kinds and the
pinned default f64 parser/formatter. No policy moved out of the gateway;
there is no new shared helper package, exported API, dependency, configuration,
interface, state field or worker. The complete request serde admission gate
still runs before normalization.

Locked Rust diagnostics for nine tokens are retained at
`/tmp/gdx-04384-gemini-request-numeric-serde-probe.jsonl`, produced by the
existing offline Cargo probe with the pinned serde/serde_json/zmij lockfile.
They cover a safe-integer control, integers above 2^53, signed/unsigned 64-bit
limits, exponent notation, negative zero, a long fraction and unsigned overflow
into f64. These local parser results alone are not executable parity evidence.

Before repair,
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatRequestPreservesSerdeToolNumbers$' -count=1`
passes four safe/fraction controls and fails fourteen changed-number cases
plus parent: `/tmp/gdx-04384-gemini-request-numbers-before.log`.
Each case exercises the actual HTTP pool for both request stream flags.
After repair,
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatRequest(PreservesSerdeToolNumbers|DecodersRequireCompleteObject)$' -count=1`
passes 20 tests, including complete-input controls:
`/tmp/gdx-04384-gemini-request-numbers-focused.log`.

Two strict baseline executable cases on clean `82fe6c2` FAIL only Godex:
`/tmp/gdx-04384-gemini-request-numbers-buffered-before-oracle.json` and
`/tmp/gdx-04384-gemini-request-numbers-stream-before-oracle.json`.
All reference and independent mutation invariants pass. Both require exact
original upstream request bytes, one authenticated single-key attempt, exact
native response/body/type/status/exit/stdout and safe durable state. Raw request
bytes are retained because decoded float64 maps cannot distinguish the
rounded large-integer mismatch. The true stream-flag request must also contain
the exact usage option; both mocks deliberately return buffered native JSON,
so these fixtures do not claim numeric SSE response coverage.
The baseline harness is `/tmp/gdx-04384-gemini-request-numbers-differential`
with `/tmp/gdx-04384-gemini-durable-final-godex`, the clean state-oracle
source worktree and `--gemini --scenario request-tool-numbers-buffered` or
`--gemini --scenario request-tool-numbers-stream`.

`rtk go test ./tools/differential -count=1` passes 271 tests:
`/tmp/gdx-04384-gemini-request-numbers-oracle-tests.log`.
`rtk go test -race ./internal/gateway/gemini ./internal/helper/jsonutil ./tools/differential -count=1`
passes 1,061 tests: `/tmp/gdx-04384-gemini-request-numbers-race.log`.
`rtk go test ./...` exits 0, but RTK reports a 10 MiB filter truncation and an
incomplete test count: `/tmp/gdx-04384-gemini-request-numbers-test-all.log`.
The unfiltered cached replay `rtk proxy go test -json ./...` exits 0 and records
9,423 test passes, zero failures and 81 package results:
`/tmp/gdx-04384-gemini-request-numbers-test-all-raw.jsonl`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-request-numbers-verify.log`.
Clean-commit 62-case Gemini and 125-case DeepSeek executable matrices follow.
Global 1:1 parity remains unproven; other lexical classes, Gemini Compact,
session/turn executable continuation, native macOS/Windows execution and live
authentication remain separate audits.


## Validated Gemini request numeric checkpoint: c66dd1a

Clean `c66dd1a60df3dfebfc9ac973e85c2a970ac4ec0d` passes all 62 Gemini cases
and all 125 DeepSeek scenarios, including independent matching-corruption
controls: `/tmp/gdx-04384-gemini-request-numbers-oracle.json` and
`/tmp/gdx-04384-gemini-request-numbers-deepseek-oracle.json`.
Both request stream flags preserve every locked numeric token in the exact
authenticated original upstream body, with one attempt and unchanged native
response/type/status/exit/stdout and safe durable state.
The binaries were built from that clean commit with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-request-numbers-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-request-numbers-final-differential ./tools/differential`.
Commands use the unchanged pinned reference with `--gemini`, or
`--scenario all --scenario-workers 3`. After checking the clean primary
worktree and unchanged remote, this commit was fast-forwarded and pushed
directly to `origin/main`. No CI wait, force push, tag, release or editor
checkout change occurred. The complete default-script milestone remains
scoped to clean `8730116`; global 1:1 parity remains unproven.


## Gemini user identity aliases and terminal invalid-request behavior

The pinned `chat_compatible_request.rs:268–309` calls
`deepseek_provider_core_user_id_from_responses_request` with the Gemini label.
`deepseek_bridge/request_params.rs:134–162` selects by direct field presence:
`user_id`, then `user`, then `safety_identifier`. The selected value must be
a string. `mojo/prodex_core/deepseek.mojo:1327–1357` trims Unicode whitespace,
accepts at most 512 bytes of ASCII letters, digits, underscores or dashes,
and omits a blank selected value. A null/blank primary never borrows a lower
alias. The exact released public failure for an invalid selected identity is
plain HTTP 502 `upstream request failed`, exit 2, with no upstream attempt;
the internal diagnostic is not the public 400 invalid-tool envelope.

Godex's shared Chat converter forwarded `user`, discarded `user_id`, and
rejected `safety_identifier`. That left valid identities lost/renamed or
turned into a timeout, and allowed invalid canonical identities upstream.
The Gemini gateway now normalizes only its own user aliases before the shared
converter and emits the canonical outbound `user_id` in its existing final
request-options step. Invalid identities return the existing terminal
preparation error; routing already preserves that error without model/key
retry or recovery waits. Claude's shared converter behavior is unchanged.
The normalization stays private in `gateway/gemini`, with no new shared
production helper, package, exported API, dependency, configuration, state
field or background work. The complete serde gate still precedes this logic.

The original HTTP pool regression passes the absent-ID control and fails the
nine alias/validation cases plus parent:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatUserIDUsesTaggedAliasesAndValidation$' -count=1`;
`/tmp/gdx-04384-gemini-user-id-before.log`.
An initial repaired test expected internal 400 diagnostics. The released
executable disproved that public expectation; the public-502 regression then
fails four invalid cases plus parent before error mapping:
`/tmp/gdx-04384-gemini-user-id-public-errors-before.log`.
After repair, the user-ID, previous numeric and complete-input regressions
pass 31 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChat(UserIDUsesTaggedAliasesAndValidation|RequestPreservesSerdeToolNumbers|RequestDecodersRequireCompleteObject)$' -count=1`;
`/tmp/gdx-04384-gemini-user-id-reviewed-focused.log`.
A separate two-key pool/router test requires zero upstream attempts, no wait
and the exact terminal preparation error, without production test hooks.

The first executable reports used the incorrect 400 expectation for invalid
identities; canonical/safety failures also stopped before reporting missing
child exchanges. These unqualified results remain at
`/tmp/gdx-04384-gemini-user-id-<suffix>-before-oracle.json` and `.stderr`.
The qualified harness retains a missing exchange as explicit failing evidence
and requires the actual released 502. Ten clean-c66dd1a baseline cases PASS
the absent control and FAIL only Godex for canonical, user-alias, safety-alias,
blank, trim-limit, null, number, over-limit and invalid-character suffixes:
`/tmp/gdx-04384-gemini-user-id-<suffix>-qualified-before-oracle.json`.
The two-key number-rotation case also FAILs only Godex, with exact terminal
502 and no reference upstream attempt:
`/tmp/gdx-04384-gemini-user-id-number-rotation-qualified-before-oracle.json`.
All reference and independent mutation contracts pass. Commands use
`/tmp/gdx-04384-gemini-user-id-qualified-differential` (the rotation case uses
`/tmp/gdx-04384-gemini-user-id-qualified-rotation-differential`), the clean
numeric-request candidate/source and `--gemini --scenario request-user-id-<suffix>`.
The fixtures require exact original authenticated request bytes, alias
priority, status/type/body/exit/stdout, zero or one upstream attempt and safe
durable state. Equal corrupted requests and responses must fail independently.

`rtk go test ./tools/differential -count=1` passes 272 tests after correcting
the public oracle contract:
`/tmp/gdx-04384-gemini-user-id-qualified-oracle-tests.log`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,795 tests: `/tmp/gdx-04384-gemini-user-id-race.log`.
`rtk proxy go test -json ./...` passes 9,436 tests across 81 packages:
`/tmp/gdx-04384-gemini-user-id-test-all.jsonl`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-user-id-verify.log`.
Clean-commit 73-case Gemini/125-case DeepSeek matrices follow.
Other request controls, combined validation-error ordering,
Gemini Compact, session/turn executable continuation, native macOS/Windows
and live authentication remain separate audits. Global 1:1 parity is not claimed.


## Validated Gemini user identity checkpoint: 4450527

Clean `4450527691a17a3a1db7b08bdfabdb05dfb78acc` passes all 73 Gemini cases
and all 125 DeepSeek scenarios, including both independent mutation sets:
`/tmp/gdx-04384-gemini-user-id-oracle.json` and
`/tmp/gdx-04384-gemini-user-id-deepseek-oracle.json`.
Valid aliases and trim/length controls preserve the exact canonical original
request. Invalid identities, including two configured keys, produce the exact
terminal plain 502 with no upstream attempt, while all preceding numeric,
buffered, continuation and stream fixtures remain matched.

Clean builds use
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-user-id-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-user-id-final-differential ./tools/differential`.
The executable commands use the unchanged pinned reference, `--gemini`, or
`--scenario all --scenario-workers 3`. The clean primary worktree and unchanged
remote were checked before direct fast-forward push to `origin/main` at
`4450527`. No CI wait, force push, tag, release or editor-worktree change
occurred. The complete default-script milestone remains scoped to clean
`8730116`; global 1:1 parity remains unproven.


## Gemini ignores the parallel-tool control after complete JSON validation

The pinned Gemini chat request path deliberately skips
`deepseek_provider_core_reject_unsupported_request_fields` and constructs a
new outbound map. Its primitive-field kernel does not insert
`parallel_tool_calls`; the Gemini bridge therefore ignores that input member.
The shared Godex Chat converter instead rejected false and forwarded other
values. This is adapter policy, not a change to Claude or DeepSeek's own
parallel-tool contracts.

The production repair is one deletion after the existing complete serde
admission gate in `gateway/gemini/compatibility_043711.go`. Malformed ignored
values remain invalid. No helper, package, API, dependency, configuration,
interface, persistent state or background work was introduced.

Before repair,
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatIgnoresParallelToolControl$' -count=1`
fails all seven value cases plus parent:
`/tmp/gdx-04384-gemini-parallel-control-before.log`.
The cases cover true, false, null, number, string, array and object. An
additional negative fixture initially mislabeled 1.7976931348623157e308 as
invalid. The locked offline Rust probe confirms that number is valid and
rejects 1e309, an unpaired surrogate and truncated JSON:
`/tmp/gdx-04384-gemini-parallel-control-serde-probe.jsonl`.
The corrected negative gate passes without changing the serde implementation.
The earlier failed fixture logs remain FAIL observations.

All seven strict executable baseline cases FAIL only Godex on clean
`4450527691a17a3a1db7b08bdfabdb05dfb78acc`, with every reference and independent
invariant satisfied:
`/tmp/gdx-04384-gemini-parallel-<true|false|null|number|string|array|object>-before-oracle.json`.
The false case exposes a missing old Godex exchange as an explicit failing
observation. The other six show the ignored member leaking into upstream.
Commands use `/tmp/gdx-04384-gemini-parallel-control-differential`, the clean
user-ID binary/source, the unchanged pinned reference, and
`--gemini --scenario request-parallel-<suffix>`.
Every fixture requires byte-exact original authenticated upstream input with
the control absent, one attempt, exact native response/status/type/exit/stdout
and safe durable state. The existing strict fresh-response invariant is reused;
matching leaked raw control bytes are rejected independently.

`rtk go test ./tools/differential -count=1` passes 273 tests before production
repair: `/tmp/gdx-04384-gemini-parallel-control-oracle-tests.log`.
The corrected focused command passes 39 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChat(IgnoresParallelToolControl|UserIDUsesTaggedAliasesAndValidation|RequestPreservesSerdeToolNumbers|RequestDecodersRequireCompleteObject)$' -count=1`;
`/tmp/gdx-04384-gemini-parallel-control-reviewed-focused.log`.
`rtk go test -race ./internal/gateway/gemini ./tools/differential -count=1`
passes 989 tests: `/tmp/gdx-04384-gemini-parallel-control-race.log`.
`rtk proxy go test -json ./...` passes 9,445 tests across 81 packages:
`/tmp/gdx-04384-gemini-parallel-control-test-all.jsonl`.
The fixture's repeated state-audit condition is kept in one local boolean;
`rtk go test -race ./tools/differential -count=1` verifies the final cleanup:
`/tmp/gdx-04384-gemini-parallel-control-final-oracle-tests.log`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-parallel-control-verify.log`.
Clean-commit 80-case Gemini/125-case DeepSeek matrices follow.
Other controls and combined-error ordering, Gemini Compact,
session/turn executable continuation, native macOS/Windows and live
authentication remain separate audits. Global 1:1 parity is not claimed.


## Validated Gemini ignored-control checkpoint: 163d67e

Clean `163d67eb9043439554e7a5dd9662c939db7bdb27` passes all 80 Gemini cases
and all 125 DeepSeek scenarios, with both independent mutation sets:
`/tmp/gdx-04384-gemini-parallel-control-oracle.json` and
`/tmp/gdx-04384-gemini-parallel-control-deepseek-oracle.json`.
All seven parallel-control value kinds disappear from the exact authenticated
upstream request. The false control no longer becomes a local rejection,
and preceding request/response, state, continuation and retry cases remain
matched. The source/binary provenance gates use the unchanged pinned reference.

The clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-parallel-control-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-parallel-control-final-differential ./tools/differential`.
Commands use `--gemini`, or `--scenario all --scenario-workers 3`.
The checked clean primary worktree and unchanged remote were fast-forwarded
and pushed directly to `origin/main` at `163d67e`, without CI wait, force push,
tag, release or editor-checkout changes. The complete default-script milestone
remains scoped to `8730116`; global 1:1 parity remains unproven.


## Gemini complete-body admission and valid nonobject outcomes

The API-key path parses a complete serde Value before request-shape validation.
Godex's complete boundary check already rejected invalid Unicode, numeric
range and depth, and its decoder rejected malformed/trailing JSON, but its
ordinary 400 error entered routing recovery and could time out. A null root
also normalized without error before the shared converter rejected it.

Released-executable observations distinguish two outcomes. Malformed/trailing
JSON, invalid surrogate/UTF-8, numeric overflow and excess depth return exact
terminal plain 502 `upstream request failed`. Valid null, array, number,
boolean and string roots return exact empty 500 with no content type.
Both have exit 2 and zero upstream attempts. The latter result is an observed
public contract; the repair does not emulate an upstream panic or infer one.

`gateway/gemini/compatibility_043711.go` now preserves the terminal parse error
and distinguishes a decoder type error/null map from a syntax/completeness
failure. A valid nonobject is presented as the exact empty 500, carrying local
`invalid_request_error` precommit evidence so ordinary status-500 model/key
retry cannot apply. All logic stays in the existing gateway and model boundary;
the routing use case consumes its existing terminal classification. No new
production helper, package, exported identifier, dependency, configuration,
state format or worker was introduced. Native OAuth/GenerateContent admission
remains a separate surface.

The initial regression assumed every invalid root returned 502. The first
executable root reports correctly FAIL that expectation on the reference:
`/tmp/gdx-04384-gemini-json-<root-null|root-array|root-number>-before-oracle.json`.
They remain unqualified. Corrected regression expectations fail all eleven
admission cases plus parent before repair:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatJSONAdmissionReturnsTerminalPreparationError$' -count=1`;
`/tmp/gdx-04384-gemini-json-admission-qualified-before.log`.
The first repair incorrectly grouped syntax/trailing decode errors with root
type errors. Two focused tests plus parent caught that mistake:
`/tmp/gdx-04384-gemini-json-admission-reviewed-focused.log`.
After distinguishing the actual decoder error class, the final focused command
passes 98 tests, including two-key pool/router no-attempt/no-wait coverage:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(ChatJSONAdmissionReturnsTerminalPreparationError|JSONAdmissionDoesNotRetryModelOrCredential|ControlTypesAreCheckedBeforeNormalization|RequestRequiresCompleteSerdeJSON|Chat(IgnoresParallelToolControl|UserIDUsesTaggedAliasesAndValidation|RequestPreservesSerdeToolNumbers|RequestDecodersRequireCompleteObject))$' -count=1 -timeout=30s`;
`/tmp/gdx-04384-gemini-json-admission-final-focused.log`.

Twelve qualified baseline cases use clean
`293b1b6cb6d543875847b6aeec0a5e8750a1360b`, the unchanged pinned reference and
`/tmp/gdx-04384-gemini-json-admission-qualified-differential`.
The valid-unicode control PASSes both products; five valid nonobject root
kinds and six malformed/serde classes FAIL only Godex. All reference and
independent mutation invariants pass:
`/tmp/gdx-04384-gemini-json-<suffix>-qualified-before-oracle.json`.
The null-rotation case also FAILs only Godex with the same exact reference
500 and no attempt despite two configured keys:
`/tmp/gdx-04384-gemini-json-root-null-rotation-qualified-before-oracle.json`.
Its harness is `/tmp/gdx-04384-gemini-json-admission-qualified-rotation-differential`.
Commands use the clean control-types candidate/source and
`--gemini --scenario request-json-<suffix>`. Independent strict contracts retain
status, empty or exact body, type presence/absence, child exit, stdout,
attempts, original authenticated request for the positive control and safe
durable state. Equal corruptions and missing exchanges fail.

`rtk go test ./tools/differential -count=1` passes 275 tests after correcting
the root expectation: `/tmp/gdx-04384-gemini-json-admission-qualified-oracle-tests.log`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,846 tests: `/tmp/gdx-04384-gemini-json-admission-race.log`.
`rtk proxy go test -json ./...` passes 9,487 tests across 81 packages:
`/tmp/gdx-04384-gemini-json-admission-test-all.jsonl`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-json-admission-verify.log`.
Clean-commit 102-case Gemini/125-case DeepSeek matrices follow.
Combined-error ordering, other controls, Gemini Compact,
session/turn executable continuation, native macOS/Windows and live
authentication remain open. Global 1:1 parity is not claimed.


## Gemini control-type admission before continuation normalization

The pinned `provider_core_chat_compatible_validate_top_level_request_shape`
requires present `stream` values to be booleans and present `instructions`
and `previous_response_id` values to be strings. This validation runs before
request construction in `chat_compatible_request.rs:44–137`. Godex admitted
other JSON types: its Chat converter defaulted invalid stream values to false,
ignored invalid instructions, and its fresh-input normalization removed a
non-string previous-response ID. Such a request could become fresh upstream
work instead of the tagged terminal local preparation failure.

The private Gemini adapter now checks these three control types after complete
serde decoding and before user-ID/continuation normalization. Invalid input
returns the existing terminal plain 502 `upstream request failed`; the existing
routing preparation-error boundary prevents a model/key replay or wait.
The check stays in `gateway/gemini`, with no new production package, shared
helper, exported identifier, dependency, state field, configuration or worker.
This stage does not change model selection or claim all top-level fields.

Before repair, the table-driven regression passes six valid-type controls and
fails eighteen invalid-type cases plus parent:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiControlTypesAreCheckedBeforeNormalization$' -count=1`;
`/tmp/gdx-04384-gemini-control-types-before.log`.
It covers null, booleans, strings, number, array and object values for each
field. After repair, the new regression and preceding Gemini request contracts
pass 64 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(ControlTypesAreCheckedBeforeNormalization|Chat(IgnoresParallelToolControl|UserIDUsesTaggedAliasesAndValidation|RequestPreservesSerdeToolNumbers|RequestDecodersRequireCompleteObject))$' -count=1`;
`/tmp/gdx-04384-gemini-control-types-focused.log`.

Nine strict baseline cases use clean
`163d67eb9043439554e7a5dd9662c939db7bdb27` and the unchanged pinned reference.
The stream-false control PASSes both products; stream-null/string/number,
instructions-null/array and previous-null/array/number FAIL only Godex.
All reference and independent matching-corruption invariants are satisfied:
`/tmp/gdx-04384-gemini-control-<suffix>-before-oracle.json`.
Invalid reference requests return the exact plain 502, exit 2 and zero upstream
attempts. Old Godex returns 200 after submitting normalized input upstream.
The harness is `/tmp/gdx-04384-gemini-control-types-differential`, the candidate
is `/tmp/gdx-04384-gemini-parallel-control-godex`, and commands use the clean
parallel-control source with `--gemini --scenario request-control-<suffix>`.
Fixtures reuse the exact accepted/preparation-failure contracts already
independently checked for user IDs; they still require exact client body/type/
status/exit/stdout, authenticated original raw request where accepted and safe
durable state. Missing child exchanges never satisfy the contract.

`rtk go test ./tools/differential -count=1` passes 274 tests before the
production repair: `/tmp/gdx-04384-gemini-control-types-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,471 tests across 81 packages:
`/tmp/gdx-04384-gemini-control-types-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,830 tests: `/tmp/gdx-04384-gemini-control-types-race.log`.
`rtk proxy make verify` passes all local completion gates:
`/tmp/gdx-04384-gemini-control-types-verify.log`.
Clean-commit 89-case Gemini/125-case DeepSeek matrices follow.
Other controls, combined validation-error
ordering, Gemini Compact, session/turn executable continuation, native
macOS/Windows and live authentication remain open. Global 1:1 parity is not claimed.


## Validated maximum-token alias checkpoint: eb82ab8

Clean `eb82ab80ad593b68b0e60ea3736dabc18a3241d6` passes all 111 Gemini cases
and all 134 DeepSeek scenarios, including independent mutation controls:
`/tmp/gdx-04384-request-token-limits-gemini-oracle.json` and
`/tmp/gdx-04384-request-token-limits-deepseek-oracle.json`.
Each report pins the unchanged 0.438.4 source/binary and exact clean candidate.
The previously recorded 44 focused tests, 3,562 race tests, 9,522 full tests
and `make verify` also pass. Commits `767edea` and `eb82ab8` were fast-forwarded
through the clean primary worktree and pushed directly to `origin/main`,
without waiting for CI. No force push, tag, release or editor-checkout changes
occurred. This checkpoint covers the named matrices, not global 1:1 parity.

The complete default gate was separately attempted from protected clean
`293b1b6cb6d543875847b6aeec0a5e8750a1360b`:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-293b1b6.log 2>&1`.
That invocation FAILed with exit 1 on
`single-key-503.prodex.state_integrity`: the reference left an empty
`state/runtime-scores.json.last-good.480291.1792644775456955623.3.tmp`,
correctly rejected as `nonempty_or_corrupt_prodex_health_scores`.
The strict isolated scenario rerun PASSes:
`/tmp/gdx-04384-release-293b1b6-single-key-503-rerun.json`.
The full unchanged-script rerun remains pending:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-293b1b6-rerun.log 2>&1`.
No assertion or state allowlist was weakened; the first report remains FAIL.
Until a complete invocation exits 0, the complete default-gate PASS remains
scoped to historical `8730116`, not any later commit or a partial phase marker.


## Numeric-control admission and canonical DeepSeek request bytes

The pinned `deepseek_primitive_core_plan` requires every present `temperature`
and `top_p` to be a JSON number. Rust's complete chat-compatible path parses
serde Value before policy and serializes the final Value with `serde_json`.
DeepSeek already rejected wrong types but returned an ordinary error that
could enter routing recovery. Gemini admitted wrong types. DeepSeek's final
standard-library encoder also preserved original numeric lexemes and escaped
HTML-sensitive and Unicode line-separator characters differently from serde.

`gateway/deepseek` now maps numeric-type failures through the existing terminal
preparation boundary and reuses `helper/jsonutil.MarshalSerdeValue04384` for
the complete outbound request. `gateway/gemini` checks present numeric types
before normalization. These are concrete protocol-adapter responsibilities;
the existing stateless JSON helper already has real consumers and owns no
routing policy or I/O. No dependency, exported API, package, interface, config,
worker or persisted format is added. The shared Claude converter is unchanged.

The new narrow regressions and existing terminal-admission table fail before
repair: 23 tests pass and 21 fail, including parents:
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini -run '^TestProdex(04384(DeepSeekRequestUsesCanonicalSerdeNumbersAndText|GeminiNumericControlsRequireNumberTypes)|04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-request-numeric-fields-before.log`.
The same command passes 44 tests after repair:
`/tmp/gdx-04384-request-numeric-fields-focused.log`.
An offline locked-dependency serde probe retains seven JSON records:
`/tmp/gdx-04384-request-numeric-fields-serde-probe.jsonl`.

Twenty executable baseline cases use clean `eb82ab8`, the pinned reference
and `/tmp/gdx-04384-request-numeric-fields-differential`:
`/tmp/gdx-04384-request-<gemini|deepseek>-<numeric-suffix>-before-oracle.json`.
All six accepted Gemini controls PASS; four invalid-type cases FAIL only Godex.
DeepSeek's ordinary and u64 controls PASS; four canonical-wire and four
invalid-type cases FAIL only Godex. Every reference and independent mutation
invariant passes. Numeric suffixes and byte-exact expectations are documented
in `tools/differential/README.md`; invalid types require plain 502, exit 2 and
zero attempts. Accepted cases retain authenticated original raw input, exact
response/status/type/stdout/exit and safe durable-state checks.

`rtk go test ./tools/differential -count=1` passes 276 tests:
`/tmp/gdx-04384-request-numeric-fields-oracle-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/helper/jsonutil ./internal/usecase/routing ./tools/differential -count=1`
passes 3,686 tests: `/tmp/gdx-04384-request-numeric-fields-race.log`.
`rtk proxy go test -json ./...` passes 9,552 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-request-numeric-fields-test-all.jsonl`.
`rtk proxy make verify` passes the local completion gate:
`/tmp/gdx-04384-request-numeric-fields-verify.log`.
Clean-commit 121-case Gemini and 144-case DeepSeek matrices follow.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Logprobs control admission and Gemini request preservation

The pinned chat-compatible optional-field path invokes primitive validation
and `deepseek_provider_core_top_logprobs_from_responses_request` for both
DeepSeek and Gemini. `deepseek.mojo:2190–2213` requires unsigned integer digits
from zero through 20 and an explicit true `logprobs` flag. Present `logprobs`
must be a boolean. Signed negative zero is a serde float, not an integer.

Godex's shared Claude converter rejects all logprobs controls, so Gemini
incorrectly rejected even valid false/zero/20 inputs. Invalid fields also
entered ordinary routing recovery. DeepSeek accepted negative counts and
negative zero, used signed parsing for unsigned values and returned ordinary
errors for invalid types, range and flag dependency.

The concrete DeepSeek adapter now parses unsigned counts and uses its existing
terminal preparation-error class for all four failures. The Gemini adapter
validates and extracts its two controls before shared conversion, carries
them explicitly as a private local map and restores them during final serde
encoding. The existing private normalization signatures and their real tests
are updated together. The shared Claude converter retains its independent
contract. All production policy stays in the concrete protocol gateways;
no new shared helper, package, exported API, interface, dependency, config,
worker or state format is introduced. The private Gemini file owns one
cohesive request-control concern and avoids a cross-gateway dependency.

Before repair, 20 regression controls pass and 37 tests fail, including parents:
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini -run '^TestProdex(04384(DeepSeekLogprobsAdmission|GeminiLogprobsAdmissionAndWire)|04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-request-logprobs-before.log`.
The same command passes the original 57 tests after repair:
`/tmp/gdx-04384-request-logprobs-focused.log`.
The final gateway suite additionally covers a true flag alone and valid
stream/identity controls together, asserting exact wire bytes, one
upstream attempt and a successful native response. Invalid cases require
the existing terminal 502 with zero attempts.

Twenty-four executable baselines use clean
`bb98216f698b6a018d19c8b9c45534b57936a973`, the pinned reference and
`/tmp/gdx-04384-request-logprobs-differential`:
`/tmp/gdx-04384-request-<gemini|deepseek>-<logprobs-suffix>-before-oracle.json`.
All twelve Gemini cases FAIL only Godex. Three accepted DeepSeek controls PASS;
nine invalid-field cases FAIL only Godex. Every reference contract and
independent matching-corruption control passes. The existing strict primitive
fixture family now covers exact valid false/zero/20 requests and terminal
null/string/count/range/dependency outcomes without accepting missing child
exchanges, equal corruptions or unsafe durable state.

`rtk go test ./tools/differential -count=1` passes 276 tests:
`/tmp/gdx-04384-request-logprobs-oracle-tests.log`.
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/gateway/chatcompat -count=1`
passes 1,606 tests: `/tmp/gdx-04384-request-logprobs-gateway-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,633 tests: `/tmp/gdx-04384-request-logprobs-race.log`.
`rtk proxy go test -json ./...` passes 9,593 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-request-logprobs-test-all.jsonl`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-request-logprobs-verify.log`.
Clean-commit 133-case Gemini and 156-case DeepSeek matrices follow.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Validated numeric-control checkpoint: bb98216

Clean `bb98216f698b6a018d19c8b9c45534b57936a973` passes all 121 Gemini cases
and all 144 DeepSeek scenarios, including their independent mutation checks:
`/tmp/gdx-04384-request-numeric-fields-gemini-oracle.json` and
`/tmp/gdx-04384-request-numeric-fields-deepseek-oracle.json`.
The exact clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-request-numeric-fields-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-request-numeric-fields-final-differential ./tools/differential`.
The matrices use the unchanged pinned reference and
`--gemini`, or `--scenario all --scenario-workers 3`, with the clean numeric
source and `--godex-commit bb98216f698b6a018d19c8b9c45534b57936a973`.
Targeted exponent DeepSeek and null-temperature Gemini reports also PASS:
`/tmp/gdx-04384-request-deepseek-numeric-exponent-targeted-oracle.json` and
`/tmp/gdx-04384-request-gemini-numeric-temperature-null-targeted-oracle.json`.
The clean primary worktree and unchanged remote were checked, fast-forwarded
and pushed directly to `origin/main` at `bb98216` without waiting for CI.
No force push, tag, release or editor-checkout changes occurred.
The separate full default-script rerun remains pending on protected `293b1b6`;
this numeric checkpoint does not advance that gate or establish global parity.


## Stop-control presence, aliases and terminal admission

Pinned `deepseek_provider_core_stop_from_responses_request` selects the first
present `stop`, `stop_sequences` or `stopSequences`, including a present null.
The shared chat-compatible Rust path applies it for both providers.
`deepseek.mojo:2216–2260` accepts a string or an array of at most 16 strings.
Empty strings/arrays and whitespace are retained; malformed lower-priority
aliases do not replace a valid selected value.

Godex's Gemini Chat converter discarded canonical/camel-case controls and
rejected the snake-case alias, while invalid canonical types could reach
upstream unchanged as fresh work without a stop control. DeepSeek already
projected the aliases correctly, but its ordinary validation errors could
enter routing recovery rather than the tagged terminal preparation failure.

The Gemini adapter now validates its selected stop value, deletes all three
aliases before shared conversion and carries only the canonical field through
its existing explicit private controls map. It reuses the package-local
`firstPresent` lookup and existing final serde writer. The concrete DeepSeek
adapter wraps its existing stop error class at the preparation boundary.
Protocol policy remains local to each gateway; the shared Claude converter,
routing ownership and state formats are unchanged. No production shared
helper, package, exported API, interface, dependency, configuration or worker
is introduced. The combined stream/identity/logprobs regression now also
requires its stop value, proving the private controls survive together.

Before repair, 34 regression controls pass and 25 tests fail, including parents:
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini -run '^TestProdex(04384(DeepSeekStopAdmissionAndAliases|GeminiStopAdmissionAndAliases)|04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-request-stop-before.log`.
The same command passes 59 tests after repair:
`/tmp/gdx-04384-request-stop-focused.log`.

Twenty-six executable baseline reports pin clean
`f4bf577507cacd03abcd6d7fbbf8f49fc51f5271`, the unchanged reference and
`/tmp/gdx-04384-request-stop-differential`:
`/tmp/gdx-04384-request-<gemini|deepseek>-<stop-suffix>-before-oracle.json`.
All thirteen Gemini cases FAIL only Godex. Eight valid DeepSeek controls PASS;
five invalid-field cases FAIL only Godex. All reference and independent
matching-corruption invariants pass. The strict primitive fixture family
retains exact original authenticated requests for accepted controls, terminal
502/exit 2/no attempt for invalid controls, exact response/status/type/stdout
and safe durable state. Empty values and selected null are distinct cases.

`rtk go test ./tools/differential -count=1` passes 276 tests:
`/tmp/gdx-04384-request-stop-oracle-tests.log`.
`rtk go test ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/gateway/chatcompat -count=1`
passes 1,638 tests: `/tmp/gdx-04384-request-stop-gateway-tests.log`.
`rtk proxy go test -json ./...` passes 9,625 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-request-stop-test-all.jsonl`.
`rtk go test -race ./internal/gateway/deepseek ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,665 tests: `/tmp/gdx-04384-request-stop-race.log`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-request-stop-verify.log`.
Clean-commit 146-case Gemini and 169-case DeepSeek matrices follow.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Complete default release gate checkpoint: 293b1b6

The full unchanged default command exits 0 from protected clean
`293b1b6cb6d543875847b6aeec0a5e8750a1360b`:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-293b1b6-rerun.log 2>&1`.
The process completed every stage, including HTTP profile-status and raw-429
checks after its `PASS: ... phase all` marker. The worktree remains clean at
that exact revision. This advances the complete default-gate checkpoint from
`8730116` to `293b1b6`, not to any subsequent gateway change.
The first invocation remains FAIL in `/tmp/gdx-04384-release-293b1b6.log`;
its reference leftover-file invariant and narrow scenario rerun remain recorded
above. No assertion or state allowlist was relaxed. Global parity is unproven.


## Preserve time-bound oracle evidence at scenario completion

The original clean `f4bf577` DeepSeek matrix returns FAIL in
`/tmp/gdx-04384-request-logprobs-deepseek-oracle.json`, with exactly
`deepseek-sse-terminal.prodex.embedded_stream_error` and its Godex counterpart.
Both products supplied the expected one terminal SSE frame, exact provider
code/message, one primary-key attempt, exit 0 and safe state. The raw report
remains FAIL: its evidence is not retroactively relabeled.

`stream_error_contract.go` correctly requires a generated timestamp within
five minutes of observation. The runner incorrectly evaluated independent
invariants again after every parallel scenario had finished. A long matrix
aged the early valid SSE result beyond that window. The scenario executor now
checks all independent invariants immediately after each scenario completes,
stores failures in that scenario's existing ordered comparison, and the final
report consumes those results. The late repeated check is removed. Timestamp
limits, strict fields, request/response bytes, state checks and negative
controls are unchanged; this is test-harness ownership, not product policy.

The new deterministic executor regression uses fresh, stale and future frames
without sleeping. Before repair one control passes and three tests fail,
including the parent:
`rtk go test ./tools/differential -run '^TestProdex04384ScenarioCompletionChecksTimeBoundSSEInvariant$' -count=1`;
`/tmp/gdx-04384-scenario-invariants-before.log`.
After repair, `rtk go test ./tools/differential -count=1` passes 280 tests:
`/tmp/gdx-04384-gemini-ignored-sampling-final-oracle-tests.log`.
Stale and future timestamps retain their exact two product-specific failing
invariants. Existing concurrent isolation and ordered-report tests still pass.

A separate temporary oracle checkout at `f4bf577` contains only this executor
timing repair and its regression, retaining the exact 156-case vocabulary.
`rtk go test ./tools/differential -count=1` passes 280 tests there:
`/tmp/gdx-04384-logprobs-oracle-clock-tests.log`.
Its harness is `/tmp/gdx-04384-logprobs-oracle-clock-differential`.
The unchanged clean `f4bf577` product binary/source are being rechecked in
`/tmp/gdx-04384-request-logprobs-deepseek-clock-oracle.json`.
The equivalent repaired harness with the 169-case stop vocabulary is
`/tmp/gdx-04384-scenario-invariants-fixed-differential`; its clean `ffca69a`
candidate rerun is `/tmp/gdx-04384-request-stop-deepseek-clock-oracle.json`.
Both reruns are pending. Candidate source and binary pins are unchanged;
the temporary oracle patches do not modify either protected candidate.


## Gemini ignores unprojected sampling controls

The pinned chat-compatible runtime invokes the shared primitive projection,
whose `deepseek.mojo:1177–1213` emits only temperature, top-p, maximum tokens
and logprobs. It omits `presence_penalty`, `frequency_penalty`, `seed` and `n`.
Gemini skips DeepSeek's unsupported-field rejection, so these controls are
ignored regardless of their valid JSON type. Godex forwarded the first three
through its shared Claude converter and incorrectly rejected `n:7`.

The Gemini normalization step now removes these four fields alongside its
already ignored parallel-tool control, after complete serde admission. Invalid
Unicode, overflow or malformed ignored values still fail terminally. The
three-line removal loop is private gateway protocol behavior; no shared Claude
or DeepSeek policy, production helper, API, interface, package, dependency,
config, worker or persisted format is added or changed.

Before repair six controls pass and 23 tests fail, including the parent:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatIgnoresUnprojectedSamplingControls$' -count=1`;
`/tmp/gdx-04384-gemini-ignored-sampling-before.log`.
The new and prior ignored-field regressions pass 37 tests after repair:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384GeminiChatIgnores(UnprojectedSamplingControls|ParallelToolControl)$' -count=1`;
`/tmp/gdx-04384-gemini-ignored-sampling-focused.log`.

Twenty-eight executable baseline reports use clean `ffca69a`, the pinned
reference and `/tmp/gdx-04384-gemini-ignored-sampling-differential`:
`/tmp/gdx-04384-gemini-ignored-<presence_penalty|frequency_penalty|seed|n>-<kind>-before-oracle.json`.
Twenty-two cases FAIL only Godex; six nonnumeric n controls PASS. Every
reference and independent matching-leak invariant passes. The existing parallel
fixture runner is extended and renamed for ignored controls, retaining its
seven old scenario names. New cases require exact authenticated original
request bytes with ignored fields absent, one attempt, exact native output/
status/type/stdout/exit and safe state. Matching leaked controls still fail.

`rtk go test ./tools/differential -count=1` passes 276 tests before the separate
executor regression: `/tmp/gdx-04384-gemini-ignored-sampling-oracle-tests.log`.
The final oracle suite passes 280 tests as recorded above.
`rtk proxy go test -json ./...` passes 9,658 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-gemini-ignored-sampling-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 2,944 tests: `/tmp/gdx-04384-gemini-ignored-sampling-race.log`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-gemini-ignored-sampling-verify.log`.
Clean-commit 174-case Gemini and 169-case DeepSeek matrices follow.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Qualified clock repair and reference seed-state failure

The repaired-oracle 156-case DeepSeek rerun PASSes for unchanged clean
`f4bf577507cacd03abcd6d7fbbf8f49fc51f5271`, with independent mutation checks:
`/tmp/gdx-04384-request-logprobs-deepseek-clock-oracle.json`.
The exact command is
`rtk proxy /tmp/gdx-04384-logprobs-oracle-clock-differential --prodex /tmp/godex-release-parity-04384-20261021/prodex-x86_64-unknown-linux-gnu --godex /tmp/gdx-04384-request-logprobs-godex --prodex-source /tmp/godex-release-parity-04384-20261021/prodex-source --godex-source /tmp/gdx-04384-request-logprobs-work --godex-commit f4bf577507cacd03abcd6d7fbbf8f49fc51f5271 --scenario all --scenario-workers 3`.
Its original matrix remains FAIL for the late timestamp checks as recorded above.

The original 169-case stop matrix also FAILs only the two product-specific
terminal-SSE timestamp invariants:
`/tmp/gdx-04384-request-stop-deepseek-oracle.json`.
No additional comparison or state difference is reported. The repaired-oracle
169-case rerun PASSes for clean `ffca69a`, including independent mutations:
`/tmp/gdx-04384-request-stop-deepseek-clock-oracle.json`.
The first report is not relabeled.

The 133-case clean `f4bf577` Gemini invocation FAILs one strict reference
invariant: `prodex: Gemini restart lacks an exact successful seed with durable ownership`.
Report: `/tmp/gdx-04384-request-logprobs-gemini-oracle.json`.
The `raw-key-bound-response-restart-key` reference seed has the exact successful
200 output, one authenticated request and a persisted binding, but the durable
auditor finds
`unknown_persistent_file:state/state.json.last-good.985671.1792647113398884097.7.tmp`.
The strict isolated rerun of that unchanged scenario PASSes:
`/tmp/gdx-04384-request-logprobs-gemini-restart-key-rerun.json`.
The other 132 cases match; no Godex invariant fails. This does not turn the
original complete 133-case invocation into PASS. No persistence allowlist,
seed requirement or reference binary was changed. The new clean `0ad214c`
174-case Gemini matrix remains pending and includes all those earlier cases.

Clean `0ad214cb731dae535f330e530accb2eb6c2ec028` targeted ignored-null-penalty,
ignored numeric n and terminal-SSE cases PASS with the committed final harness:
`/tmp/gdx-04384-gemini-ignored-presence_penalty-null-targeted-oracle.json`,
`/tmp/gdx-04384-gemini-ignored-n-number-targeted-oracle.json` and
`/tmp/gdx-04384-scenario-invariants-terminal-targeted-oracle.json`.
The full 174-case Gemini/169-case DeepSeek matrices are pending:
`/tmp/gdx-04384-gemini-ignored-sampling-oracle.json` and
`/tmp/gdx-04384-gemini-ignored-sampling-deepseek-oracle.json`.
The source-pin worktrees remain clean and unchanged while these execute.
The complete default-gate PASS remains scoped to `293b1b6`.
Global 1:1 parity is not claimed.


## Validated stop-control checkpoint: ffca69a

Clean `ffca69a56287e082b076f4ba52711a95af8e9a29` passes all 146 Gemini cases
and all 169 DeepSeek scenarios, including independent mutation controls:
`/tmp/gdx-04384-request-stop-gemini-oracle.json` and
`/tmp/gdx-04384-request-stop-deepseek-clock-oracle.json`.
The DeepSeek report uses the explicitly qualified completion-time oracle
repair described above; its original timestamp-window FAIL is retained.
The preceding `f4bf577` 133-case Gemini reference-state FAIL and strict isolated
PASS also remain separate evidence, not a retrospectively successful matrix.
The broader clean stop candidate includes and passes all those request and
continuation scenarios. The product binaries and candidate-source pins remain
exact and unchanged throughout qualification.

The clean stop binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-request-stop-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-request-stop-final-differential ./tools/differential`.
The targeted empty-array Gemini and shadow-null DeepSeek reports also PASS:
`/tmp/gdx-04384-request-gemini-stop-empty-array-targeted-oracle.json` and
`/tmp/gdx-04384-request-deepseek-stop-shadow-null-targeted-oracle.json`.
The clean primary worktree and unchanged remote were checked and fast-forwarded;
commits `f4bf577` and `ffca69a` were pushed directly to `origin/main` without
waiting for CI. No force push, tag, release or editor-checkout change occurred.
Clean `0ad214c` also passes its complete 169-case DeepSeek matrix with the
committed repaired oracle:
`/tmp/gdx-04384-gemini-ignored-sampling-deepseek-oracle.json`.
Its 174-case Gemini invocation remains pending. The complete default-script
PASS remains scoped to `293b1b6`; global 1:1 parity is not claimed.


## Gemini model selection precedes shape admission without repairing invalid JSON

The pinned `fallback/body.rs` reads a model through serde Value `as_str` and
filters blank strings. The runtime then substitutes the selected fallback model
before the chat-compatible adapter validates model shape. Thus a valid
non-string model is treated as absent and uses the default chain. That same
substitution first requires parsing the complete original serde Value; failed
parses return the original body unchanged, which the adapter rejects terminally.

Godex instead skipped model fallback for non-string values. Five executable
cases reached the provider with an empty model instead of the tagged default.
Malformed surrogate/UTF-8 strings had the opposite problem: Go replaced their
invalid text before model substitution and the gateway saw a valid new request.
The routing use case now defaults valid non-string models and checks whether
the whole original body is safe to rewrite using existing stateless
`helper/jsonutil` boundary and numeric-token checks. Invalid bodies reach the
existing concrete-gateway terminal admission unchanged. Model selection remains
in `usecase/routing`; protocol errors remain in `gateway/gemini`. Compact policy,
account selection and continuation ownership are unchanged. There is no new
helper API, package, interface, dependency, config, worker or persisted format.

Before repair six controls pass and nine tests fail, including two parents:
`rtk go test ./internal/usecase/routing -run '^TestProdex04384GeminiModel(TypesUseDefaultBeforeGateway|RewritePreservesInvalidSerdeInput)$' -count=1`;
`/tmp/gdx-04384-gemini-model-input-before.log`.
The same command passes all 15 tests after repair:
`/tmp/gdx-04384-gemini-model-input-focused.log`.
The regressions require one selected default-model invocation and prevent
malformed original Unicode/numbers from becoming eligible for model repair.

Eleven executable baselines pin clean
`0ad214cb731dae535f330e530accb2eb6c2ec028` and the unchanged reference:
`/tmp/gdx-04384-gemini-control-model-<suffix>-before-oracle.json`.
Null, empty/blank string and overflow controls PASS. Five non-string model
cases and two malformed-Unicode cases FAIL only Godex. All reference and
independent corruption invariants pass. The first ten use
`/tmp/gdx-04384-gemini-model-input-differential`; the added raw-UTF-8 case uses
`/tmp/gdx-04384-gemini-model-input-qualified-differential`.
The existing control-type fixture family now builds an original model member
without a duplicate key, checks the exact selected upstream model, and retains
status/body/type/stdout/exit, authenticated original request, attempts and safe
state. Invalid original serde must return plain 502, exit 2 and zero attempts.

`rtk go test ./tools/differential -count=1` passes 280 tests before and after
the raw-UTF-8 fixture addition:
`/tmp/gdx-04384-gemini-model-input-oracle-tests.log` and
`/tmp/gdx-04384-gemini-model-input-qualified-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,673 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-gemini-model-input-test-all.jsonl`.
`rtk go test -race ./internal/usecase/routing ./internal/gateway/gemini ./internal/helper/jsonutil ./tools/differential -count=1`
passes 3,053 tests: `/tmp/gdx-04384-gemini-model-input-race.log`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-gemini-model-input-verify.log`.
Clean-commit 185-case Gemini and 169-case DeepSeek matrices follow.
Other controls, combined-error ordering, Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain open.
Global 1:1 parity is not claimed.


## Validated ignored-sampling and committed-oracle checkpoint: 0ad214c

Clean `0ad214cb731dae535f330e530accb2eb6c2ec028` passes all 174 Gemini cases
and all 169 DeepSeek scenarios, including independent mutation checks:
`/tmp/gdx-04384-gemini-ignored-sampling-oracle.json` and
`/tmp/gdx-04384-gemini-ignored-sampling-deepseek-oracle.json`.
Both reports use the committed completion-time invariant repair, unchanged
published 0.438.4 reference and exact clean candidate. Earlier reference-state
and late-clock FAIL reports remain FAIL and are retained above.

The clean binaries were built with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-ignored-sampling-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-ignored-sampling-final-differential ./tools/differential`.
Commands use `--gemini`, or `--scenario all --scenario-workers 3`.
Commits `54dad09`, `0ad214c` and documentation-only `c74bdcc` were checked,
fast-forwarded through the clean primary worktree and pushed directly to
`origin/main`, without waiting for CI. No force push, tag, release or
editor-checkout change occurred.

The next clean model-input candidate `8d618e0` already passes all 169 DeepSeek
scenarios: `/tmp/gdx-04384-gemini-model-input-deepseek-oracle.json`.
Its targeted numeric-model, raw-UTF-8-model and null-model cases also PASS:
`/tmp/gdx-04384-gemini-control-model-<number|utf8|null>-targeted-oracle.json`.
Its full 185-case Gemini report is pending:
`/tmp/gdx-04384-gemini-model-input-oracle.json`.
A new full default gate is running from the separate protected clean
`8d618e097c8549015785bce1e23c89999198d42f` worktree:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-8d618e0.log 2>&1`.
That process is not advanced or modified while it executes. The completed
default-gate PASS remains scoped to `293b1b6` until this entire invocation
exits 0, including stages after its phase marker. Global parity is unproven.


## DeepSeek top-level control types before request construction

The actual pinned DeepSeek runtime first invokes
`runtime_deepseek_chat_request_body_with_options`, which uses the shared
`runtime_provider_chat_compatible_request_body` validation, even when the
simple-request conformance body is selected later. A present `stream` must be
a boolean, and present `instructions`/`previous_response_id` must be strings.
Reading only the later compiled simple-request projection misses that gate.

Godex defaulted wrong-type streams to false, ignored wrong-type instructions
and treated wrong-type previous IDs as absent. Each could create valid fresh
upstream work from an input the released runtime rejected. The concrete
DeepSeek gateway now checks those types at its existing Responses-object
admission point, after its input-size guard and before request construction.
Failures use the existing terminal preparation class and plain 502 mapping;
the routing use case retains ownership of retries and dispatch. The focused
private file owns only this protocol type invariant. No shared production
helper, package, exported API, interface, dependency, config, worker or
persisted format is added. Gemini production code is unchanged in this stage.

Before repair 34 regression controls pass and 27 tests fail, including parents:
`rtk go test ./internal/gateway/deepseek -run '^TestProdex(04384DeepSeekControlTypesAreCheckedBeforeTranslation|04380DeepSeekInvalidInputIsTerminalBeforeUpstream)$' -count=1`;
`/tmp/gdx-04384-deepseek-control-types-before.log`.
The same command passes all 61 tests after repair:
`/tmp/gdx-04384-deepseek-control-types-focused.log`.
The transport table proves wrong types never reach upstream and retain the
generic terminal presentation. Valid boolean/string controls remain accepted.

Nine executable baselines pin unchanged clean `8d618e0`, the unchanged
reference and `/tmp/gdx-04384-deepseek-control-types-differential`:
`/tmp/gdx-04384-deepseek-control-<suffix>-before-oracle.json`.
The stream-false control PASSes. All eight wrong-type cases FAIL only Godex;
each incorrectly produces a 200 response after one authenticated upstream
attempt, while the reference returns plain 502, exit 2 and zero attempts.
All reference and independent mutation invariants pass. The existing strict
primitive fixture family supplies the exact original request/response, status/
type/stdout/exit, retry, credential and safe-state checks. These cases are
registered only in the DeepSeek executable matrix; no Gemini cases are repeated.

`rtk go test ./tools/differential -count=1` passes 280 tests:
`/tmp/gdx-04384-deepseek-control-types-oracle-tests.log`.
`rtk go test -race ./internal/gateway/deepseek ./internal/usecase/routing ./tools/differential -count=1`
passes 2,894 tests: `/tmp/gdx-04384-deepseek-control-types-race.log`.
`rtk proxy go test -json ./...` passes 9,703 tests across 81 packages
(71 passing packages and ten with no tests):
`/tmp/gdx-04384-deepseek-control-types-test-all.jsonl`.
`rtk proxy make verify` passes the complete local gate:
`/tmp/gdx-04384-deepseek-control-types-verify.log`.
Clean-commit 178-case DeepSeek qualification follows. The separate 185-case
Gemini run remains scoped to unchanged Gemini source in clean `8d618e0`.
Other controls, combined-error ordering, complete DeepSeek JSON admission,
Gemini Compact, executable session/turn continuation, native macOS/Windows
and live authentication remain open. Global 1:1 parity is not claimed.


## Validated Gemini preparation checkpoint: a7fabe9

Clean `a7fabe987e50e30330599c534367323254db8f00` passes all 192 Gemini
cases and 213 DeepSeek scenarios, including independent mutation controls:
`/tmp/gdx-04384-gemini-preparation-errors-oracle.json` and
`/tmp/gdx-04384-gemini-preparation-errors-deepseek-oracle.json`.
The exact clean product and harness were built with
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-preparation-errors-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-preparation-errors-final-differential ./tools/differential`.
The matrices use `--gemini`, or `--scenario all --scenario-workers 3`, matching
clean source/commit pins and the unchanged published 0.438.4 reference.
After fetching and checking the clean primary worktree and remote, `a7fabe9`
was fast-forwarded and pushed directly to `origin/main` without waiting for CI.
The separate `4c270db` 213-case DeepSeek matrix also PASSes; its 199-case Gemini
matrix is still pending. The complete default gate remains scoped to `8d618e0`.
No force push, tag, release or editor-checkout change occurred.


## Gemini candidate-count admission precedes tool and shape validation

Pinned `local_rewrite_gemini.rs:31-41` and `local_rewrite_gemini_send.rs:147-158`
validate candidate-count aliases before tools, returning structured terminal
400 `invalid_request_error`. Complete serde admission runs first. The Mojo
`gemini_bridge_request_write_candidate_validation` compares raw fragments
from serialized serde Values: equivalent floats, decoded strings and ordered
objects compare equal, while integer 1 and float 1.0 remain distinct. Null
aliases are inactive and every active selected value must be integer 1.

The API-key adapter omitted this check. Its existing native validator also
compared only integer types and lexical number spellings, reporting an alias
conflict for canonical equal booleans, strings, arrays, objects and floats.
The Gemini gateway now reuses its private validator before tools and controls,
and compares aliases through the existing technical serde Value writer.
Routing and HTTP delivery recognize exactly the three local candidate-count
messages, preserving terminal status/body/headers without treating unrelated
upstream failures as local errors. The existing two-key real-gateway/router
test proves zero upstream attempts and no retry wait. No new production
helper, package, exported API, interface, dependency, configuration, worker or
persisted format is introduced. Claude and DeepSeek policies are unchanged.

The focused command below first FAILs with eight controls passing and 26
failures, including parents, then PASSes all 34 tests:
`rtk go test ./internal/gateway/gemini ./internal/usecase/routing ./internal/delivery/http/proxy -run '^TestProdex(04384Gemini(CandidateValidationPrecedesToolsAndControls|NativeCandidateAliasesCompareCanonicalSerdeValues|RequestAdmissionDoesNotRetryModelOrCredential)|04380(LocalGeminiToolValidationErrorMatchIsFailClosed|GeminiInvalidFunctionToolRejectionWireMatchesTaggedJSON))$' -count=1`.
Logs: `/tmp/gdx-04384-gemini-candidate-count-cross-before.log` and
`/tmp/gdx-04384-gemini-candidate-count-focused.log`.

Twenty-two executable baselines pin unchanged clean `4c270db`, the unchanged
reference and `/tmp/gdx-04384-gemini-candidate-count-before-differential`:
`/tmp/gdx-04384-gemini-candidate-<suffix>-before-oracle.json`.
Seventeen cases FAIL only Godex. Five controls PASS: null aliases, integer one,
null/one aliases, valid candidate plus invalid tools, and malformed Unicode.
All reference and independent mutation invariants pass. Commands use
`--gemini --scenario request-primitive-candidate-<suffix>` with the exact clean
source/commit pins. Fixtures retain original bodies, canonical upstream bytes,
exact 400/502 body/type/headers, stdout/exit, attempts and safe durable state.
The existing fixture error field is renamed to cover both tools and candidates;
all prior assertions and header/attempt/state mutations remain active.

`rtk go test ./tools/differential -count=1` passes 281 oracle tests:
`/tmp/gdx-04384-gemini-candidate-count-oracle-tests.log`.
`rtk go test ./internal/gateway/gemini -count=1` passes 894 tests:
`/tmp/gdx-04384-gemini-candidate-count-gateway-tests.log`.
`rtk proxy go test -json ./...` passes 9,792 tests in 71 packages; ten packages
have no tests and 18 optional installed-Codex/executable tests are skipped:
`/tmp/gdx-04384-gemini-candidate-count-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./internal/delivery/http/proxy ./tools/differential -count=1`
passes 3,348 tests: `/tmp/gdx-04384-gemini-candidate-count-race.log`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-candidate-count-verify.log`.
Clean-commit 221-case Gemini and 213-case DeepSeek qualification follows.
Native OAuth execution, other controls and combined errors, Gemini Compact,
executable session/turn continuation, native macOS/Windows and live login
remain unproven. Global 1:1 parity is not claimed.


## Retained reference restart mismatch on 4c270db

The first unchanged clean `4c270dbd273182ecb2803eb2126ee9a705a52f49`
199-case Gemini report FAILs exactly `raw-key-bound-response-restart-key`:
`/tmp/gdx-04384-gemini-tool-priority-oracle.json`.
The reference seed returns 200 with one original attempt and verified durable
ownership, but its changed-key restart unexpectedly returns 200 with a second
upstream attempt. Godex returns the expected terminal 503, exit 2 and no new
attempt. This is not an orphan-temp-file failure. The cause remains unexplained.
All other 198 cases pass, as does the separately pinned 213-case DeepSeek matrix.

The isolated unchanged restart rerun PASSes with both products returning
503, exit 2 and only the seed upstream attempt:
`/tmp/gdx-04384-gemini-tool-priority-restart-key-rerun-oracle.json`.
A full unchanged 199-case rerun is pending:
`/tmp/gdx-04384-gemini-tool-priority-rerun-oracle.json`.
No assertion, expected status, owner policy or state allowlist was changed.
The original full report remains FAIL and this candidate is withheld from
`origin/main` pending complete executable qualification.


## Gemini top-level beta completions cannot become Responses work

Pinned `runtime_provider_chat_compatible_request_body` rejects beta completion
fields before shared shape/history conversion. Mojo `deepseek_beta_fields_plan`
rejects the presence of root `prefix`, `suffix` or `prompt`, regardless of the
value. The tagged local pipeline presents this preparation failure as plain
terminal 502 with no provider attempt. Earlier Gemini candidate/tool validation
still has its structured 400 precedence, after complete serde admission.

Godex silently discarded these root fields and executed an ordinary Responses
request. The Gemini gateway's existing private control-admission function now
rejects all three present fields before conversion. The entire production
change is five lines; no shared helper, package, interface, dependency,
configuration, worker, exported API or durable-state format is added. The
existing generic terminal preparation error keeps model/credential routing
from replaying or waiting. Unrelated nested metadata remains ignored normally.
DeepSeek and native Gemini code retain their existing separate policies.
Input-item beta fields and other input-shape decisions remain a separate audit.

Before repair six controls pass and 17 tests fail, including parents:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(RejectsPresentBetaCompletionFields|RequestAdmissionDoesNotRetryModelOrCredential)$' -count=1`;
`/tmp/gdx-04384-gemini-beta-admission-cross-before.log`.
The same command passes all 23 tests after repair:
`/tmp/gdx-04384-gemini-beta-admission-focused.log`.
The two-key real gateway/router regression verifies zero upstream attempts and
no retry wait for all three fields; malformed JSON and candidate-count controls
remain in the same table.

Eleven executable baselines pin clean
`5f49a5ff86b6399fadb65ddb18ba649097ffe6b0`, the unchanged published reference
and `/tmp/gdx-04384-gemini-beta-admission-before-differential`:
`/tmp/gdx-04384-gemini-beta-<suffix>-before-oracle.json`.
Seven present-field cases FAIL only Godex, which returns 200 after one upstream
attempt instead of plain 502/exit 2/zero attempts. Four controls PASS: nested
metadata, earlier malformed tools, earlier invalid candidate and invalid
Unicode. All reference and independent mutation invariants pass. Commands use
`--gemini --scenario request-primitive-beta-<suffix>` and exact clean source/
commit pins. Existing exact request/response/status/header/stdout/exit/attempt/
state assertions and independent matching-corruption mutations cover the cases.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-gemini-beta-admission-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,808 tests across 71 packages; ten
packages have no tests and 18 optional installed-Codex/executable tests skip:
`/tmp/gdx-04384-gemini-beta-admission-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,021 tests: `/tmp/gdx-04384-gemini-beta-admission-race.log`.
`rtk proxy make verify` PASSes the full local gate:
`/tmp/gdx-04384-gemini-beta-admission-verify.log`.
Clean-commit 232-case Gemini and 213-case DeepSeek matrices follow. Protected
`5f49a5f` separately runs its 221-case Gemini/213-case DeepSeek qualification
and the entire default gate from `/tmp/gdx-04384-release-5f49a5f-work`:
`rtk proxy scripts/verify-release-parity.sh > /tmp/gdx-04384-release-5f49a5f.log 2>&1`.
Its four exact clean targeted candidate null/equal-floats/tools/controls reports
PASS: `/tmp/gdx-04384-gemini-candidate-<suffix>-targeted-oracle.json`.
The complete default-gate PASS remains scoped to `8d618e0` until every newer
stage exits successfully, including the final HTTP stages after the phase
marker. Other input/response controls, Gemini Compact, executable session/turn
continuation, native macOS/Windows and live authentication remain unproven.
Global 1:1 parity is not claimed.


## Gemini input arrays cannot discard invalid entries or beta prefixes

Pinned `deepseek_provider_core_messages_from_responses_request` rejects every
nonobject array entry. Before examining an object's type, the compiled
`deepseek_input_item_validation` rejects a present `prefix`, including null or
false. Gemini API-key translation uses these same validators. Later preparation
errors become plain terminal 502 before any upstream attempt; candidate/tool
errors keep their earlier structured 400 boundary.

Godex's shared Chat converter silently discarded nonobject array entries and
ignored item prefixes. A malformed history could therefore execute using only
the remaining valid message. The Gemini adapter's existing item-normalization
loop now rejects either condition before adding implicit message types. This
is a two-line replacement in the owning concrete gateway. No shared converter
policy, helper, interface, package, dependency, configuration, worker, exported
API or persisted format changes. Ordinary message arrays, item suffix/prompt
fields and nested metadata retain their separate accepted behavior.

The focused command below first FAILs with nine controls passing and 14
failures, including parents, then PASSes all 23 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(InputItemsCannotBeDiscardedOrLosePrefix|RequestAdmissionDoesNotRetryModelOrCredential)$' -count=1`.
Logs: `/tmp/gdx-04384-gemini-input-items-before.log` and
`/tmp/gdx-04384-gemini-input-items-focused.log`.
Existing two-key gateway/router coverage now includes mixed number/message
arrays and null item prefixes, proving zero upstream work and no retry wait.

Fifteen executable baselines pin clean
`a82be84bb77f124ac2f02fa3fc01a43c1be13918`, the unchanged reference and
`/tmp/gdx-04384-gemini-input-items-before-differential`:
`/tmp/gdx-04384-gemini-input-item-<suffix>-before-oracle.json`.
Ten invalid-item cases FAIL only Godex. Five controls PASS: ordinary arrays,
ignored item metadata, earlier tools, earlier candidates and invalid Unicode.
All reference and independent mutation invariants pass. Commands use
`--gemini --scenario request-primitive-input-item-<suffix>` with the exact clean
source/commit pins. Fixtures retain original arrays, original canonical
upstream bytes, exact status/body/type/headers/stdout/exit, attempt counts and
safe durable state; existing independent matching-corruption checks stay active.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-gemini-input-items-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,821 tests in 71 packages; ten packages
have no tests and 18 optional installed-Codex/executable tests are skipped:
`/tmp/gdx-04384-gemini-input-items-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,034 tests: `/tmp/gdx-04384-gemini-input-items-race.log`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-input-items-verify.log`.
Clean-commit 247-case Gemini and 213-case DeepSeek qualification follows.
Protected `5f49a5f` now passes all 213 DeepSeek scenarios with independent
mutations: `/tmp/gdx-04384-gemini-candidate-count-deepseek-oracle.json`.
Its Gemini matrix and complete default gate, the unchanged `4c270db` Gemini
rerun and clean `a82be84` matrices remain pending. The first `4c270db` reference
restart mismatch remains a retained FAIL. Main remains published at `a7fabe9`.
Unsupported item kinds and per-item fields/content, other controls, Gemini
Compact, executable session/turn continuation, native macOS/Windows and live
authentication remain open. Global 1:1 parity is not claimed.


## Gemini explicit input-item types cannot fall through to loose text

Pinned `deepseek_input_item_validation` permits explicit `message`, function/
custom/MCP/shell calls and the corresponding output kinds. Every other present
type, including null, wrong-type values, input/output text, image and reasoning,
returns an input-item error. The API-key runtime translates it to terminal plain
502 before upstream work. Absent types retain their separate implicit-message
path; earlier candidate/tool validation still returns structured 400.

Godex skipped validation whenever `type` was present. Its shared converter could
then turn an unsupported item's content into an ordinary user message. The
concrete Gemini adapter now checks explicit kinds in its existing item loop,
keeping the shared Claude converter and native Gemini translator independent.
The small local switch uses the pinned supported set; no helper, interface,
package, dependency, worker, configuration, exported API or durable-state format
is introduced. Existing multi-turn and supported tool conversion stay covered
by the complete matrix rather than a new speculative abstraction.

The focused command below first FAILs with 11 controls passing and 15 failures,
including parents, then PASSes all 26 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(RejectsUnsupportedExplicitInputTypes|RequestAdmissionDoesNotRetryModelOrCredential)$' -count=1`.
Logs: `/tmp/gdx-04384-gemini-input-kinds-before.log` and
`/tmp/gdx-04384-gemini-input-kinds-focused.log`.
The real two-key gateway/router table now proves unsupported/null kinds have
zero upstream attempts and no retry wait.

Sixteen executable baselines pin clean
`f4a277da9fd00ac5d45dfd570875e793cd7114d7`, the unchanged reference and
`/tmp/gdx-04384-gemini-input-kinds-before-differential`:
`/tmp/gdx-04384-gemini-input-kind-<suffix>-before-oracle.json`.
Ten cases FAIL only Godex, which executes ordinary work instead of rejecting it.
Six controls PASS: explicit/implicit messages, existing rejected images,
earlier tools/candidates and malformed Unicode. Every reference and independent
mutation invariant passes. Commands use
`--gemini --scenario request-primitive-input-kind-<suffix>` and exact clean
source/commit pins. Fixtures retain original JSON, canonical upstream bytes,
exact response/status/headers/stdout/exit, attempts and safe durable state.
No matching-corruption or zero-attempt assertion is removed or relaxed.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-gemini-input-kinds-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,835 tests in 71 packages; ten packages
have no tests and 18 optional installed-Codex/executable tests are skipped:
`/tmp/gdx-04384-gemini-input-kinds-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,048 tests: `/tmp/gdx-04384-gemini-input-kinds-race.log`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-input-kinds-verify.log`.
Clean-commit 263-case Gemini and 213-case DeepSeek qualification follows.
Protected `a82be84bb77f124ac2f02fa3fc01a43c1be13918` now passes all 213
DeepSeek cases and independent mutations:
`/tmp/gdx-04384-gemini-beta-admission-deepseek-oracle.json`.
Its Gemini matrix, the protected input-item matrices, `5f49a5f` Gemini/default
gates and unchanged `4c270db` Gemini rerun remain pending. The original
reference changed-key restart report remains FAIL. Main remains at `a7fabe9`.
Message roles/content, call identities and other per-item fields, remaining
request/response controls, Gemini Compact, executable session/turn continuation,
native macOS/Windows and live authentication remain open. Global 1:1 parity
is not claimed.


## Validated candidate/tool admission checkpoint: 5f49a5f

Clean `5f49a5ff86b6399fadb65ddb18ba649097ffe6b0` passes all 221 Gemini
cases and 213 DeepSeek scenarios, including independent mutation controls:
`/tmp/gdx-04384-gemini-candidate-count-oracle.json` and
`/tmp/gdx-04384-gemini-candidate-count-deepseek-oracle.json`.
Its clean product/harness builds use
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-candidate-count-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-candidate-count-final-differential ./tools/differential`.
Matrices use `--gemini`, or `--scenario all --scenario-workers 3`, with exact
clean source/commit pins and the unchanged published 0.438.4 reference.
This larger Gemini matrix includes every scenario from the earlier 199-case
`4c270db` matrix, plus 22 candidate-count cases. After checking this inclusion,
fetching and checking the clean primary/remote, the two small commits were
fast-forwarded together and pushed directly to `origin/main` without CI wait.
Main is `5f49a5f`. No force push, tag, release or editor-checkout edit occurred.

Both older `4c270db` full Gemini reports remain FAIL. The unchanged 199-case
rerun has a different reference failure on endpoint restart: its successful
seed leaves `state/state.json.last-good.2472424.1792654026233650692.7.tmp`,
which the unchanged strict state auditor rejects. Both final restart responses
are the expected 503, exit 2, with only the original seed upstream attempt.
Report: `/tmp/gdx-04384-gemini-tool-priority-rerun-oracle.json`.
The first changed-key reference replay failure remains unexplained. No state
allowlist, owner/status expectation, header requirement or mutation is relaxed;
the later complete PASS is scoped to `5f49a5f`, not retroactively to `4c270db`.

Clean `f4a277da9fd00ac5d45dfd570875e793cd7114d7` and
`105d3fde3505f5d0a93ceb2ef24d53099735c86a` separately pass all 213 DeepSeek
scenarios and independent mutations:
`/tmp/gdx-04384-gemini-input-items-deepseek-oracle.json` and
`/tmp/gdx-04384-gemini-input-kinds-deepseek-oracle.json`.
Their Gemini matrices remain pending, alongside `a82be84` Gemini and the
entire default `5f49a5f` gate. The last completed default gate remains `8d618e0`.


## Gemini explicit message roles cannot default malformed input to user

Pinned `deepseek_input_item_validation` checks a present role on an explicit
message as exactly user, assistant, system or developer. Omitted role is valid.
Null, wrong-type, empty, unknown, tool and padded labels are rejected. The
API-key runtime presents plain terminal 502 with no provider attempt; its
candidate/tool gate still runs earlier and preserves structured 400 errors.

Godex's shared Chat converter defaulted non-string/empty roles to user and
forwarded arbitrary strings. Gemini now checks its explicit-message role before
that shared conversion. Existing item admission/implicit-type normalization
moves into one private `request_input_items_04384.go` function in the same
concrete gateway. This names a real protocol boundary, shortens the parent
normalizer and keeps the production file below the normal 250-line target.
It has one caller, no I/O or background work, and does not qualify as a generic
shared helper. Native Gemini and shared Claude behavior remain independent.
No interface, package, dependency, configuration, exported API or persisted
format is added. Existing item-prefix/type rules retain their rejection paths.

The focused command below first FAILs with 13 controls passing and 13 failures,
including parents, then PASSes all 26 tests:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(ExplicitMessageRolesRequireTaggedValues|RequestAdmissionDoesNotRetryModelOrCredential)$' -count=1`.
Logs: `/tmp/gdx-04384-gemini-message-roles-before.log` and
`/tmp/gdx-04384-gemini-message-roles-focused.log`.
The real two-key gateway/router table proves null/tool roles have zero upstream
attempts and no retry wait. Unit controls retain all four supported roles.

Fifteen executable baselines pin clean
`105d3fde3505f5d0a93ceb2ef24d53099735c86a`, the unchanged reference and
`/tmp/gdx-04384-gemini-message-roles-before-differential`:
`/tmp/gdx-04384-gemini-message-role-<suffix>-before-oracle.json`.
Nine invalid-role cases FAIL only Godex, which executes an ordinary request.
Six controls PASS: user, omitted role, escaped user, earlier tools/candidates
and malformed Unicode. All reference and independent mutation invariants pass.
Commands use `--gemini --scenario request-primitive-message-role-<suffix>` with
exact clean source/commit pins. Original JSON, exact canonical upstream bytes,
response/status/headers/stdout/exit, attempts and safe durable state remain
covered by the existing independent matching-corruption checks.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-gemini-message-roles-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,847 tests in 71 packages; ten packages
have no tests and 18 optional installed-Codex/executable tests are skipped:
`/tmp/gdx-04384-gemini-message-roles-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,060 tests: `/tmp/gdx-04384-gemini-message-roles-race.log`.
`rtk proxy make verify` PASSes the full local gate:
`/tmp/gdx-04384-gemini-message-roles-verify.log`.
Clean-commit 278-case Gemini and 213-case DeepSeek qualification follows.
Previously started protected matrices and the entire `5f49a5f` default gate
remain pending; no earlier reference FAIL is relabeled. Further message content,
call IDs/names/output validation, instructions-only behavior and remaining
request/response controls still require source audit and executable proof.
Gemini Compact, executable session/turn continuation, native macOS/Windows and
live authentication remain unproven. Global 1:1 parity is not claimed.


## Validated top-level beta admission checkpoint: a82be84

Clean `a82be84bb77f124ac2f02fa3fc01a43c1be13918` passes all 232 Gemini
cases and 213 DeepSeek scenarios with independent mutation controls:
`/tmp/gdx-04384-gemini-beta-admission-oracle.json` and
`/tmp/gdx-04384-gemini-beta-admission-deepseek-oracle.json`.
Clean builds use
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-beta-admission-godex ./cmd/godex` and
`rtk go build -trimpath -o /tmp/gdx-04384-gemini-beta-admission-final-differential ./tools/differential`.
The commands use `--gemini`, or `--scenario all --scenario-workers 3`, exact
clean source/commit pins and the unchanged published reference. After fetching
and checking the clean primary and remote, `a82be84` was fast-forwarded and
pushed directly to `origin/main` without waiting for CI. Main is `a82be84`.
No force push, tag, release or editor-checkout edit occurred.
The protected `5f49a5f` full default gate has passed its Gemini and WebSocket
continuation stages but is still running; its complete PASS is not claimed.


## Gemini invalid message-content roots cannot disappear during conversion

Pinned `deepseek_input_item_content_tag` accepts a present explicit-message
content root only as string, object or array. Null, boolean and numeric values
return an input-shape error, presented by the API-key pipeline as terminal
plain 502 before upstream dispatch. This is separate from content-part rules.

Godex's shared converter silently discarded such a message and could execute
using the valid message that followed it. The existing private Gemini input-item
admission now checks the content root before shared conversion. The change is
one local type switch; no helper, package, interface, dependency, worker,
configuration, exported API or persisted format is added. The native Gemini
translator and shared Claude policy stay independent. Earlier candidate/tool
structured 400 errors and complete serde admission keep their precedence.
String and text-part-array controls retain exact canonical upstream bytes.

Before repair 15 controls pass and nine tests fail, including parents:
`rtk go test ./internal/gateway/gemini -run '^TestProdex04384Gemini(ExplicitMessageContentRequiresSupportedShape|RequestAdmissionDoesNotRetryModelOrCredential)$' -count=1`;
`/tmp/gdx-04384-gemini-content-types-before.log`.
The same command passes all 24 tests after repair:
`/tmp/gdx-04384-gemini-content-types-focused.log`.
The real two-key gateway/router table proves mixed invalid/valid content never
creates an upstream attempt or retry wait.

Eight executable baselines pin clean
`0830ef5b92c001e36bae6e3a223cbfdd8ddba76a`, the unchanged reference and
`/tmp/gdx-04384-gemini-content-types-before-differential`:
`/tmp/gdx-04384-gemini-content-<suffix>-before-oracle.json`.
The null, boolean and number cases FAIL only Godex, which executes the remaining
message. Five controls PASS: string/array content, earlier tools/candidates and
invalid Unicode. Every reference and independent mutation invariant passes.
Commands use `--gemini --scenario request-primitive-message-content-<suffix>`
with exact clean source/commit pins. Original JSON, canonical upstream bytes,
exact response/status/headers/stdout/exit, attempt counts and safe durable state
retain the existing independent matching-corruption checks.

`rtk go test ./tools/differential -count=1` passes 281 tests:
`/tmp/gdx-04384-gemini-content-types-oracle-tests.log`.
`rtk proxy go test -json ./...` passes 9,855 tests in 71 packages; ten packages
have no tests and 18 optional installed-Codex/executable tests are skipped:
`/tmp/gdx-04384-gemini-content-types-test-all.jsonl`.
`rtk go test -race ./internal/gateway/gemini ./internal/usecase/routing ./tools/differential -count=1`
passes 3,068 tests: `/tmp/gdx-04384-gemini-content-types-race.log`.
`rtk proxy make verify` PASSes the complete local gate:
`/tmp/gdx-04384-gemini-content-types-verify.log`.
Clean-commit 286-case Gemini and 213-case DeepSeek qualification follows.
Earlier input-item/kind/role Gemini matrices and the entire default `5f49a5f`
gate remain pending. Main remains the qualified published `a82be84`; older
reference failures remain FAIL. Content-part validation/conversion, call IDs/
names/output, instructions-only input and remaining request/response controls
still require audit and executable proof. Gemini Compact, executable session/
turn continuation, native macOS/Windows and live authentication remain unproven.
Global 1:1 parity is not claimed.
