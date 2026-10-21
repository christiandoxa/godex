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
