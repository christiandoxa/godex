# Prodex 0.438.2 continuation and WebSocket executable differential

This harness compares an official SHA-256-pinned Prodex `0.438.2` binary
and Godex using isolated two-profile homes, synthetic credentials, and a
loopback-only upstream. It does **not** send real model requests or use
the caller's account credentials.

The default mode checks HTTP Responses continuation ownership, turn-state
affinity, overload/quota and full-context recovery, already-committed
streaming output, cancellation, and process restart. Historical status
and retry differences may remain within this bounded HTTP observation;
the fixture is not a full feature-parity certificate.

## Exact HTTP history replay boundary

The opt-in `TestProdex04380ExecutableHistoryReplayBoundary` test verifies nine
request-history shapes against the pinned executables. Each case creates its
response owner with a real successful HTTP seed request before the upstream
rejects the previous ID. Only exact compaction/completed-turn/function-call
histories allow a second same-owner attempt; padded tags and
`compaction_trigger` retain the original 400. Recovery removes only the previous
ID and preserves context-window metadata and all other request fields.

`scripts/verify-release-parity.sh --phase=preflight-oracles` supplies all five
`GODEX_TEST_HISTORY_*` executable/source/commit settings and runs the test.
Ordinary tests skip this executable fixture when paths are absent. Offline
negative controls reject missing seeds, extra attempts, changed owners,
false success, changed errors, retained previous IDs, and lost context.

## Strict WebSocket differential

Use `--websocket` to exercise **nineteen** request paths against both
executables (including three paths on a freshly restarted gateway). The
original thirteen owner/continuation paths are listed first:

1. Seed a soft `session_id` preference and record its actual selected owner.
2. `response.created` and `keepalive` followed by precommit overload:
   the failed prelude must be discarded before the successful retry.
   Prodex retries the *same* first profile in this deterministic fixture.
3. A heartbeat and visible output followed by overload must not trigger a
   second upstream attempt or duplicate model execution.
4. A verified `previous_response_id` must outrank a conflicting soft session.
5. A verified `x-codex-turn-state` must also outrank the soft session.
6. An *unbound* `previous_response_id` may discover an upstream profile.
7. An *unbound* `turn_state` may likewise discover a profile.
8. An *unbound* missing previous-response chain containing a completed
   tool call must make four bounded **same-owner** attempts while preserving
   every tool field, then surface `error / 409 / stale_continuation`.
9. A *verified bound* previous-response chain with a missing completed tool
   result must follow the same bounded retry and generic error frame contract.
   Ordinary bound continuations without tool output retain their original
   response envelope.
10. A soft-session-only request after hard-affinity activity retains the
    pinned reference's observed owner selection.
11–13. Restart the gateway process and repeat hard previous-response,
   hard turn-state, and soft-session-only requests. No local socket or
   in-memory router cache survives the restart.

The four additional tagged **0.438.2 terminal-status** cases require an
`error` with a present but invalid primary `status` to remain nonterminal
when secondary `status_code` is 400. The invalid values are the string
`"invalid"`, a fractional numeric `400.5`, and overflowing u16 `65536`;
all three must forward the exact following `response.completed` frame with
**one** upstream attempt. Conversely, `status:65535` is a valid terminal
error and the intentionally delayed second completion may not be forwarded.
Every case checks exact frame bodies, strict same-owner count and negative
controls against status truncation, premature close, replay or leaked
post-terminal output. The original tagged-status gate observed **50** synthetic WebSocket
upstream attempts across both binaries. Two more cases now verify that a
**fresh** connection-limit error in either `code` or nested `error.code`
triggers exactly one new WebSocket attempt against the **same** profile.
The mixed top-level/nested code precedence must not leak the precommit error,
rotate owner, duplicate the user request or replay any committed output.
The expanded gate observes **58** upstream attempts across both binaries;
negative controls reject wrong retry count, owner, response ID or wire frame.

The pinned Prodex executable selected account-a on all these synthetic
requests; **this fixture does not prove that soft session IDs always select
account-a**, or that a hard previous-response owner may be discarded.
The harness now rejects mismatched selected owners even if attempt counts
and client event types still match.

Run from a **clean committed Godex checkout**, with a locally built candidate
containing Go VCS metadata:

```sh
go build -trimpath -o /tmp/godex-043710-candidate ./cmd/godex
go run ./tools/continuationparity \
  --prodex /path/to/official/prodex-0.438.2 \
  --prodex-source /path/to/clean/prodex-tag-0.438.2 \
  --godex /tmp/godex-043710-candidate \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)" \
  --websocket
```

The official reference executable must report `prodex 0.438.2`, have
SHA-256 `3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f`,
and correspond to source commit
`decfed45f884fe2bef20911303beab7b198698b7`.
The WebSocket mode additionally rejects a dirty Godex source tree, a
different expected commit, or missing/modified Go executable VCS metadata.

The harness fails on mismatched event order, missing visible output,
extra upstream attempts, leaked precommit response IDs, wrong hard-affinity
owner, missing-chain error type/code/status, rewritten tool outputs,
and invalid framing. Unit negative controls intentionally mutate these
observations. See `scripts/verify-release-parity.sh` for the exact-tagged
release-only invocation. That release script still blocks on *all* other
oracle checks; local scenario passes do not authorize a tag or release.


## Tagged Prodex 0.438.2 completed-idle WebSocket permit oracle

The `--websocket-capacity` flag is a separate published SHA/source-tag
verified two-executable fixture, not a substitute for the original 58-attempt
WebSocket continuation differential. Both products receive identical isolated
synthetic profiles and a **weighted hard per-profile execution limit of 2**.
Five distinct local client WebSocket connections each complete a native
Responses turn and then **remain open**; the fixture checks one authenticated
upstream dispatch per turn on `account-a`. A sixth turn on the **first**
client WebSocket must reuse the previously cached upstream transport.
Expected per product: exactly **6 completed turns**, **6 upstream requests**,
**5 upstream handshakes**, no client re-connect, no duplicate dispatch, and
no owner or request-body changes. Keeping five idle connections open makes a
leaked execution permit observable instead of accidentally masking it by
closing every completed client session. Reuse when capacity is again ready
must not throw away session lineage or create a new upstream handshake.

The Go package's `TestProdex04382WebSocketCapacityOracleRejectsMaterialMutations`
requires the fixture to detect lost/extra dispatch, changed owner or model
request bytes, missing/mutated completion IDs, and a sixth handshake. No
real OAuth access or model API traffic is used. The independent test of
**saturated active-turn reuse waiting** remains in Godex's real Router/TCP
integration suite; this cross-executable fixture specifically proves the
completed **idle** resource release/reuse lifecycle.
