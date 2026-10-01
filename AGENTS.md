# AGENTS.md

This file applies to the entire repository. A more specific `AGENTS.md` in a
subdirectory may add or tighten rules for that subtree, but it must not weaken
repository-wide architecture, security, compatibility, or quality requirements.

Keep this file operational. Detailed product design belongs in maintained
repository documentation, especially `README.md` and `docs/ARCHITECTURE.md`.

The mandatory Clean Architecture, domain-boundary, helper-placement, and
no-bloat rules in this file are merge requirements, not optional preferences.

## Scope and sources of truth

When repository guidance disagrees, use this order:

1. Security, credential-safety, data-integrity, and compatibility invariants.
2. The mandatory Clean Architecture, domain-boundary, helper-placement, and
   no-bloat rules in this file.
3. Behavior enforced by tests, CI workflows, `Makefile`, and `go.mod`.
4. Maintained architecture and user documentation.
5. Other repository guidance and file-local Go conventions.

When existing documentation conflicts with a mandatory rule in this file,
follow the mandatory rule and update the documentation in the same change.

Do not make a failing check pass by weakening an invariant, deleting meaningful
coverage, bypassing an architecture boundary, or hiding an error.

Repository prose, comments, examples, fixtures, and new documentation must be
written in English. Use synthetic identities, paths, tokens, and account IDs in
tracked content.

## Project summary

Godex is a small Go CLI that works with the official Codex CLI. Its core domain
is isolated `CODEX_HOME` profiles, multiple ChatGPT accounts, deterministic
account selection, safe routing, and cross-platform installation.

The official Codex CLI owns OpenAI authentication and token refresh. Godex owns
profile isolation, local metadata, process orchestration, and account-routing
policy. Keep those responsibilities separate.

Godex is intentionally narrow. Do not copy the size, provider surface,
abstraction graph, or accidental complexity of the project it replaces.

### Mandatory TUI framework

All Godex terminal-interactive UI that corresponds to a Prodex TUI surface MUST
be implemented with `github.com/charmbracelet/bubbletea`. This includes live
dashboards/watchers, interactive lists or pickers, confirmation prompts, login
menus, and password-entry flows. Do not implement equivalent TUI behavior with
ad-hoc ANSI screen clearing, manual terminal event loops, or a second TUI
framework. Plain line-oriented output remains required as a non-TTY/script
fallback where the reference command has one. Companion Charm packages may be
added only when a concrete UI requirement justifies them; Bubble Tea remains the
program/event-loop owner.

## Mandatory Clean Architecture

All production code MUST implement the architecture and data-flow principles
from:

https://github.com/khannedy/golang-clean-architecture

Use that repository as the mandatory architectural baseline. Adopt its layer
ownership and request flow, not every framework, database, configuration format,
or third-party library used by the template. Godex should remain standard-library
first and must not add Fiber, GORM, Viper, Kafka, or similar dependencies unless
a real Godex requirement independently justifies them.

Godex strengthens that baseline with mandatory domain separation. Keep the
reference architecture's layers recognizable, then divide each layer by the
business domain it serves. Do not use a flat layer directory as a catch-all.

A feature is not complete merely because it works. It must also respect the
required layer boundaries below.

### Required data flow

For inbound work:

```text
external input
    -> delivery
    -> model
    -> usecase
    -> entity
    -> repository
    -> persistence
```

For outbound integrations:

```text
usecase
    -> model
    -> gateway
    -> external system
```

For Godex, external input includes CLI arguments, environment variables, stdin,
local proxy requests, and operating-system signals. Persistence includes local
state and isolated profile files. External systems include the official `codex`
process and approved OpenAI-facing transports.

Do not bypass this flow for convenience. In particular:

- delivery must not call a repository or gateway directly;
- repository and gateway packages must not contain use-case orchestration;
- entities must not know about CLI, files, processes, HTTP, or persistence;
- helpers must not become a hidden path around layer ownership;
- `cmd/godex` must not contain business rules.

### Mandatory domain separation

Godex uses a **layer-first, domain-second** production layout:

