# Prodex 0.437.11 continuation and WebSocket executable differential

This harness compares an official SHA-256-pinned Prodex `0.437.11` binary
and Godex using isolated two-profile homes, synthetic credentials, and a
loopback-only upstream. It does **not** send real model requests or use
the caller's account credentials.

The default mode checks HTTP Responses continuation ownership, turn-state
affinity, overload/quota and full-context recovery, already-committed
streaming output, cancellation, and process restart. Historical status
and retry differences may remain within this bounded HTTP observation;
the fixture is not a full feature-parity certificate.

## Strict WebSocket differential

Use `--websocket` to exercise **thirteen** request paths against both
executables (including three paths on a freshly restarted gateway):

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
  --prodex /path/to/official/prodex-0.437.11 \
  --prodex-source /path/to/clean/prodex-tag-0.437.11 \
  --godex /tmp/godex-043710-candidate \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)" \
  --websocket
```

The official reference executable must report `prodex 0.437.11`, have
SHA-256 `634f67882045afc4cc74b3046b784b33618ff8e14a1009fd9d1bc44d699746fa`,
and correspond to source commit
`83ae4cb18ca8000de7cb563291539392c71444e0`.
The WebSocket mode additionally rejects a dirty Godex source tree, a
different expected commit, or missing/modified Go executable VCS metadata.

The harness fails on mismatched event order, missing visible output,
extra upstream attempts, leaked precommit response IDs, wrong hard-affinity
owner, missing-chain error type/code/status, rewritten tool outputs,
and invalid framing. Unit negative controls intentionally mutate these
observations. See `scripts/verify-release-parity.sh` for the exact-tagged
release-only invocation. That release script still blocks on *all* other
oracle checks; local scenario passes do not authorize a tag or release.
