# sessionfooterparity

`sessionfooterparity` compares the saved-session footer contract in the exact
Prodex `0.438.2` executable and a clean Godex executable. It runs both under a
real Linux PTY and with ordinary pipes, using one credential-free synthetic
Codex helper for version probes, app-server readiness, and a launch-owned
SessionStart callback.

The helper never contacts a provider. The gate checks canonical identity,
picker fallback, all-stream TTY suppression, append-only ordering, terminal
control/socket leakage, and child exit-status preservation, including a
synthetic non-zero child exit whose footer must still be appended after
teardown.

Prodex 0.438.2 also gets a dedicated normal-buffer handoff case: the fake
Codex first enters/exits its alternate screen, prints native usage, then exits.
The cross-binary fixture requires the parent to append `DEC 1047` only (no
second `DEC 1049`, cursor restore, clear, or duplicate/truncated resume line).
The negative-control unit test rejects stale cursor restoration and missing
buffer selection independently of parity between the two executables.

```text
sessionfooterparity \
  --prodex /path/to/prodex-0.438.2 \
  --godex /path/to/godex \
  --prodex-source /path/to/prodex-source \
  --godex-source /path/to/godex-source \
  --godex-commit <full-godex-sha>
```

Linux PTY support is required. The command verifies the pinned Prodex digest,
source commit, and clean Godex build metadata before launching either binary.