```text
internal/
    delivery/<transport>/<domain>/
    model/<domain>/
    model/converter/<domain>/
    usecase/<domain>/
    entity/<domain>/
    repository/<domain>/
    gateway/<integration-or-domain>/
```

The top-level directories preserve the Clean Architecture layers. The next
directory names the domain, capability, or external integration that owns the
code. A representative Godex layout is:

```text
internal/
    delivery/cli/account/
    delivery/cli/auth/
    delivery/cli/runtime/
    entity/account/
    entity/routing/
    entity/session/
    model/account/
    model/auth/
    model/proxy/
    usecase/account/
    usecase/auth/
    usecase/routing/
    usecase/runtime/
    repository/account/
    repository/routing/
    gateway/codex/
    gateway/openai/
```

This is an ownership example, not a requirement to create every directory.
Create a package only when the corresponding behavior exists.

Apply these rules to every production change:

- Every business file must belong to exactly one architectural layer and one
  domain. A file that owns several domains must be split before merge.
- Do not place production `.go` files directly in `internal/entity`,
  `internal/model`, `internal/usecase`, or `internal/repository`. Those roots may
  contain only domain directories and an optional package-documentation file.
- `internal/delivery/<transport>` may contain a small dispatcher or transport
  bootstrap, but each command or endpoint's behavior belongs in its domain
  subpackage.
- `internal/gateway` must be split by concrete integration or protocol, such as
  `codex` or `openai`; it must not become one package containing every adapter.
- Use stable capability names such as `account`, `auth`, `routing`, `session`,
  `proxy`, and `runtime`. Do not group unrelated behavior under `core`, `common`,
  `service`, `manager`, `app`, or `misc`.
- A domain does not need every layer. Do not create empty packages, placeholder
  interfaces, or mirrored directory trees for hypothetical future work.
- Name files after the behavior or concept they own, for example `login.go`,
  `remove.go`, `selector.go`, `affinity.go`, or `account.go`. Avoid broad files
  such as `service.go`, `manager.go`, `domain.go`, or `types.go` when a precise
  name is available.
- Keep tests beside the package and domain they verify. Cross-domain integration
  tests may live in a focused integration-test package that names the workflow.
- Existing flat packages must not be used as precedent. When a flat file is
  materially changed, move it into the correct domain package as part of the
  same coherent change when that can be done safely.

Cross-domain behavior must remain explicit:

- A use case may consume another domain's narrow model, entity, or interface
  only when the workflow genuinely spans both domains.
- Do not import another domain's use-case implementation for convenience.
- Do not import another domain's concrete repository or gateway adapter.
- A workflow that coordinates several domains belongs to the domain that owns
  the user-visible outcome. If no domain clearly owns it, create one narrowly
  named orchestration use case; never create a generic coordinator or service
  package.
- Shared business concepts are not helpers. Give a genuinely shared concept its
  own focused domain package with clear semantics and at least two real
  consumers.
- Domain dependencies must be acyclic. A proposed cycle means ownership is
  unclear and must be redesigned, not hidden behind an interface or helper.

### Layer ownership

- `cmd/godex/`: composition root, dependency construction, signal handling, and
  final process exit mapping only.
- `internal/delivery/<transport>/<domain>/`: input parsing, transport-level
  validation, conversion to models, use-case invocation, output formatting, and
  transport error presentation. CLI behavior belongs under
  `internal/delivery/cli/<domain>/`.
- `internal/model/<domain>/`: request, response, event, and gateway data-transfer
  models for one domain.
  Models describe data crossing application boundaries; they do not own domain
  policy, persistence, terminal output, or external I/O.
- `internal/model/converter/<domain>/`: reusable conversions between models and
  entities for one domain when conversion is non-trivial. Keep trivial
  conversions at the call site.
- `internal/usecase/<domain>/`: application workflows, transaction boundaries,
  account selection, retry decisions, affinity decisions, and orchestration for
  one domain.
- `internal/entity/<domain>/`: domain types, identity, validation, invariants,
  and side-effect-free business rules for one domain.
