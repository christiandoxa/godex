# Differential harness

Build and run from this worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
/tmp/differential \
  --prodex /tmp/prodex-04361-reference-20261009-01/target/debug/prodex \
  --godex /tmp/godex-w15-current \
  --prodex-source /tmp/prodex-04361-reference-20261009-01 \
  --godex-source "$PWD" \
  --godex-commit "$(git rev-parse HEAD)"
```

The harness creates separate temporary homes, starts one loopback mock upstream,
and runs both products independently through four bounded scenarios: a
successful request, a synthetic 429 followed by a client retry, cancellation
while the upstream is delayed, and two launches against the same home. Each
run records bounded stdout/stderr, child response, upstream requests, ordered
upstream events, retry count, cancellation observation, exit status, and a
redacted durable-state snapshot. The comparison also has mutation sentinels
for every observed category. It requires the exact Prodex source commit and
version before running; the Godex source commit is recorded from the supplied
checkout so the harness can run against any local candidate build.

Use `--scenario success|retry|cancel|restart` to rerun one case while
investigating a mismatch; the default is `--scenario all`.


Run only from a committed, clean candidate checkout; build the Godex binary
from that exact source. The supplied --godex-commit pin must match HEAD.
The canonical reference is Prodex tag 0.436.1, not an arbitrary binary release.

**Fail-closed behavior:** the harness prints a structured JSON comparison and
returns a nonzero process exit for any mismatch, missing child/upstream evidence,
or a run that violates the scenario-specific expected outcome. The cancellation
scenario expects both interrupted children to exit 1 and requires observed
cancellation; ordinary success, retry, and restart runs require exit 0, status
200, a real response body, and the expected number of upstream requests.
The negative controls alone never imply parity. Current header, durable state,
stderr, and upstream metadata differences are intentionally still reported.

Do not interpret this four-scenario harness as proof of full 1:1 parity;
additional transport, provider, and persistence contracts remain to verify.
