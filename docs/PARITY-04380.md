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
| 0.438.0 pinned release differential | OPEN | Prior 0.437.14 4-phase finite gate PASS is historical; update exact source/binary pins and rerun all phases against clean 0.438.0 candidate with active negative controls. |
| Three-profile quota/auth restart | PARTIAL | Prior Godex 0.437.14 synthetic and multi-process tests PASS. Independent two-executable multi-profile restart differential is still needed. |

**Status is PARTIAL, not certified 1:1 production parity.** Only a fully
pinned oracle and adequate behavioral evidence can mark a surface VERIFIED.
User account secrets must never appear in tests, logs or persisted fixtures.
Push checkpoint commits normally to `main`, without force or waiting for CI;
never replace existing WIP in `/home/doxa/IdeaProjects/godex`.
