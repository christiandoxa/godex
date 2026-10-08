# Differential harness

Build and run from this worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
/tmp/differential \
  --prodex /tmp/godex-parity-prodex-0.436.1/target/debug/prodex \
  --godex /tmp/godex-04361-wave3/w15-artifacts/godex \
  --prodex-source /tmp/godex-parity-prodex-0.436.1 \
  --godex-source /tmp/godex-04361-wave3/w15-differential
```

The harness creates separate temporary homes, starts one loopback mock upstream,
runs both products with the same synthetic provider request, records bounded
stdout/stderr, child response, upstream observations, exit status, and durable
state, then reports a negative-control comparison. It requires the exact
Prodex source commit and version before running.
