# Native context differential probe

`contextparity` launches a recording synthetic Codex executable through the
official Prodex `0.438.2` binary and a Godex candidate. It registers isolated
synthetic profiles, seeds a `models_cache.json` entry whose maximum is larger
than its active context, then checks explicit context/compaction overrides,
profile-only launches, and model switching. The probe compares the values that
reach the child process and does not make a model request.

From a **clean committed Godex checkout**, build the candidate and
run the strict native-context probe against the pinned reference:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/contextparity \
  --prodex /path/to/official/prodex-0.438.2 \
  --godex /tmp/godex-candidate \
  --prodex-source /path/to/tagged/prodex-source \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)"
```

The reference is Prodex tag `0.438.2` at commit
`decfed45f884fe2bef20911303beab7b198698b7`, with the pinned binary
SHA-256 `3f2fcd776fee0335dc49910d0651adc43a55c8627913556c307755b1575dac3f`.
The probe rejects unclean or mismatched source trees, a stale/dirty Godex
executable based on its embedded VCS metadata, and cached maximum-window
promotion. It is a required stage in the fail-closed release-only parity
gate, `scripts/verify-release-parity.sh`.