- `internal/repository/<domain>/`: persistence adapters for one domain, including
  local state, locking, atomic writes, and profile filesystem operations.
- `internal/gateway/<integration>/`: contracts and adapters for external
  processes, protocols, and services, including the official Codex CLI.
- `internal/config/`: configuration loading, environment resolution, and
  application wiring support. It must not become a second use-case layer.
- `internal/helper/`: the only shared production-helper root. It must contain
  focused concern-specific subpackages, not a catch-all package.
- `internal/testutil/`: reusable test-only fixtures and helpers that are needed
  by more than one test package.
- `internal/version/`: build and version metadata only.
- `tools/` or `cmd/tools/<name>/`: developer, build, migration, or repository
  maintenance tools. Production packages must not depend on them.
- `docs/`: maintained architecture, contracts, and implementation notes.
- `.github/workflows/`, `.goreleaser.yml`, `install.sh`, and `install.ps1`:
  build, release, and installation surfaces.

Do not invent a new top-level architectural layer without documenting its
ownership, dependency direction, and reason in `docs/ARCHITECTURE.md`.

### Dependency direction

Dependencies must remain explicit and acyclic:

```text
cmd                         -> config, delivery, usecase, repository, gateway
config                      -> standard library and approved config packages
delivery/<transport>/<d>    -> model/<d>, usecase/<d>
usecase/<d>                 -> model/<d>, entity/<d>, consumed ports
repository/<d>              -> entity/<d>
gateway/<integration>       -> relevant model packages
model/converter/<d>         -> model/<d>, entity/<d>
entity/<d>                  -> standard library only
helper/<concern>            -> standard library or approved narrow dependencies
```

Apply these rules:

- `internal/entity/<domain>` must not import another Godex application layer.
- `internal/model/<domain>` must not import delivery, usecase, repository, or
  gateway.
- `internal/usecase/<domain>` depends on narrow repository and gateway contracts, not
  concrete terminal, filesystem, process, or HTTP details.
- `internal/repository/<domain>` must not import delivery, usecase, or gateway.
- `internal/gateway/<integration>` must not import delivery, usecase, or
  repository.
- `internal/delivery/<transport>/<domain>` must not import concrete repository or
  gateway adapters.
- `internal/helper/<concern>` must not import entity, model, delivery, usecase,
  repository, gateway, config, or `cmd`; otherwise it is not a generic helper.
- No package may import `cmd/godex`.
- A domain package must not import another package solely to avoid placing code
  in the correct owner. Imports must reflect real domain collaboration.
- Wire dependencies explicitly at the composition root. Do not add a dependency
  injection framework, service locator, global registry, or reflection-based
  container.
- Prefer private implementation details and a small deliberate API surface.
- Define interfaces at the boundary that consumes them, or in a focused
  repository/gateway contract package when multiple use cases share the same
  contract. Keep interfaces small.
- Add a package or interface only when it creates a durable ownership boundary,
  removes an invalid dependency, or enables meaningful deterministic testing.

Existing violations must not be copied into new code. Code touched by a change
must move toward these boundaries. A large migration may be staged, but every
stage must compile, preserve behavior, and avoid introducing another temporary
architecture.

## Shared helpers, utilities, and tools

Do not create `utils.go`, `helper.go`, `helpers.go`, `common.go`, `misc.go`, or
`tools.go` in arbitrary packages as a dumping ground for reusable code.

Reusable code must be placed according to ownership:

1. Used by only one package: keep it as an unexported function or focused private
   file inside that owning package.
2. Reused by several use cases in one domain and contains business policy: keep
   it in that domain's entity or use-case package. It is not a generic helper.
3. Reused across domains because it represents a real business concept: give it
   a focused domain package with explicit semantics; do not call it `shared`.
4. Reused by two or more production packages and purely technical, independent
   of domain policy: place it under `internal/helper/<specific-concern>/`.
5. Reused only by tests: place it under
   `internal/testutil/<specific-concern>/`.
6. Used for repository development or build automation: place it under `tools/`
   or `cmd/tools/<name>/`.

