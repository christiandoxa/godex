# Continuation loopback fixture

This executable creates isolated two-account ChatGPT profile homes, starts one
loopback upstream, and runs the same client sequence through the pinned Prodex
0.437.7 binary and a Godex binary:

- hard previous-response plus conflicting soft-session routing;
- turn-state routing;
- precommit overload/quota responses and full-context replay attempts;
- a streamed response with visible output;
- cancellation and a fresh process restart.

Run it from a committed candidate checkout:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/continuationparity \
  --prodex /home/doxa/.local/bin/prodex \
  --godex /tmp/godex-candidate \
  --prodex-source /tmp/prodex-04373-timeout-20261010
```

The backend records account IDs and synthetic request modes only; it never
records bearer tokens. It answers the quota endpoint with a valid ready usage
payload. The fixture preloads equivalent hashed lineage and quota state: Prodex
uses profile names, while Godex uses managed account IDs. The output includes
response status markers, per-mode attempt counts, and the account trace so a
status difference can be diagnosed as a seed or routing difference. It is a
bounded differential observation, not a certificate of identical recovery
policy: the tagged runtime and Godex may return a terminal 503 or wait for a
bounded recovery epoch where the other returns a full-context signal. Focused
routing tests assert the hard-affinity contract directly.
