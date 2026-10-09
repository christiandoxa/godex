# Prodex 0.437.0 managed-profile lifecycle differential

This tool compares real Prodex and Godex binaries, each operating in a
separate fresh HOME/PRODEX_HOME/GODEX_HOME, over 19 sequential CLI operations.
Every CLI invocation is a new process. No account login, provider API key,
or external model request is needed. The subprocess receives a minimal
environment with nonfunctional outbound proxy settings.

It pins the Prodex source to commit
b70f7429fb163760e3cd35a6564a0e79f9281a49 and the official Linux
artifact SHA-256 to
0082ed1348183cc53b0dc4ad44d9f6a5e009d3d3dcc8bbf372bc2eec60447d76.
It checks that the Godex executable's embedded clean VCS revision equals the
candidate source commit. Both source checkouts must be clean.

To run from a clean Godex checkout, build the two Go binaries and invoke:

    go build -trimpath -o /tmp/godex-candidate ./cmd/godex
    go build -trimpath -o /tmp/profileparity ./tools/profileparity
    /tmp/profileparity --prodex /path/to/official/prodex-0.437.0 \
      --godex /tmp/godex-candidate \
      --prodex-source /path/to/prodex-tag-0.437.0 \
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


### External CODEX_HOME ownership

The profile lifecycle also creates a synthetic pre-existing external
CODEX_HOME with a sentinel file. Both clients must register it as
unmanaged, retain it on refused --delete-home, preserve the sentinel,
and leave it untouched when the profile entry is eventually removed.
Any symlinked, world-readable or modified external directory fails the
domain oracle. Tests reject a false managed designation or an external
path redirected under a managed profile home.
