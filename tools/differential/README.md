# Differential harness

Build and run from a clean, committed Godex worktree:

```bash
go test ./tools/differential
go build -trimpath -o /tmp/differential ./tools/differential
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
/tmp/differential \
  --prodex /absolute/path/to/prodex-0.436.1 \
  --godex /tmp/godex-candidate \
  --prodex-source /absolute/path/to/prodex-0.436.1-source \
  --godex-source "$PWD" \
  --godex-commit "$(git rev-parse HEAD)"
```

The harness creates separate temporary homes, starts one loopback mock upstream,
and runs both products independently through five bounded scenarios: a successful request; a
synthetic 429 followed by a retry owned by the **Codex shim**; a single-key
429 that must remain terminal without **proxy** retry; cancellation while
upstream is delayed; and two launches against the same home. Each
run records bounded stdout/stderr, child response, upstream requests, ordered
upstream events, retry count, cancellation observation, exit status, and a
redacted durable-state snapshot. The comparison also has mutation sentinels
for every observed category. It requires the exact Prodex source commit and
version before running. In addition to the Godex source commit, it verifies the
candidate executable's embedded Go VCS revision and requires `vcs.modified=false`.
A stale binary, dirty build, or build without VCS metadata fails before execution.
Both reference and candidate source trees must also have no tracked modifications
or untracked files; a locally edited Prodex checkout cannot serve as the oracle.
The resulting PASS is limited to the five named synthetic scenarios and is
not equivalent to a global provider, live-TUI or transport parity certificate.

Use `--scenario success|retry|single-key-429|cancel|restart` to rerun one case while
investigating a mismatch; the default is `--scenario all`.


Run only from a committed, clean candidate checkout; build the Godex binary
from that exact source. The supplied --godex-commit pin must match HEAD **and the binary's embedded
Go VCS build revision**. The harness refuses binaries built from dirty or
uncommitted source.
The canonical reference is Prodex tag 0.436.1, not an arbitrary binary release.

**Fail-closed behavior:** the harness prints a structured JSON comparison and
returns a nonzero process exit for any mismatch, missing child/upstream evidence,
or a run that violates the scenario-specific expected outcome. The cancellation
scenario expects both interrupted children to exit 1 and requires observed
cancellation; ordinary success, shim-retry, and restart runs require exit 0, status
200 and a real response body. The single-key 429 case requires exit 2,
status 429, the original structured rate-limit error, and no proxy-owned
retry; all cases require the expected number of upstream requests.
The negative controls alone never imply parity. Current header, durable state,
stderr, and upstream metadata differences are intentionally still reported.

Do not interpret this five-scenario harness as proof of full 1:1 parity;
additional transport, provider, and persistence contracts remain to verify.

### Strict fixture oracle and audited transport metadata

The mock verifies the exact synthetic Bearer credential **before redacting** its
bytes, and every upstream request must use POST /v1/chat/completions with the
expected model, user text, and non-streaming request. Every successful response
must contain the expected translated output and usage totals. Equal invalid
credentials or equal corrupted model output cannot pass.

HTTP header comparison remains exact except for three explicitly audited
transport fingerprints: the current Date value (both must parse as fresh HTTP
dates), Prodex's default Server: tiny-http (Rust) versus Godex's absent server
fingerprint, and the tagged HTTP client's duplicate identical Go-http-client/1.1
User-Agent versus one occurrence in Go. All other headers, including
Authorization evidence, Accept, Content-Type, Content-Length, Retry-After, and
X-Codex-*, retain strict comparison and negative-control tests. These exceptions
are **not** global exclusions or proof that opaque durable-state layouts are
equivalent.
