# Native context differential probe

`contextparity` launches a recording synthetic Codex executable through the
official Prodex `0.437.13` binary and a Godex candidate. It registers isolated
synthetic profiles, seeds a `models_cache.json` entry whose maximum is larger
than its active context, then checks explicit context/compaction overrides,
profile-only launches, and model switching. The probe compares the values that
reach the child process and does not make a model request.

From a **clean committed Godex checkout**, build the candidate and
run the strict native-context probe against the pinned reference:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/contextparity \
  --prodex /path/to/official/prodex-0.437.13 \
  --godex /tmp/godex-candidate \
  --prodex-source /path/to/tagged/prodex-source \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)"
```

The reference is Prodex tag `0.437.13` at commit
`f5694b42a749c61f272e7bf378c376816fb6b1e8`, with the pinned binary
SHA-256 `027dfcb094fab7c7fab7487dee3e2b22eabf93aab4bec5150e982391eccad874`.
The probe rejects unclean or mismatched source trees, a stale/dirty Godex
executable based on its embedded VCS metadata, and cached maximum-window
promotion. It is a required stage in the fail-closed release-only parity
gate, `scripts/verify-release-parity.sh`.
