# Prodex 0.438.0 managed-profile lifecycle differential

This tool compares real Prodex and Godex binaries, each operating in a
separate fresh HOME/PRODEX_HOME/GODEX_HOME, over 29 sequential CLI operations.
Every CLI invocation is a new process. No account login, provider API key,
or external model request is needed. The subprocess receives a minimal
environment with nonfunctional outbound proxy settings.

It pins the Prodex source to commit
8cab9794b0e08b2cb8507df8321a8e38ebd6492f and the official Linux
artifact SHA-256 to
9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a.
It checks that the Godex executable's embedded clean VCS revision equals the
candidate source commit. Both source checkouts must be clean.

To run from a clean Godex checkout, build the two Go binaries and invoke:

    go build -trimpath -o /tmp/godex-candidate ./cmd/godex
    go build -trimpath -o /tmp/profileparity ./tools/profileparity
    /tmp/profileparity --prodex /path/to/official/prodex-0.438.0 \
      --godex /tmp/godex-candidate \
      --prodex-source /path/to/prodex-tag-0.438.0 \
      --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)"

The tool returns exit 1 for *any* unexpected outcome and produces a JSON
report only when every independent stage and both state projections pass.
The profile schemas differ intentionally; the oracle verifies their
observable semantics (names, active selection, providers, private managed
homes, retained external homes, refused external deletion, removed-home
behavior, and exact exit codes), not raw hashes.
Adversarial Go tests cover corrupt state and credential-free isolation.

This is necessary evidence for managed-profile lifecycle parity, not proof
that arbitrary OAuth, multi-profile recovery, quota, goals, TUI, or other
runtime behavior matches. The separate DeepSeek differential and release
gate remain required and fail-closed.


### Startup quota warmup

Two startup cases extend the lifecycle oracle beyond the 29 profile and CLI
operations. With a compatible managed profile and a synthetic successful
usage response, both binaries issue exactly one `/backend-api/wham/usage`
request using GET, the synthetic bearer/account identity, and the Codex quota
compatibility headers, then persist a valid generation-one usage snapshot. With
the same profile and an upstream 503, both issue exactly one authenticated
request and persist no snapshot. The cases use a loopback fixture and wait for
the durable snapshot before comparing state, so they verify the bounded startup
probe contract without requiring a real account or upstream service. A request
with the wrong method, path, or credentials cannot satisfy the oracle even if
both products make the same mistake.


### External CODEX_HOME ownership

The profile lifecycle also creates a synthetic pre-existing external
CODEX_HOME with a sentinel file. Both clients must register it as
unmanaged, retain it on refused --delete-home, preserve the sentinel,
and leave it untouched when the profile entry is eventually removed.
Any symlinked, world-readable or modified external directory fails the
domain oracle. Tests reject a false managed designation or an external
path redirected under a managed profile home.


### Profileless provider gateway startup

The additional negative CLI case supplies a synthetic DeepSeek --api-key
but deliberately configures no managed/active Codex-compatible profile.
Prodex 0.438.0 requires compatible profile selection before starting the
gateway, so both implementations must reject startup with exit 1 and
leave all state unchanged. The separate Go regression proves a configured
provider profile can still open and close the raw-key gateway successfully.

This narrow startup contract does not prove gateway request behavior with
an unauthenticated OpenAI profile. That remains independently unverified.


### CLI parser failure parity

Both product CLIs must reject an invalid gateway --provider openai as a
usage error with exit code 2, not a runtime error with exit code 1.
A source-audited typed Go CLI argument error preserves the original message
while giving the canonical parser status. This check runs before any profile
is created and must not mutate the synthetic state.


### Gateway argument validation matrix

Eight further syntactic errors have been added to the executable
differential: unknown or late unknown options, missing provider/listen/
base-url/api-key arguments, unexpected positional input, and conflicting
--presidio with --no-presidio. All must return exit 2 from both
executable CLIs without changing the credential-free persisted state.
A separate Go test ensures a valid argument set that fails during
runtime initialization is **not** misclassified as a usage error.

### Prodex 0.438.0 startup quota 401 reload

The isolated `startup_usage_401_credential_reload` scenario rotates a
synthetic managed-profile credential after a rejected HTTP quota request. Both
executables must make two correctly authenticated GET requests and persist one
valid quota snapshot. An unchanged second authorization header, an unexpected
third attempt, or a missing snapshot fails the oracle.

The fixture resolves each product's independently managed profile home rather
than assuming identical internal directory layouts. It rejects ambiguous or
symlinked paths, operates only inside test-specific temporary roots, and never
uses personal accounts. This does not certify recovery of inherited auth
penalties across a process restart.

### Exact Prodex 0.438.0 Linux socket exhaustion differential

In addition to synthetic profile lifecycle and HTTP 401 reload checks,
`profileparity` runs `gateway_fd_pressure_recovery` on Linux against the
published SHA-pinned Prodex binary and the clean Go candidate. Each CLI uses
its own synthetic managed ChatGPT credential and temporary HOME, an inherited
`RLIMIT_NOFILE=80`, and a loopback-only HTTP upstream. The tool opens 120
partial-header TCP connections, confirms the real child owns 80/80 file
descriptors via `/proc/PID/fd`, then queues a fully formed GET while the
listener has no descriptors. Neither gateway may exit or handle that GET
before capacity returns. Closing the held connections must release exactly
one queued GET and allow one new GET on the unchanged port. The upstream
must observe exactly three model-list requests overall.

The fixture allocates a short private temporary directory because the Rust
product limits capability-secret path length; this does not weaken socket or
credential checks. Every failure aborts the whole differential, and negative
controls reject missing FD saturation, wrong counts, lost listener/queue, and
unclean shutdown. Internal Rust socket-clone failure and native Windows/macOS
behaviors are distinct from this Linux shared observable contract.
