# Godex

<p align="center">
  <strong>Multiple isolated ChatGPT accounts for the official Codex CLI.</strong>
</p>

<p align="center">
  <a href="https://github.com/christiandoxa/godex/actions/workflows/ci.yml"><img src="https://github.com/christiandoxa/godex/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-1.27-00ADD8?logo=go&logoColor=white" alt="Go 1.27" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg" alt="Apache 2.0" /></a>
</p>

## What is Godex?

Godex is a small Go wrapper around the official OpenAI Codex CLI. It gives each
ChatGPT account an isolated Codex home, stores only account metadata in local
state, and launches Codex through a loopback-only managed runtime.

ChatGPT login and token refresh remain owned by Codex. Godex does not implement
OAuth, store credentials in state.json, or require a daemon, database,
dashboard, or remote service.

Fresh requests may rotate between eligible accounts. Existing conversations
stay with their account, and an established stream is never replayed. Managed
launches probe quota once before the local proxy starts: accounts that are
explicitly exhausted are skipped for fresh work, while a failed quota probe is
kept eligible rather than being treated as exhaustion. `godex quota` provides a
bounded one-shot view of OpenAI/Codex 5-hour and weekly usage windows; it intentionally does not add Prodex's live dashboard or multi-provider quota
surface. The runtime details are documented in [Runtime rotation and affinity](docs/ROTATION.md).

## Requirements

- The official Codex CLI 0.153.2 or newer available as `codex`.
- Linux, macOS, or Windows on amd64 or arm64 for release binaries.
- Go 1.27.1 or newer only when building from source.

Check Codex before installing or building Godex:

~~~bash
codex --version
~~~

## Installation

### Linux and macOS

~~~bash
curl -fsSL https://github.com/christiandoxa/godex/releases/latest/download/install.sh | sh
~~~

### Windows PowerShell

~~~powershell
iwr https://github.com/christiandoxa/godex/releases/latest/download/install.ps1 -UseBasicParsing | iex
~~~

The installers select the current operating system and architecture, verify
the archive SHA-256 digest against checksums.txt, install godex, and run
godex --version.

For a pinned version or custom install directory:

~~~bash
GODEX_VERSION=0.1.0 GODEX_INSTALL_DIR="$HOME/.local/bin" \
  curl -fsSL https://github.com/christiandoxa/godex/releases/latest/download/install.sh | sh
~~~

Private mirrors and local fixtures can set GODEX_REPOSITORY and
GODEX_RELEASE_BASE_URL. Set GODEX_VERSION when the mirror does not expose
GitHub release metadata.

## Usage

Import an existing Codex login, or log in once for each ChatGPT account:

~~~bash
godex profile import-current main
godex login --name personal
godex login --name work
~~~

For a headless machine, use Codex device authentication:

~~~bash
godex login --name server --device-auth
~~~

List accounts and launch the normal Codex experience:

~~~bash
godex accounts
godex current
godex quota --all --once
godex quota --all --detail --once
godex
~~~

Choose an account explicitly:

~~~bash
godex account use work
godex run --account work -- --model MODEL
~~~

Prodex-compatible Codex runtime feature flags are translated to Codex `-c`
overrides before the remaining Codex arguments. Explicit later Codex `-c`
arguments still win:

~~~bash
godex run --web-search indexed exec "review this repository"
godex run --rollout-budget-tokens 100000 exec "work on this repository"
godex run --current-time-reminder --respect-system-proxy
~~~

Supported wrapper flags are `--web-search`, `--rollout-budget-tokens`,
`--rollout-budget-reminders`, `--rollout-budget-sampling-weight`,
`--rollout-budget-prefill-weight`, `--current-time-reminder`,
`--current-time-reminder-interval`, `--current-time-clock-source`,
`--respect-system-proxy`, and `--no-respect-system-proxy`. Put them before the
first Codex argument, or use `--` to end Godex option parsing.

