# Native context differential probe

`contextparity` launches a recording synthetic Codex executable through the
official Prodex `0.437.7` binary and a Godex candidate. It registers isolated
synthetic profiles, seeds a `models_cache.json` entry whose maximum is larger
than its active context, then checks explicit context/compaction overrides,
profile-only launches, and model switching. The probe compares the values that
reach the child process and does not make a model request.

From this checkout after building the candidate:

```bash
go build -trimpath -o /tmp/godex-candidate ./cmd/godex
go run ./tools/contextparity \
  --prodex /home/doxa/.local/bin/prodex \
  --godex /tmp/godex-candidate
```

The oracle is Prodex tag `0.437.7` at source commit
`7c774609cf9920d509539cf0e248872f53f6396e`; the binary is pinned by version
and SHA-256. The negative-control test fails if a cached maximum is accepted as
the active context window.
