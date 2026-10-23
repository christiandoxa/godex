# Native context differential probe

`contextparity` launches a recording synthetic Codex executable through the
official Prodex `0.439.0` binary and a Godex candidate. It registers isolated
synthetic profiles, seeds a `models_cache.json` entry whose maximum is larger
than its active context, then checks explicit context/compaction overrides,
profile-only launches, and model switching. The probe compares the values that
reach the child process and does not make a model request.

From a **clean committed Godex checkout**, build the candidate and
run the strict native-context probe against the pinned reference:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/contextparity \
  --prodex /path/to/official/prodex-0.439.0 \
  --godex /tmp/godex-candidate \
  --prodex-source /path/to/tagged/prodex-source \
  --godex-source "$PWD" --godex-commit "$(git rev-parse HEAD)"
```

The reference is Prodex tag `0.439.0` at commit
`433aad44bb249d5c25a231a6b06c2fa522a39a1f`, with the pinned binary
SHA-256 `2dec9565fd94eb59aeb83ae5e7cc4b772db130cbd45258d86a2104c8f4ba186a`.
The probe rejects unclean or mismatched source trees, a stale/dirty Godex
executable based on its embedded VCS metadata, and cached maximum-window
promotion. It is a required stage in the fail-closed release-only parity
gate, `scripts/verify-release-parity.sh`.

The native-argument matrix also records exact child argv for root/exec/resume
config scopes, repeated and inline config forms, a literal `--` tail, and
`/model`, `/permissions` and `/presets` prompt text. Each product gets a shared
synthetic rollout plus its original selected profile. Exact argv, exit status,
child CODEX_HOME and unchanged profile config/model cache are required. Mutation
checks reject reordered or dropped overrides and rewritten prompt text. The
recording child performs no authentication, tools or model turn.