With Codex 0.159.2, `--current-time-reminder-interval N` is measured in seconds
and emits `reminder_interval_seconds`. Prodex 0.434.2's older request-count
config field is not accepted by that Codex version. Numeric config values must
fit signed TOML integers; token weights must be finite and nonnegative.

Find sessions across managed profiles, or only sessions for this directory:

~~~bash
godex session list --limit 20
godex session current --parent-only
godex session list --profile work --query repair --json
godex session current --id-only
godex session list --resume-command
godex session resume UNIQUE_ID_PREFIX
~~~

Sessions are sorted newest first. `--cwd PATH` changes the directory matched by
`session current`. `--parent-only` hides spawned subagent sessions; subagents are
included by default. `--json`, `--id-only`, and `--resume-command` are mutually
exclusive. `--limit 0` returns no rows. Missing or ambiguous profile selectors and
session prefixes fail clearly. Resume uses the owning account's isolated home,
even when another account is active. Listing is read-only and does not expose
conversation contents or credentials. Oversized stores fail with a bounded-scan
error; incomplete rollouts without valid metadata are skipped.

Inspect the installation:

~~~bash
godex doctor
godex --version
~~~

`doctor` checks the supported Codex version and managed HTTP/SSE configuration
with the same isolated, ten-second capability probe used before launch. An
unsupported runtime fails with an upgrade error. The probe does not log in or
submit a model request.

Available account commands:

| Command | Purpose |
| --- | --- |
| godex accounts | List managed accounts. |
| godex account list | Alias for godex accounts. |
| godex current | Show the active managed account. |
| godex profile import-current [NAME] | Import the ChatGPT login from the current Codex home. |
| godex quota [--all] [--detail] [--once] [SELECTOR] | Show a one-shot OpenAI/Codex quota snapshot. |
| godex account use SELECTOR | Set the preferred account. |
| godex account remove SELECTOR | Remove an account. |

Quota output stays compact by default. Add `--detail` for Prodex 0.434.2's
expanded quota-window view: `5H_RESET_AT` and `WEEKLY_RESET_AT` contain exact
UTC reset timestamps in RFC 3339 format, while `5H_WINDOW_SECONDS` and
`WEEKLY_WINDOW_SECONDS` contain the upstream window lengths in seconds.
Missing fields, disabled accounts, and failed probes display `-` for unavailable
quota values. Probe failures retain the `error` state without printing error
contents. The command remains one-shot, with or without `--once`. Without
`--all` or a selector, it shows the active account.

Selectors match an exact account ID, friendly name, or email. Ambiguous
selectors fail. Unknown top-level commands are treated as Codex subcommands and
run through the same managed account runtime. Repeating login for an existing ChatGPT account updates its
profile credentials instead of creating a duplicate. Existing sessions, history,
and Codex configuration survive repeat login and import-current.

## Configuration

| Variable | Default | Use |
| --- | --- | --- |
| GODEX_HOME | ~/.godex | State, profiles, locks, and staging files. |
| GODEX_CODEX_BIN | codex | Codex executable to invoke. |
| GODEX_UPSTREAM_URL | https://chatgpt.com/backend-api | Upstream URL for compatible test environments. |
| CODEX_HOME | ~/.codex | Source profile used only by `godex profile import-current`; managed launches use isolated homes. |

Treat each profile's auth.json like a password. Do not copy it into source
control, backups, bug reports, or fixtures.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — package ownership and runtime design.
- [Runtime rotation and affinity](docs/ROTATION.md) — selection, retries,
  commitment, streaming, and forwarding rules.
- [Core parity audit](docs/PARITY.md) — Prodex 0.434.2 equivalents, deliberate
  differences, and validation limits.
- [AGENTS.md](AGENTS.md) — engineering invariants for contributors.

## Build from source

~~~bash
go build -trimpath -o ./bin/godex ./cmd/godex
./bin/godex --version
~~~

The repository verification gate is:

~~~bash
make verify
~~~

## License

Apache-2.0. See [LICENSE](LICENSE).