Examples of acceptable focused helper package paths include
`internal/helper/pathutil`, `internal/helper/fileutil`, and
`internal/helper/jsonutil`. The leaf package name must describe one concrete
technical responsibility. Do not create a broad package named only `utils`,
`common`, `shared`, `helpers`, or `tools`.

The `internal/helper` root must not contain production `.go` files directly.
Every helper belongs in a concern-specific child package. A helper package must
never be used to share business logic between domains or to bypass a layer
boundary.

A shared helper is allowed only when all of the following are true:

- it has at least two real consumers in the current codebase;
- its API is smaller and clearer than duplicating the behavior;
- it is stateless or has explicit ownership and lifecycle;
- it contains no Godex business policy or use-case decisions;
- it does not perform hidden filesystem, process, network, logging, or global
  state work;
- it does not create an import cycle or reverse a Clean Architecture dependency;
- it has focused tests when its behavior is non-trivial.

Do not extract code merely because two snippets look similar. Extract a helper
when the behavior and invariant are genuinely the same. Do not create a helper
for one call site in anticipation of hypothetical reuse.

Converters between transport models and entities belong in
`internal/model/converter/<domain>`, not in a generic helper package. Persistence
helpers that know state-file semantics belong in the owning
`internal/repository/<domain>` package. Codex protocol or process helpers belong
in `internal/gateway/codex`. Business rules belong in the owning entity or
use-case domain package.

## No-bloat policy

Simple, direct, readable code is a hard requirement.

- Prefer the smallest complete implementation that satisfies the current
  requirement and tests.
- Prefer the Go standard library and existing dependencies.
- Do not add speculative provider abstractions, plugin systems, event buses,
  generic repositories, framework wrappers, code generation, reflection,
  inheritance-style base types, or configuration for features that do not exist.
- Do not create `Manager`, `Base`, `Common`, `Factory`, or `Service` abstractions
  that combine unrelated responsibilities. A type name must state what it owns.
- Do not create a single `service.go`, `manager.go`, or `app.go` that accumulates
  all operations for a domain. Split use cases by action and keep any shared
  private policy in a precisely named file.
- Do not add pass-through wrappers that only rename a standard-library or
  dependency call without enforcing an invariant or boundary.
- Do not keep old and new implementations in parallel unless an explicit,
  tested migration or compatibility requirement needs both.
- Remove dead code, obsolete branches, unused configuration, and superseded
  helpers in the same coherent change.
- Keep control flow direct. Prefer early returns and small cohesive functions
  over deep nesting and long switch statements.
- A function should normally stay below 40 non-comment lines. Longer functions
  require a clear single responsibility and should be split when independent
  steps can be named and tested.
- A production file should normally stay below 250 lines. The hard repository
  source-size guard remains 400 lines; approaching that limit requires a clear
  cohesive reason, not merely passing CI.
- Production Go files under `cmd` and `internal` must stay within the repository
  source-size guard, currently 400 lines. Do not weaken or game the guard with
  meaningless file splits.
- One file should own one cohesive concern. Split unrelated types or workflows,
  but do not create dozens of tiny files that make navigation harder.
- One package should own one layer/domain intersection. If its name, files, or
  constructor dependencies reveal several independent capabilities, split the
  package along those capability boundaries instead of adding another manager.
- For non-mechanical changes, keep the total diff reviewable. Complex behavior
  changes should normally remain below 500 changed lines, and other
  non-mechanical changes should remain below 800 changed lines. Split larger
  work into independently compiling, tested, useful stages.
- Every new exported identifier, package, dependency, interface, goroutine, and
  configuration option must justify its maintenance cost.

When reviewing a change, reject it for bloat even if tests pass when a clearly
smaller design provides the same behavior, safety, and maintainability.

## Before changing code

- Read the owning package, its tests, relevant callers, and maintained docs before
  writing code.
- Identify both the owning domain and the correct Clean Architecture layer before
  creating a file or type.
- Reject a location that would put multiple domains into one package merely
  because the code uses the same transport, state file, or external process.
