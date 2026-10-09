# Differential harness

Build and run from this worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
/tmp/differential \
  --prodex /absolute/path/to/verified/prodex-0.436.1 \
  --godex /absolute/path/to/binary-built-from-godex-source \
  --prodex-source /absolute/path/to/prodex-0.436.1-source \
  --godex-source . \
  --godex-commit "$(git rev-parse HEAD)"
```

The harness creates separate temporary homes, starts one loopback mock upstream,
runs both products with the same synthetic provider request, records bounded
stdout/stderr, child response, upstream observations, exit status, and durable
state, then reports a negative-control comparison. It requires the exact
Prodex source commit and version before running.


The Godex binary must be built from the same committed source given by --godex-source.
Any semantic comparison difference, missing response evidence, or nonzero product exit
causes a nonzero harness exit (even if both products fail identically).
A successful run proves only this synthetic DeepSeek scenario, not full parity.
Do not use actual provider credentials or production accounts.