Managed profiles are pinned while Codex runs. Login updates or removal of a
profile in use fail clearly. Interrupted profile transactions recover on the next
account read or mutation. Do not run older Godex versions concurrently: earlier
versions used time-only lock reclamation and cannot enforce profile-use leases.

`godex login status [--account SELECTOR]` and `godex logout [--account SELECTOR]`
use the active account by default and run native Codex without quota preflight
or rotation. Logout retains the managed account, settings, and sessions.
`--account` fixes both the launch home and the runtime account pool.
Explicit UUIDs and hexadecimal prefixes of at least four characters in native
`resume`, `exec resume`, `fork`, `exec fork`, `delete`, `archive`, and `unarchive` commands resolve
across managed profiles. A conflicting explicit account fails. A bare full UUID
is shorthand for `session resume`. Native names, `--last`, and interactive pickers
remain scoped to the active or explicitly selected profile. Local native commands
such as `mcp`, `features`, and `completion` bypass quota and rotation.

`queue --thread UUID_OR_PREFIX --message TEXT` also resolves the owning profile;
the `--thread=VALUE` form is preserved. Named queue targets stay profile-local.
Native `debug app-server`, `app-server daemon`, and `app-server proxy` are rejected
because they discard managed configuration or escape the profile lease and proxy
lifetime. Use `godex exec` or a foreground `godex app-server` for managed work.

Native command recognition preserves root options and wrapper-generated config.
Picker/name/`--last` resumes keep the selected rollout home and retain the enabled
upstream owner pool, without fresh-work quota selection. An explicit `--account`
still restricts that pool. Passthrough `login` mutations and `logout` are rejected
with guidance to use `godex login`/`godex logout`, whose managed workflows enforce
identity registration and exclusive credential mutation. `login status` stays
read-only. Local session deletion/archive commands also bypass quota and routing.

Model traffic uses an explicit HTTP/SSE OpenAI Responses configuration; native
Codex account/bootstrap and authentication endpoints retain their normal HTTPS
transport. Godex rejects unexpected WebSocket upgrades and routing/auth-store
config overrides. It supplies both the chosen bearer credential and its ChatGPT
account routing header. A broken committed stream fails downstream HTTP and is
never replayed on another account.

Managed config is applied in the innermost `exec`, `exec resume`, `exec fork`,
or `exec review` scope. Other Codex overrides retain their order and precedence.
Routing and credential-store overrides, including quoted keys and whole provider
tables, `--oss`/`--local-provider`, and remote app-server routing flags are rejected before the Codex child starts.
Arguments after Codex's `--` delimiter remain literal. Launch capability checks
validate strict config through `exec-server --listen stdio` with closed stdin
and an empty temporary home; they do not submit a model request.

Upstream conversation ownership survives process restarts in a bounded
`routing.json` containing hashes and account metadata, never raw continuation
secrets or credentials. A resumed session keeps its rollout home and uses the
account that served it, even if the first request rotated accounts. Unknown
opaque continuations fail clearly. Native/imported sessions without an existing
binding use their containing profile. Stable ownership is retained; the store
refuses new conversations if its 8,192 protected-binding ceiling is reached.
Looking up an older durable owner does not depend on the 4,096-entry cache.
Independent managed processes serialize first-owner selection until the binding
is committed, then release the OS guard before forwarding the response stream.

Launch quota exhaustion is temporary: accounts become eligible for a fresh
upstream attempt at their observed reset deadline. When the reset is unknown,
Godex retries eligibility after one minute. Upstream quota responses still
quarantine the account before output commitment. Hard conversation ownership
bypasses fresh-work quota selection and is never rotated at a quota reset.

`godex account disable SELECTOR` retains credentials, configuration, and sessions
while excluding the account from new launches and routing. `godex account enable
SELECTOR` restores eligibility. Both fail if Codex currently uses the profile.
Use disable for retained deactivation, logout to remove only credentials, and
`account remove` to delete the entire managed profile and its native state.
Godex deliberately keeps the existing destructive remove contract; it does not
silently change the default or add a second archive tree.
