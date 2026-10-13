# Prodex 0.437.9 continuation and WebSocket executable differential

This harness compares an official SHA-256-pinned Prodex `0.437.9` binary
and Godex using isolated two-profile homes, synthetic credentials, and a
loopback-only upstream. It does **not** send real model requests or use
the caller's account credentials.

The default mode checks HTTP Responses continuation ownership, turn-state
affinity, overload/quota and full-context recovery, already-committed
streaming output, cancellation, and process restart. Historical status
and retry differences may remain within this bounded HTTP observation;
the fixture is not a full feature-parity certificate.

## Strict WebSocket differential

Use `--websocket` to exercise four request/response paths against **both**
executables:

1. An upstream `response.created` prelude and `keepalive` followed by a
   `server_is_overloaded` failure. The prelude must be discarded; the
   retry must complete with visible output and no leaked failed response ID.
   Tagged Prodex 0.437.9 retries the same original account here, so a
   second attempt **does not** prove cross-account rotation.
2. A heartbeat and actual visible output followed by overload. The client
   must receive the committed frames and there must be **no** second
   upstream attempt or duplicate execution.
3. A verified `previous_response_id` bound to the primary account and
   a conflicting soft `session_id` bound to the secondary account.
   The hard previous-response owner must receive exactly one attempt.
4. Restart the gateway process, open a fresh local WebSocket, and repeat
   the hard-owner conflict. The original account must still own the request
   after in-memory connection and routing caches are gone. This is a
   persistence check, not merely a warm same-socket continuation.

Run from a **clean committed Godex checkout**, with a locally built candidate
containing Go VCS metadata:

```sh
go build -trimpath -o /tmp/godex-04379-candidate ./cmd/godex
go run ./tools/continuationparity \
  --prodex /path/to/official/prodex-0.437.9 \
  --prodex-source /path/to/clean/prodex-tag-0.437.9 \
  --godex /tmp/godex-04379-candidate \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)" \
  --websocket
```

The official reference executable must report `prodex 0.437.9`, have
SHA-256 `58027b0d1825b47e60c35ce1082c4249ed2fa9629174cdf8805312d24562b37e`,
and correspond to source commit
`0e321c92f4e580aa0b090085d5af13a8b856c3f1`.
The WebSocket mode additionally rejects a dirty Godex source tree, a
different expected commit, or missing/modified Go executable VCS metadata.

The harness fails on mismatched event order, missing visible output,
extra upstream attempts, leaked precommit response IDs, wrong hard-affinity
owner, and invalid framing. Unit negative controls intentionally mutate these
observations. See `scripts/verify-release-parity.sh` for the exact-tagged
release-only invocation. That release script still blocks on *all* other
oracle checks; local scenario passes do not authorize a tag or release.
