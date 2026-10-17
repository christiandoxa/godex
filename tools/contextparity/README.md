# Native context differential probe

`contextparity` launches a recording synthetic Codex executable through the
official Prodex `0.438.1` binary and a Godex candidate. It registers isolated
synthetic profiles, seeds a `models_cache.json` entry whose maximum is larger
than its active context, then checks explicit context/compaction overrides,
profile-only launches, and model switching. The probe compares the values that
reach the child process and does not make a model request.

From a **clean committed Godex checkout**, build the candidate and
run the strict native-context probe against the pinned reference:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/contextparity \
  --prodex /path/to/official/prodex-0.438.1 \
  --godex /tmp/godex-candidate \
  --prodex-source /path/to/tagged/prodex-source \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)"
```

The reference is Prodex tag `0.438.1` at commit
`c3c96af2a9b87896e262e37f37cd4741a562a66f`, with the pinned binary
SHA-256 `b347889a6fa5aec6f3a0f00a1d8346eb2df30b9933361e924900904ed85e165e`.
The probe rejects unclean or mismatched source trees, a stale/dirty Godex
executable based on its embedded VCS metadata, and cached maximum-window
promotion. It is a required stage in the fail-closed release-only parity
gate, `scripts/verify-release-parity.sh`.