- Search for an existing type, model, converter, helper, contract, or boundary
  before introducing another.
- Decide explicitly whether shared code stays local, belongs to an owning layer,
  or qualifies for `internal/helper` under the rules above.
- Trace shared behavior end to end and fix the root cause instead of patching one
  call site.
- Establish current behavior with the narrowest useful test when practical.
- State material assumptions and make the smallest coherent change.
- Avoid speculative abstractions, premature configurability, drive-by cleanup,
  and unrelated formatting churn.
- Preserve unrelated user changes in the working tree.
- Do not commit, push, tag, publish, or create a release unless explicitly
  authorized.

## Core behavior and safety invariants

### Authentication and profiles

- Delegate interactive and device authentication to the official `codex`
  executable.
- Give every managed account its own isolated `CODEX_HOME`.
- Treat `auth.json` and every bearer, ID, or refresh token as a password.
- Keep credentials inside the owning profile. Godex metadata files must not
  contain tokens.
- Never print tokens, JWT payloads, cookies, or authorization headers in output,
  logs, errors, tests, or fixtures.
- Logging in to the same ChatGPT account must update the existing profile rather
  than create an accidental duplicate.
- Mutating profile and state operations must remain atomic, recoverable, and safe
  under concurrent commands.

### Account selection and routing

- Account selection must be deterministic and bounded.
- Explicit selectors must fail clearly when missing or ambiguous.
- Conversation or turn affinity is stronger than load balancing.
- Retry or rotate only before a downstream response is committed.
- Never replay a request on another account after output or stream data has been
  committed.
- Never rotate in the middle of an established stream.
- Preserve upstream status, headers, payload, and stream framing unless a small,
  documented compatibility transformation is required.
- Keep request and stream hot paths free from avoidable disk I/O, unbounded
  buffering, and unowned background work.

### CLI behavior

- Preserve Codex arguments unless Godex explicitly owns the option.
- Keep normal command output on stdout and diagnostic errors on stderr.
- Preserve meaningful child-process exit status.
- Avoid printing Godex notices while an interactive Codex TUI is running.
- User-visible command, flag, default, help, output, and exit-code changes are
  compatibility changes and require tests and documentation.

## Go conventions

- Run `gofmt`; do not hand-format Go source.
- Prefer the standard library and existing dependencies. A new dependency needs
  a concrete benefit that cannot be achieved clearly with current code.
- Keep package names short, lowercase, and specific. Avoid name stuttering.
- Keep functions focused and control flow direct. Prefer early returns over deep
  nesting.
- Do not create a helper used once unless it names or enforces a real invariant.
- Avoid positional boolean-heavy APIs. Prefer a small input or options struct
  when it makes call sites clearer.
- Accept `context.Context` as the first argument for cancellable operations. Do
  not store contexts in structs.
- Every goroutine must have a clear owner, cancellation path, and completion or
  shutdown strategy.
- Avoid mutable package globals and hidden behavior in `init` functions.
- Return errors from internal packages; do not use `panic`, `log.Fatal`, or
  `os.Exit` for ordinary failures.
- Wrap errors with useful operation context using `%w`, and preserve
  `errors.Is`/`errors.As` behavior.
- Error text should be concise, lowercase, and safe to display without leaking
  credentials.
- Invoke child processes with argument arrays, not shell-concatenated commands.
- Comments should explain contracts, invariants, or surprising decisions rather
  than restating syntax.
- Keep constructors explicit and small. A constructor requiring many unrelated
  collaborators is evidence that the type owns too much.

## Testing

Every behavior change needs observable regression coverage. Every bug fix should
include a test that fails without the fix.

- Prefer tests at the narrowest boundary that proves the behavior.
- Add integration coverage when a change crosses delivery, usecase, repository,
  or gateway boundaries.
- Use table-driven tests for parsers, selectors, classifiers, converters, and
  validation rules when multiple cases share one contract.
- Use `t.TempDir`, `t.Setenv`, fake executables, and `httptest` instead of real
  homes, credentials, or upstream services.
