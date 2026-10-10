# Prodex 0.437.1 behavioral parity checkpoint

Canonical Prodex tag: 0.437.1
Source commit: 98918c32e0398990fccf45901d94da0c545d0810
Official Linux x86_64 executable SHA-256:
171482e7ce38ebfd04b5efa564d3d118c7f542b27f30065fa29dd737d18d9d88

Both the active DeepSeek differential harness and managed-profile lifecycle
oracle now require the exact tagged source and version; the latter also
checks the public executable SHA-256. The release-gating shell script checks
the same binary and tagged source before it builds the clean candidate.

## Differences introduced since 0.437.0

The canonical release notes describe no new CLI surface and two relevant
Responses precommit recovery changes.

1. **Headerless SSE inspection.** When the original Responses request
   explicitly sets stream:true, an upstream HTTP 200 without Content-Type
   (or with a blank value) must still enter the SSE precommit inspector.
   The classifier may rotate after a precommit server_is_overloaded event,
   but must not invent a MIME header or replay output that was already
   committed. An explicit application/json response remains unary.
   The Go router propagates stream intent through both initial and
   response-recovery sends, and its HTTP delivery path preserves the
   missing Content-Type and Date instead of inventing them. Positive
   and negative tests exercise real Go HTTP wire framing and full-router
   precommit rotation.

2. **Bounded same-owner turn-state recovery.** A Responses HTTP request
   with a valid, pinned x-codex-turn-state and no previous_response_id
   may retry the same account after an uncommitted response.failed
   server_is_overloaded event. The new Go policy reproduces the tagged
   Mojo delay planner: five retries maximum, a 60-second planning window,
   250/500/1000/2000/4000ms exponential bases plus request-ID modulo
   251ms jitter, millisecond-rounded upstream Retry-After precedence,
   no shortening of long retry advice to force an early attempt, and
   no same-owner retry for previously committed streams or previous-ID
   continuation repair. An explicit upstream text/event-stream MIME takes
   precedence even if the original request sets stream:false: the response
   has already been identified as an SSE precommit overload, so the bound
   retry must not depend on a redundant request flag. This was reproduced
   as a failing-before test and fixed. Conversely, explicit JSON/text
   or headerless unary responses are not reclassified and retried. Admission
   slots are released before each wait,
   reacquired for the same owner, and verified to have no underflow or
   leaks after recovery or exhaustion. A real six-attempt negative-control
   test verifies that the original upstream failure remains observable
   once the retry budget is exhausted.

The Rust/Mojo architectural reorganization in this patch is not by itself
an independent requirement to reimplement the same internal modules in Go.

## Verification evidence and limitations

- Exact source-to-code tests cover missing/blank/explicit MIME, gzip,
  unary vs streaming requests, precommit overload, preserved output,
  same-owner retry, retry exhaustion, cancellation, long Retry-After,
  60-second deadline, previous-response precedence and admission slots.
- The pinned official Prodex binary is also used to rerun all named
  DeepSeek synthetic provider scenarios and the independent managed and
  external profile lifecycle test, from a clean committed Godex candidate.
- The retained Codex 0.162.0 app-server qualification tests still use a
  SHA-256-verified upstream binary and an isolated credential-free home.
- The bound retry tests also verify source-header Retry-After precedence
  over embedded error-message/header advice, even when upstream advice
  exceeds the available 60-second deadline. Three shuffled race-test
  iterations of the routing and HTTP proxy packages passed.
- **OpenAI API-key gateway startup parity:** A synthetic API-key profile is
  intentionally a direct native Codex launch, not a Godex HTTP gateway
  profile. With that profile active, both the pinned Prodex binary and Godex
  reject `gateway` before opening a listener with `gateway provider does not
  expose an OpenAI-compatible proxy`; neither sends a request to the fake
  upstream. ChatGPT-authenticated profiles continue to use the native OpenAI
  gateway transport, and their request forwarding is covered by the isolated
  fixture evidence above.
