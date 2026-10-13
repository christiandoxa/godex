# Prodex 0.437.8 parity checkpoint

Reference source: `8c5c6f40fb4f864d1542f9c8fa48d15863316652` (`0.437.8`).
Official Linux amd64 binary SHA-256: `49de897a147c1ab7ec7aebdcd6d8fd2075cb0b6de58487062eedc9be3da4eb1e`.

## Native recovery classification

Tagged source `mojo/prodex_core/launch_config.mojo` includes an exact
`codex_error_info="other"` compatibility classification for the terminal
message `request timed out` or `Error running remote compact task: request
timed out` when HTTP status is absent, 408 or 504. Godex previously returned
no recovery class for these native terminal errors; its existing typed
recovery classifier now returns `transport` without bypassing explicit HTTP
client errors, successful/cancelled turn guards, ownership or accepted-work
checks. See `recovery_timeout_04378_test.go` for failing-before/after
classifier tests, persisted post-child rollout and independently guarded live
tail supersession tests. The event replay, tool effects and account selection
algorithms were not changed.

## Request-scoped precommit watchdog

Tagged Prodex source runtime_proxy/recovery_deadline.rs, Mojo
quota_pressure.mojo, and failure_response.rs require one monotonic
precommit deadline per logical Responses/unary-compact/WebSocket-message
request. Its default is max(stream idle 300,000ms, recovery epoch 30,000ms),
or five minutes. Godex now scopes one timeout across routing, admission,
profile waits and upstream attempts and returns typed 504 upon expiration.
An expiring watchdog cancels transport work but is disarmed upon successful
precommit: a committed streaming response is not cancelled just because its
precommit budget expires. The optional routing constructor budget permits
small fixture durations without changing production defaults.

Regressions verify cancellation and 504 for both HTTP Responses and compact
through the real local HTTP handler, plus a non-Responses route and stream
survival after precommit. This does not yet prove complete official-binary
parity of every timeout and WebSocket frame boundary.

## Outstanding 0.437.8 gaps

This is NOT evidence of full binary parity. Multi-epoch provider cooldown,
full-history compaction/affinity boundary, app-server and WebSocket message
deadline qualification, provider surfaces, persistence and cross-process
negative-control evidence still require source-pinned behavioral testing.
Do not tag v0.2.1 based on the passing Go suite alone.


## Release-gating oracle target

The strict source/binary provenance pins in the independent profile, CLI
error, context, continuation and DeepSeek differentials now reference official
Prodex tag 0.437.8 (commit
8c5c6f40fb4f864d1542f9c8fa48d15863316652, Linux x86_64 SHA-256
49de897a147c1ab7ec7aebdcd6d8fd2075cb0b6de58487062eedc9be3da4eb1e).
The release verification script refuses substituted oracle binaries/sources.
Its strict durable state inventory remains an explicit release blocker; do
not treat domain-specific PASS as an overall FULL PARITY certificate.


## Independent canonical 0.437.8 executable qualification

Against clean candidate commit 6fd4871 and the exact official Linux binary:

- Managed profile lifecycle: PASS, 31 of 31 subprocess stages, including
  credential-free startup usage probes.
- CLI rejection differential: PASS, all eight probes and negative control.
- Native Codex context configuration: PASS, three explicit model/profile cases
  and the cached-window negative control.
- Strict DeepSeek success fixture: FAIL; the verifier reported an observable
  stderr mismatch. This remains a release blocker pending root-cause
  analysis, in addition to broader durable-state and provider qualification.

Do not infer full parity or remove strict comparison dimensions from the
independent differential merely because focused Go tests or CI pass.
