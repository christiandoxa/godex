# sessionfooterparity

`sessionfooterparity` compares the saved-session footer contract in the exact
Prodex `0.437.12` executable and a clean Godex executable. It runs both under a
real Linux PTY and with ordinary pipes, using one credential-free synthetic
Codex helper for version probes, app-server readiness, and a launch-owned
SessionStart callback.

The helper never contacts a provider. The gate checks canonical identity,
picker fallback, all-stream TTY suppression, append-only ordering, terminal
control/socket leakage, and child exit-status preservation, including a
synthetic non-zero child exit whose footer must still be appended after
teardown.

```text
sessionfooterparity \
  --prodex /path/to/prodex-0.437.12 \
  --godex /path/to/godex \
  --prodex-source /path/to/prodex-source \
  --godex-source /path/to/godex-source \
  --godex-commit <full-godex-sha>
```

Linux PTY support is required. The command verifies the pinned Prodex digest,
source commit, and clean Godex build metadata before launching either binary.
