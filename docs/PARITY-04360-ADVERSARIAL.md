# Exact Prodex 0.436.0 parity delta — adversarial audit

**Assessment:** the `0.435.9 → 0.436.0` *launch-option compatibility delta*
is implemented and regression-tested locally. **Full cross-implementation
1:1 parity is not certified.** Prior gaps remain open; see
[`PARITY-04359-ADVERSARIAL.md`](PARITY-04359-ADVERSARIAL.md).

## Immutable reference and scope

- Prodex `0.435.9`: `c9cede4b14f6865ddc3281acc82315d2693cc3c8`.
- Prodex `0.436.0`: `3e1e065b05635a21851b7d6752fb9867026b85a7`.
- Tagged source: `mojo/prodex_core/launch_args_common.mojo`,
  `crates/prodex-runtime-launch/tests/src/lib/args_codex_0161.rs`,
  `crates/prodex-cli/tests/src/codex_0161.rs`, and
  `docs/release-notes/0.436.0.md`.
- The sole production Mojo change adds
  `--cyber-access-program` to the shared value-taking-option predicate.
  The remaining tagged diff is release metadata, compatibility audits,
  additional tests and upstream-marker guards.

The visible release contract is to preserve the **option and adjacent value**
across native Codex command parsing, `exec resume`/fork ID selection,
governed HTTP launch configuration, explicit Daybreak override order, and
run/Super argument forwarding. It does **not** grant Cyber/Daybreak entitlement,
change default models, or validate the program enum in Godex.

## Closed observable deltas and sentinels

| Tagged contract | Godex production path | Regression evidence |
| --- | --- | --- |
| A separated Cyber program value is not a command, session ID, or prompt | `internal/delivery/cli/runtime/session.go` / `nativeOptionTakesValue` | `TestProdex04360CyberAccessProgramGlobalResumeSelector`, `TestProdex04360CyberProgramMustNotBecomeSessionOrPrompt`, `TestProdex04360CyberProgramForkRetainsThreadIdentity` |
| The native `--` separator ends resume-ID scanning, even if the following text looks like a UUID | `internal/delivery/cli/runtime/session.go` / `findResumeSessionSelector` | `TestProdex04360CyberProgramMustNotBecomeSessionOrPrompt` and corrected legacy `TestNativeSessionArgumentForms` |
| Managed Codex config does not misplace the Cyber option/value pair or consume its value as a command | `internal/gateway/codex/config_arguments.go` / `codexOptionTakesValue` | `TestProdex04360CyberProgramPairPreservedByManagedProxyConfiguration`, `TestProdex04360ManagedProxyDoesNotSplitCyberPairOrForceDaybreak` |
| The run/Super parser and native session catalogue retain explicit program and session identity | `internal/delivery/cli/runtime`, `internal/usecase/session` | `TestProdex04360CyberProgramSurvivesNativeSessionResolution`, `TestProdex04360CyberProgramPassesRunAndSuperWithoutDaybreakOptIn` |
| Explicit `cli_daybreak` opt-in, `daybreak=true/false` ordering, GPT-6.1 Sol and Ultra effort are preserved; none is added by default | Existing run/Super native passthrough and model override planner | `TestProdex04360DaybreakExplicitOrderingIsPreserved`, `TestProdex04360ExplicitSolAndUltraStayUnchanged`, `TestProdex04360SuperWithoutExplicitProgramDoesNotInventDefaults` |
| Invalid program values remain Codex-owned validation, never silently normalized by Godex | Existing native argv forwarding | `TestProdex04360CyberProgramValidationRemainsCodexOwned`, opt-in official-binary smoke below |

**Red/green + adversarial mutation:** Before the production changes,
native ID selection and governed-config placement regressions failed on
all three official programs in multiple positions. Delimiter handling also
failed on a UUID-looking prompt. After the fix, separate intentional
mutations to the native option predicate, governed-config predicate, and
literal separator rule all made their corresponding sentinel fail;
production bytes were restored identically before the focused re-run.

## Exact Codex rust-v0.161.0 compatibility verification

The official `rust-v0.161.0` GitHub release archive
`codex-x86_64-unknown-linux-musl.tar.gz` was downloaded only to a
temporary isolated directory and verified against the SHA-256 recorded by
the exact Prodex release audit:

`b1efb95097660d7f2e5a3887618a23f2ea1b0d548078bf92b0f7a5d229a0cef2`

The extracted official CLI reports `codex-cli 0.161.0`. Its isolated
`features list` output reports `cli_daybreak` as **under development** and
**false (disabled)**. Its help names exactly `standard`, `daybreak_blue`
and `daybreak_red`; all three reach `codex exec review`'s
**unsupported-program validation** before any model turn. The deliberately invalid `daybreak-blue` spelling is rejected by
the official parser. These four cases are guarded by the optional test
`TestProdex04360OfficialCodex0161CyberProgramParseBoundary` (set
`GODEX_TEST_CODEX_0161_BIN` to the verified binary).

Using a temporary synthetic Godex profile and credential-free HOME,
eight actual Godex `run --dry-run` / `s --dry-run` launch plans
(three official values plus one invalid value on each path) were decoded
and handed to that same verified upstream parser; **8/8 passed** with the
expected review/invalid-value validation. No Codex model turn, network
provider call or external account authentication was attempted. The
installed user Codex executable remains **0.160.1** and was not replaced.

## Guarantees explicitly not claimed

1. **Cyber/Daybreak availability on a governed custom provider:** Codex
   restricts these features to its built-in `openai` provider.
   `godex-openai` is an HTTP proxy provider; preserving the CLI option
   cannot override upstream provider eligibility or supply entitlement.
   Daybreak's `cli_daybreak` feature remains opt-in.
2. **Full Prodex runtime recovery parity:** Prodex's tagged automatic
   usage-limit/goal retarget pipeline uses
   `retarget_codex_exec_resume_args`. Godex's session catalogue preserves
   selected native options, but its runtime does not have an independently
   proven 1:1 equivalent of every Prodex automatic retarget/replay path.
3. **Inherited 0.435.9 open contracts:** local-overload pressure policy
   (including fresh Compact shedding), exhaustive multi-provider routing,
   live failures, and the broader runtime/state/Codex boundary corpus
   have not all been verified differentially. Prior 0.435.9 tests and
   CI green status do not automatically close them.
4. **Upstream 0.161.0 cross-OS remote stdio MCP environment limitation:**
   retained as documented in Prodex's exact
   `migration/codex-rust-v0.161.0-audit.md`; Godex cannot repair it by
   rewriting Cyber arguments.

Do not describe passing regression suites or remote GitHub Actions
as an exhaustive parity certificate. Promote the verified incremental
delta to `main` only with clean full/race tests, source-size/static
guards, cross-platform CI and preserved primary WIP.
