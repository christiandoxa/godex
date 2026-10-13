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

## Outstanding 0.437.8 gaps

This is a verified narrow classification fix, NOT evidence of full binary parity.
The new absolute request-scoped precommit watchdog (HTTP Responses, unary
compaction, WebSocket message), multi-epoch provider cooldown and local 504
classification, full-history compaction/affinity boundary, provider surfaces,
persistence and cross-process evidence still require source-pinned behavioral
qualification. Do not tag `v0.2.1` based on the passing Go suite alone.
