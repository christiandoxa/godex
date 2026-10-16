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
| Local proxy under accept and accepted-socket resource pressure | PARTIAL | Real isolated Linux `RLIMIT_NOFILE=96` subprocess exhausted file descriptors to EMFILE against the actual Godex proxy listener. HTTP request queued through real TCP during pressure succeeded exactly once after release, and next request succeeded on the same port; native Go `net/http.Server.Serve` supplies bounded temporary-accept retry. Go has no equivalent tiny_http accepted-socket clone step. `TestProdex04380ProxyListenerSurvivesActualEMFILEAndQueuedRequest` PASS; actual Prodex two-executable descriptor-pressure oracle and indefinite-exhaustion/shutdown proof remain open. |
| Official Codex 0.163.0 compatibility | PARTIAL | Official `rust-v0.163.0` asset metadata and SHA-256 verified; extracted binary `codex-cli 0.163.0` successfully completed real isolated app-server `initialize`, `thread/list` active and archived through the Godex protocol on Linux x86_64. `TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList` PASS. `response.incomplete`, compaction, native retry/interrupt and Windows MCP environments still need separate executable evidence. |
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

## Real Linux TCP descriptor-pressure checkpoint

The tagged Prodex 0.438.0 `tiny_http` patch protects the owned loopback
listener when accepting a socket or cloning an accepted socket encounters
EMFILE. Godex uses the Go standard-library HTTP server rather than the Rust
socket-cloning transport; Go's `net/http.Server.Serve` has its own bounded
retry for temporary `net.Listener.Accept` errors. The actual Godex
`Proxy.Start()` path was tested in a dedicated subprocess that sets
`RLIMIT_NOFILE=96` and opens descriptors until Linux reports EMFILE. It
sends one HTTP response before pressure, opens a real queued TCP connection
under pressure, verifies that it does not complete prematurely, then releases
the file descriptors. The queued request must produce the second response
exactly once; an independent fresh connection must produce the third response
on the identical listening endpoint. The captured native HTTP server log must
contain the actual accept EMFILE error; empty synthetic transient-error mocks
do not qualify. The success path requires all these facts.

This provides **verified real Linux accept-pressure recovery** on Godex.
It does not certify the Rust accepted-socket-clone path (which Go does not
use), nor an equivalent two-executable actual-descriptor-pressure fixture,
Windows listener behavior or indefinitely persistent resource exhaustion.

## Official Codex 0.163.0 native app-server handshake

The release `rust-v0.163.0` was verified through the official OpenAI/Codex
GitHub release API, with tagged commit
`83086188ae0614bd882818ff1aa23f3194962012`. The published Linux x86_64
musl tarball SHA-256 is
`beb339e8ecd9ba057a6bb2dc81ad8ca474812edfbf0fc5056ca818797648dde4`;
its independently extracted 296,865,480-byte `codex` executable has SHA-256
`399ff69c431c066e40de318fba45c2b07f45a76b19939d247ee4828fc70c4215`
and reports `codex-cli 0.163.0`. The opt-in Go regression runs its actual
`app-server --listen stdio://` with a private empty configuration and state
home, then performs real JSON-RPC `initialize` and active/archived `thread/list`
handshakes using Godex's existing bounded protocol, without starting a model
turn or loading an installed Codex login. All requests completed successfully.

Use `GODEX_TEST_CODEX_0163_APP_SERVER_BIN=/path/to/sha-verified/codex` to run
the optional release-gate check. The ordinary release gate does not download
or replace the user's Codex executable. This is VERIFIED for the narrow native
app-server handshake on Linux x86_64 only; the other native Codex 0.163.0
changes documented by Prodex remain PARTIAL/OPEN pending targeted tests.