- Standard CI tests must not require a real OpenAI account, live login, network
  access, or cost-bearing model calls.
- Assert public behavior and durable state, not incidental implementation order.
- Keep tests deterministic; do not use sleeps when a controllable clock, signal,
  or synchronization point can express the condition.
- Mark tests parallel only when their environment, filesystem, ports, and global
  process state are isolated.
- Do not add production-only hooks solely to make a test compile. Prefer
  dependency injection at an existing boundary.
- Run race detection for state, locking, process, proxy, or concurrency changes.
- Shared helper packages require their own focused tests when they contain
  branching, parsing, normalization, retries, encoding, or platform behavior.
- Architecture refactors must preserve or improve import direction; do not use a
  helper package to make an invalid dependency compile.

Use the narrowest useful command first, then broaden:

```bash
go test ./internal/<layer>/<domain>
go test ./...
make verify
```

`make verify` is the default completion gate and checks formatting, source size,
vet, race tests, and the build.

When dependencies change, also run:

```bash
go mod tidy
git diff -- go.mod go.sum
```

When release configuration, installers, archive naming, or build metadata
changes, run the relevant syntax checks and a local snapshot:

```bash
make snapshot
```

Report commands exactly as run. A skipped or failed command must not be described
as passing.

## Security and data handling

- Treat CLI input, environment variables, state files, auth files, HTTP input,
  and child-process output as untrusted at their boundaries.
- Validate paths, selectors, persisted versions, and structured input before use.
- Use restrictive permissions for credential-bearing directories and files where
  the platform supports them.
- Redact secrets before constructing logs or user-facing errors.
- Keep proxy listeners on loopback unless a separately reviewed authenticated
  design explicitly permits otherwise.
- Do not weaken TLS verification or silently fall back to insecure transport.
- Bound request metadata, bodies retained for retry, affinity maps, queues,
  attempts, and background work.
- Review changes to dependencies, workflows, installers, and release scripts as
  supply-chain-sensitive changes.

## Compatibility and platform review

Before changing a shared contract, check its effect on:

- CLI commands, flags, defaults, help, output, and exit status;
- `GODEX_HOME`, `GODEX_CODEX_BIN`, and other environment behavior;
- `state.json` versions, account IDs, selectors, and profile layout;
- existing `CODEX_HOME` contents and official Codex CLI behavior;
- request, stream, retry, rotation, and affinity semantics;
- archive names, checksums, installers, and release workflows;
- Linux, macOS, and Windows path, permission, process, signal, and rename
  semantics.

Persisted format changes require an explicit compatibility decision: preserve,
migrate, or reject with a clear version error. Never silently reinterpret old
state.

Use platform-specific files and build tags only for genuine operating-system
behavior. Keep the common contract in shared code and rely on native CI lanes
when local validation cannot cover another platform.

## Documentation and release discipline

- Documentation must describe current behavior, not planned behavior presented as
  complete.
- Update README examples, CLI help, architecture notes, and tests in the same
  change when their contract changes.
- Update `docs/ARCHITECTURE.md` when layer ownership, dependency direction, or
  package placement changes.
- Keep detailed design in `docs/`; do not grow this file into a duplicate
  architecture manual.
- Do not add release notes, bump versions, create tags, or publish artifacts
  without explicit authorization.
- Never claim a feature is production-ready while critical paths contain a
  placeholder, silent fallback, known secret leak, unbounded retry, untested
  compatibility break, Clean Architecture violation, or known bloated design.

## Agent handoff

Lead with the outcome. Then report:

1. Files changed and the behavior they now own.
2. Domain ownership, Clean Architecture placement, and dependency-direction
   decisions.
3. Helper reuse decisions, including why code stayed local or moved under a
   shared helper root.
4. Important compatibility and security decisions.
5. Exact validation commands with pass, fail, or skip status.
6. Remaining risk, assumptions, and follow-up work.
7. Whether any commit, push, tag, or release occurred.

Be precise about incomplete work. Do not hide failed validation, architecture
violations, oversized changes, or imply that a local mock proves a live
authentication or upstream integration path.
