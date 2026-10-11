# Prodex 0.437.3 oracle and process-tree parity

Canonical Prodex is tag `0.437.3`, source commit
`03815a6157e7a67e83e38fc2a79bd679cc988fa0`, and the installed Linux x86_64
binary is pinned to SHA-256
`834275c168121ce181eb7a7ff0b730b392c23e853803b6dea4a27e6166a6d074`.

The active differential and managed-profile harnesses now reject any other
Prodex source, version, or binary digest. The release workflow uses the same
pin.

## Independent process-tree oracle

Prodex 0.437.3 gives each session-owned app-server companion its own process
group and installs a Linux parent-death guard. The deterministic Go oracle
launches a helper subprocess, which starts a second test process through the
same app-server process configuration, then kills the helper. The guarded child
must exit; a deliberately unconfigured child is a negative control and must
remain alive until the test cleans it up. The fixture writes only a synthetic
PID under `t.TempDir()` and never logs credentials or user paths.

Before the production change, the guarded child survived helper termination.
After the change, the guarded and negative-control cases both pass. This
distinguishes the test result from a broad behavioral-parity claim: it proves
the Linux parent-death contract and normal process-group setup, not every
platform's process supervisor or a live Codex session.

The opt-in Codex wire fixture is also pinned to the official `codex-cli
0.162.1` app-server command and validates credential-free thread-index RPCs.
