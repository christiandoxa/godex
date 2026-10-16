# Godex ↔ Prodex 0.438.0 parity checkpoints

**Target:** official Prodex tag `0.438.0`, source commit
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`.
Published Linux x86_64 SHA-256 for `prodex-x86_64-unknown-linux-gnu`:
`9ec1702c58983a8e3bd7f2e7f49c34a2d656f766fd535bbe34d08c94d436ac2a`.
Reference: tagged `docs/release-notes/0.438.0.md` and source migrations.

The previous pinned baseline is `docs/PARITY-043714.md`. Do not silently
transfer 0.437.14 differential PASS claims to the new 0.438.0 target.

## Behavioral delta matrix

| Surface | Status | Evidence / next proof |
| --- | --- | --- |
| Windows signed process exit and Unix cancellation | PARTIAL | A source-derived signed-i32 policy now detects Windows Ctrl-C NTSTATUS and real Unix SIGINT; original Go logic detected only exit 130. Tagged negative controls and native Linux child-signal tests PASS; Windows cross-compiled test binary, but Windows runtime execution has not been observed. |
| Local proxy under accept and accepted-socket resource pressure | OPEN | Source release fixes both temporary accept failure and cloned socket failure, preserves listening address and queued request after bounded retry. Need real Linux descriptor-pressure tests for Godex listener and connection flow. |
| Official Codex 0.163.0 compatibility | OPEN | Tagged source audits 77 critical files and 90 semantic contracts, including app-server init/list, response.incomplete, compaction, interrupt, session and MCP behavior. Need independent Codex 0.163.0 oracle and matching Godex tests. |
| 0.438.0 pinned release differential | PARTIAL | Exact 0.438.0 tagged source, release manifest, downloaded Linux binary/version and SHA-256 were verified. All six executable differential harnesses and the fail-closed release script now pin the new source/binary. `go test ./tools/...` PASS; 4-phase cross-binary tests remain to be executed against a clean committed Godex candidate. |
| Three-profile quota/auth restart | PARTIAL | Prior Godex 0.437.14 synthetic and multi-process tests PASS. Independent two-executable multi-profile restart differential is still needed. |

**Status is PARTIAL, not certified 1:1 production parity.** Only a fully
pinned oracle and adequate behavioral evidence can mark a surface VERIFIED.
User account secrets must never appear in tests, logs or persisted fixtures.
Push checkpoint commits normally to `main`, without force or waiting for CI;
never replace existing WIP in `/home/doxa/IdeaProjects/godex`.

## Official 0.438.0 oracle checkpoint

The tagged source checkout is clean at
`8cab9794b0e08b2cb8507df8321a8e38ebd6492f`. The official Linux
x86_64 executable was downloaded from the published release, measured at
39,355,488 bytes, verified against the published SHA256SUMS, and reported
`prodex 0.438.0`. Its release-manifest version and source commit match the
same tag. The release gate and all six independently source-verified Go
binary differentials have been pinned to these exact values, with their
negative controls retained. Existing `TestProdex043714...` names remain
historical regression cases; their names are not evidence that 0.438.0
cross-binary comparisons passed. Re-run the gate on a **clean Godex commit**
before promoting any parity surface to VERIFIED.
