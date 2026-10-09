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
and runs both products independently through eight bounded scenarios: a successful request; a
synthetic 429 followed by a retry owned by the **Codex shim**; a single-key
429 that must remain terminal without **proxy** retry; cancellation while
upstream is delayed; two launches against the same home; and a terminal 429 followed by a
fresh process with the same home and a healthy provider, which must succeed. Each
run records bounded stdout/stderr, child response, upstream requests, ordered
upstream events, retry count, cancellation observation, exit status, and a
redacted durable-state snapshot. The comparison also has mutation sentinels
for every observed category. It requires the exact Prodex source commit and
version before running. In addition to the Godex source commit, it verifies the
candidate executable's embedded Go VCS revision and requires `vcs.modified=false`.
A stale binary, dirty build, or build without VCS metadata fails before execution.
Both reference and candidate source trees must also have no tracked modifications
or untracked files; a locally edited Prodex checkout cannot serve as the oracle.
The resulting PASS is limited to the eight named synthetic scenarios and is
not equivalent to a global provider, live-TUI or transport parity certificate.

Use `--scenario success|retry|single-key-429|single-key-503|cancel|restart|recover-after-429|recover-after-503` to rerun one case while
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

Do not interpret this eight-scenario harness as proof of full 1:1 parity;
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


The source-pinned DeepSeek runtime produces a known fixed stderr startup
banner on Prodex, whereas Godex emits none. The narrow comparator accepts
only that exact banner, and for cancellation it requires both programs to
report the same client timeout semantics through their validated respective
loopback routes. Any unexpected auth error, warning, quota diagnostic,
non-loopback destination, or changed cancellation reason fails the test.
This is a documented CLI-presentation difference, **not literal stderr
byte-for-byte parity**.


### Fixture durable-state integrity

Both products are checked independently for output artifacts that the
synthetic provider workflow must not create: foreign Codex sessions, polluted
Codex history, invalid JSON state, leaked synthetic credentials, and
inconsistent routing/last-good recovery snapshots. File permissions are
checked against world-writable artifacts and file size/type bounds. These
checks run on **actual temporary state files** after each product exits, with
negative controls for corruption and secret leakage.

The Rust and Go repositories use deliberately different private bookkeeping
layouts. Raw durable file inventories and digests are still included and
compared under the fail-closed strict gate. The integrity checks do **not**
certify equivalence of account/goal/journal semantics, so the raw layout
difference remains a blocker pending domain-specific restart/fault injection
proof.


The recover-after-429 case runs **four real product processes** across two
isolated per-product homes: the first process receives a terminal structured
429, then a fresh process with the same persistent home must complete a
healthy request without stale retry/backoff state. The oracle checks actual
upstream attempt counts and both process generations, rather than simply
comparing equal file hashes. This tests one lifecycle path, not every
persistence or concurrent-recovery contract.


The added single-key-503 scenario checks the canonical terminal HTTP 503:
a sole provider must expose the outage unchanged rather than waiting for
an ineligible same-key recovery attempt and returning a local timeout.


The recover-after-503 fixture uses the same isolated state home before and
after a real process restart. It ensures provider health changes caused by
an upstream outage do not block a later healthy request. Prodex writes a
runtime-scores sidecar after the failure; this is a real state transition,
so the differential audit must review its semantics rather than treating
it as harmless private-file noise.


The exact Prodex 0.436.1 runtime can create a versioned
runtime-scores.json health sidecar, associated locks, and numeric
runtime-scores.json.*.tmp artifacts following an upstream overload. The
temporary files are only accepted with the audited three-number naming
scheme, bounded file type and size, and synthetic-secret scanning.
These artifacts are not treated as proof of equal **health scoring**:
the fixture verifies the subsequent process's observable routing outcome,
while multi-profile health priority still needs its own oracle.


The synthetic fixture's sole-key Prodex health-score writer was observed to
leave a versioned envelope with generation >= 1 and an empty value map.
The state auditor now decodes committed and in-flight health-score sidecars
and fails on corrupt envelopes, unknown fields, or nonempty scores, which
would affect provider selection. Godex's route-memory list is separately
required to be empty. This proves both lack active health-selection state
for the single-key fixture, not that arbitrary multi-profile health ranking
policies agree.
