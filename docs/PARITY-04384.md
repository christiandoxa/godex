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
`1.7976931348623158e308` that the pinned serde default rejects. Independent
offline tests using the release's Rust `serde 1.0.228` and `serde_json
1.0.151` dependencies confirmed the typed duplicate, Unicode, recursive
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
`serde_json 1.0.151` canonical value representation. Godex previously used
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