- **Durable state validation and process leases:** Routing snapshots reject
  negative binding timestamps, and persisted route-health updates preserve a
  newer or stronger record when an older process reports late. On Linux,
  runtime broker cleanup treats a zombie PID as stale, matching the tagged
  process-absence proof while retaining live leases. Focused regressions and
  race tests cover these state and lifecycle boundaries.
- **Startup quota warmup:** The managed-profile lifecycle oracle now covers a
  successful usage probe and an upstream 503. Each binary performs exactly one
  authenticated `GET /backend-api/wham/usage` request at startup, including
  the synthetic bearer/account identity and the Codex quota compatibility
  headers; success persists a valid generation-one usage snapshot, while
  failure persists none. The warmup uses one synchronous probe and bounded
  queued probes, with profile homes and base-URL/no-proxy policy preserved. The
  oracle waits for the durable state before comparing the two implementations
  and rejects a matching pair that sends the wrong request contract.
- **Completed-body half-close forwarding:** A raw TCP client that sends a
  complete `POST /responses` body and then half-closes its write side is
  forwarded by both binaries. Before the fix, Godex treated the server request
  context cancellation as a failed request and returned an empty 200 without
  contacting the upstream; the fixed proxy captures the complete body before
  detaching that transport cancellation. Incomplete bodies still fail before
  dispatch, and the focused proxy regression covers both paths.
- **Provider endpoint identity:** Continuation routing identities now include a
  configured provider API URL. A persisted continuation cannot silently move
  to a different upstream after endpoint migration, while an unchanged
  endpoint retains a stable identity. The runtime and profile-catalog tests
  cover the migration and stability cases.
- **App-server argument encoding:** Companion app-server launch arguments now
  encode control characters as valid TOML escapes. The regression previously
  failed on a model containing BEL because `strconv.Quote` emitted an invalid
  `\\a` escape; the corrected serializer is qualified against the official
  Codex 0.162.0 app-server binary.
- **App-server turn identity validation:** Super app-server inspection now
  rejects empty, oversized, or Unicode-control turn IDs before sending
  `turn/interrupt`. A synthetic Unix app-server regression reproduces the
  tagged Codex boundary for a 129-byte ID and a C1 control character, and the
  credential-free official Codex 0.162.0 thread-index qualification remains
  passing.
- **Optional-tool probe cancellation:** Super's bounded `node`/`npx` probes
  run in an isolated process group and terminate descendants on timeout or
  cancellation. This prevents inherited output pipes from keeping a canceled
  probe blocked; a descendant-pipe regression and the normal-PATH differential
  suite both complete without the former probe hang.
- **Not yet certified:** raw durable-file layouts differ, and broader
  multi-profile recovery, OAuth/user auth, queue lifecycle, TUI, live
  goal monitoring, WebSocket transport and all provider integrations
  do not yet have complete cross-binary behavioral evidence.

Therefore a passing Go test suite or isolated fixture does **not** prove
full feature-for-feature parity. The v0.2.1 release gate remains
fail-closed on unverified material behavior.


## CLI provider-preset validation

The exact 0.437.1 binary rejects gateway --provider openai with argument
exit status 2, since openai is the native route and is not an external
provider preset. Godex previously returned status 1. It now uses a typed
argument-usage error and reports the same exit status without swallowing
the diagnostic. The independent profile lifecycle oracle gained a
21st CLI stage that checks both binary exit codes and unchanged profile
state. This does not imply gateway request processing is fully equivalent.


### Expanded gateway syntax differential

A command-line matrix against Prodex 0.437.1 identified eight more
argument-validation cases that previously returned runtime exit code 1
on Godex but parser exit code 2 on Prodex: missing values, unexpected
options/positional args, and mutually exclusive Presidio toggles.
Godex's gateway argument parser now wraps these as typed CLI usage
errors without changing actual runtime startup failures. The
credential-free cross-binary lifecycle oracle exercises all cases
as separate subprocesses, expanding its coverage to 29 steps.
