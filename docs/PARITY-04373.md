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

## Independent CLI error oracle

`tools/clierrorparity` now runs a credential-free executable matrix against
the same pinned source commit and Linux binary digest. It covers the valid
empty-profile `current` read plus rejected positional or option forms for
`current`, profile/session listing, `info`, `status`, and profile creation or
removal. It also runs a synthetic mutation negative control: a candidate that
accepts `current synthetic-extra` must fail the oracle.

The first run exposed a real delivery mismatch. Canonical Prodex rejected
`current synthetic-extra`; Godex returned success because the profile-backed
dispatcher discarded the arguments before the profile command parser saw them.
The dispatcher now forwards those arguments, and the regression test plus the
executable oracle require rejection with a diagnostic. The oracle remains
narrow: it does not claim parity for authentication, live providers or quota,
interactive TUI rendering, or platform-specific process supervision.

At this checkpoint the following domains remain **NOT VERIFIED** by the new
executable evidence: live ChatGPT/device authentication and token refresh;
real provider quota and transport behavior; full TUI lifecycle and terminal
resize/signal handling; Windows and macOS runtime/process semantics beyond
cross-build compilation; release download/install execution; and arbitrary
durable-state or concurrent mutation paths outside the existing focused
fixtures.
