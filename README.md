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
kept eligible rather than being treated as exhaustion. `godex quota` follows
Prodex 0.435.1's default detailed pool view and five-second live refresh. Without
`--profile` or `--raw`, the command inspects the aggregate pool; `--once`
renders one snapshot and `--raw` performs one raw OpenAI JSON fetch. Provider
filters also expose the 0.435.1 virtual DeepSeek, local OpenAI-compatible, and
Anti-Gravity quota surfaces. The runtime details are documented in [Runtime
rotation and affinity](docs/ROTATION.md).

## Requirements

- The official Codex CLI 0.153.2 or newer available as `codex` (audited compatibility target: Codex 0.160.0).
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

Export or import OpenAI/Anthropic/Kiro/Copilot profile bundles compatible with Prodex `0.435.1`:

~~~bash
PRODEX_PROFILE_EXPORT_PASSWORD=... godex profile export --password-protect profiles.json
PRODEX_PROFILE_IMPORT_PASSWORD=... godex profile import profiles.json
godex profile export --no-password profiles.json
~~~

Encrypted exports use Prodex v2 Argon2id + AES-256-GCM-SIV envelopes; imports
also accept legacy Prodex v1 PBKDF2-SHA256 encrypted envelopes. Plain exports
use the Prodex v1 envelope. Bundle files are private and bounded. In an interactive
terminal, omitting both protection flags opens a Bubble Tea protection prompt;
protected export/import passwords are entered through a masked Bubble Tea prompt
when the matching `PRODEX_PROFILE_*_PASSWORD` environment variable is unset.
Non-TTY workflows remain fail-closed and require explicit flags/environment.

`godex profile import claude` imports an existing Claude Code OAuth credential
from `CLAUDE_CONFIG_DIR` or `~/.claude/.credentials.json`. The source is read as
a bounded regular file, symlinked roots/files are rejected, and the managed copy
is written owner-only. Without `--name`, a matching Anthropic identity updates
the existing profile; otherwise a Prodex-compatible unique `claude-*` name is
created. `--name` forces a distinct profile and `--activate` makes it active.
Anthropic profiles also round-trip through the same plain/encrypted bundle format:
`auth_json` stays empty, provider account/auth-method metadata is preserved, and
the validated `.credentials.json` secret file is carried as the Prodex provider
secret payload. Existing Anthropic profiles update by matching name/provider and
rollback restores the prior provider metadata and secret if a later bundle action
fails. Kiro profiles also round-trip with required `kiro_auth.json` and optional
`kiro_model_catalog.json`; auth/provider metadata is preserved and both secret
files are validated before import/export. Rollback removes an imported optional
catalog when the previous profile did not have one.

`godex profile import kiro` now mirrors Prodex built-in Kiro discovery. It reads
the current Kiro/Amazon Q `data.sqlite3` read-only (honoring `KIRO_DATA_DIR` /
`Q_CLI_DATA_DIR` before the standard local-data locations), selects the same
auth-key priority, derives Kiro provider identity/state, and snapshots
`kiro_auth.json`. Bounded `kiro whoami` and `chat --list-models --format json`
metadata calls enrich identity/catalog data; model-catalog refresh failure is
non-fatal and shown as a warning. Re-import updates the matching
`auth_key + profile ARN/name` identity, and a different `--name` is rejected.
Copilot profiles also round-trip in Prodex's metadata-only bundle form: provider
host/login/API/SKU/plan and profile email are preserved while `auth_json` stays
empty and no token or secret file is bundled. `godex profile import copilot`
now mirrors Prodex's external credential discovery: it parses `COPILOT_HOME` or
`~/.copilot/config.json` (including `//` comments), tries config token, keytar,
libsecret, then Copilot SDK credential backends, and queries the bounded
`/copilot_internal/user` endpoint for provider metadata. Tokens remain external
and are never persisted in Godex profile state or bundle payloads. Identity
matching uses trimmed host + config login, default names use `copilot-<login>`,
and a different `--name` is rejected when that account is already imported.


Copilot model traffic now has a foreground Responses bridge for a selected or
active Copilot profile. Godex resolves the external Copilot token at launch,
prefers direct OAuth `/models` auth with the legacy token exchange as fallback,
forwards `/responses` and `/responses/compact` with Prodex-compatible headers,
canonicalizes Copilot model aliases, strips non-compaction encrypted content,
and detects agent/vision inputs for Copilot request headers. The launch also
writes private bounded Copilot model catalogs: the exact Prodex `0.435.1`
provider/static catalog is merged with account `/models` metadata, per-model
prompt/context limits drive Codex context and auto-compaction budgets, and an
explicit user `model_catalog_json` override always wins.

Copilot runtime now supports a managed multi-profile credential pool for default
or active-profile launches. The selected profile is preferred for the first fresh
request, profiles whose runtime credential cannot be prepared are filtered out,
and the existing routing layer performs bounded pre-commit rotation and durable
conversation affinity. Explicit `--profile` launches remain single-profile hard
affinity. Copilot model selection now also follows Prodex's bounded pre-commit
fallback chains: quota/rate-limit/transient/not-found failures may advance to the
next model, while auth failures, bare 429 responses, and any committed response do
not replay. Launch-time auth resolution already matches Prodex 0.435.1; the
reference does not perform a separate per-request Copilot credential refresh on
this native Responses path. The Copilot endpoint contract now also matches the
0.435.1 registry: `/responses`, `/responses/compact`, `/chat/completions`, and
`/messages` use the provider transport, while GET `/models` and `/models/{id}` are
emulated locally from the merged static/account catalog. Trace context is preserved
for passthrough routes and unsupported Copilot endpoints remain fail-closed.

Managed Anthropic/Claude profiles now have a foreground runtime bridge against the
same local Codex Responses provider boundary. Godex reads each profile's private
Claude OAuth .credentials.json, refreshes an expired token through bounded
claude auth status --json, and never persists the bearer outside the profile
secret. Default or active launches form a selected-first multi-profile pool;
unusable credentials are excluded, explicit --profile stays hard-affinity, and
the generic router retains durable conversation ownership and pre-commit
credential rotation.

For /responses, Godex translates the 0.435.1 lossless Responses subset to
OpenAI Chat Completions, applies Anthropic's model fallback order before rotating
credentials, and translates JSON/SSE output back to Responses. Bare 429 and auth
failures do not advance models. /chat/completions and /messages remain native
passthrough surfaces with Anthropic OAuth headers, GET /models and /models/{id}
are emulated from the verified 0.435.1 Anthropic catalog metadata, and
/responses/compact uses Prodex's bounded local-fallback summary contract without
an upstream model call. Launch catalogs use the 0.435.1 model IDs, aliases, 200k
context window, and 180k default auto-compact limit.

The same bridge now supports Prodex-compatible raw Anthropic API-key launches:
`godex run --provider anthropic` resolves `--api-key`, then
`ANTHROPIC_API_KEYS`, then `ANTHROPIC_API_KEY`; plural keys accept comma,
semicolon, or newline separators and rotate selected-first across fresh requests.
`--base-url` accepts only absolute credential-free HTTP(S) URLs and may override
the Anthropic endpoint. API-key Responses exhaust the model fallback chain before
credential rotation; native `/messages` uses `x-api-key` while chat-compatible
routes use bearer auth, matching 0.435.1. Provider secret environment variables
are removed from the Codex child process and raw keys are never persisted in
profile metadata, routing bindings, or bundles. Without API keys, the same
`--provider anthropic` shortcut resolves managed Claude OAuth profiles.

DeepSeek now has the bounded 0.435.1 raw-key runtime plus the advanced
request-side Responses adapter. `godex run --provider deepseek` resolves
`--api-key`, then `DEEPSEEK_API_KEYS`, then `DEEPSEEK_API_KEY`; plural keys
use Prodex's comma/semicolon/newline parsing and rotate through stable synthetic
routing IDs. The launch path writes the dedicated
`prodex-deepseek-model-catalog.json` used by Prodex (launch model first, then
`auto/pro/flash` and the current DeepSeek model IDs), while later user
`model_catalog_json` overrides still win. The 0.435.1 defaults remain
`deepseek-v4-pro`, `https://api.deepseek.com`, 1,048,576 advertised context
tokens, and a 900,000-token automatic-compaction threshold.

Responses requests now map DeepSeek reasoning effort, primitive sampling/token
controls, stop sequences, logprobs, user IDs, JSON mode, message/tool replay,
local shell/tool calls, named tool choice, and strict function schemas before
`/chat/completions`. `deepseek.strict_tools` is resolved from the active
Codex `config.toml` before `PRODEX_DEEPSEEK_STRICT_TOOLS`, matching Prodex;
invalid strict schemas fail before upstream. Tool replay shares the same RTK
normalization used by chat-compatible responses. `pro -> flash` /
`flash -> pro` model fallback still precedes credential rotation. Chat
Completions and native Messages stay passthrough, Models list/single remains local,
and Responses Compact remains the bounded local fallback with no model call.

DeepSeek-specific response/SSE reasoning/tool shaping and the web-search/beta-base
request routes remain the next 0.435.1 parity slice; unsupported web-search
requests still fail before upstream instead of being silently downgraded.

Local OpenAI-compatible Responses endpoints now match Prodex's `--url` surface.
For example:

~~~bash
godex run --url http://127.0.0.1:8131 --model qwen3-coder exec "review this repository"
godex run --url http://127.0.0.1:8131 \
  --context-window 32768 --auto-compact-token-limit 30000
~~~

A root URL is normalized to `/v1`; an explicit path is preserved with only its
trailing slash removed. Godex generates the same `prodex-local` Codex provider
configuration: Responses wire API, OpenAI-auth requirement, WebSockets disabled,
reasoning summaries disabled, web search disabled, apps/JS REPL/image generation
disabled. The default model is `unsloth/qwen3.5-35b-a3b`, with a 16,384-token
context window and 14,000-token auto-compact threshold. `--local-model`,
`--local-context-window`, and `--local-auto-compact-token-limit` remain aliases
for the standard model/context flags. Local launches use the selected or active
Codex home for session state but run Codex directly against the local endpoint;
they do not enter Godex's quota gate, upstream account rotation, or provider
credential proxy.

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

For the audited Codex 0.160.0 target, Godex keeps the wrapper flag
`--current-time-reminder-interval N` but emits the Codex-native
`reminder_interval_seconds` override. Prodex 0.435.1 still renders its legacy
`reminder_interval_model_requests` wrapper field, so Godex deliberately performs
this compatibility conversion at delivery rather than forwarding the legacy key.
Numeric config values must fit signed TOML integers; token <redacted> must be finite
and nonnegative.

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

Human session lists use Bubble Tea when stdin/stdout are terminals. Short lists
render inline and exit immediately; longer lists use a scrollable alternate screen
with `j/k`, arrow keys, PgUp/PgDn, Home/End, and `q`/Esc/Enter to exit. JSON, ID,
resume-command, and non-TTY outputs remain plain and deterministic.

Inspect the installation:

~~~bash
godex doctor
godex --version
~~~

`doctor` checks the supported Codex version and managed HTTP/SSE configuration
with the same isolated, ten-second capability probe used before launch. An
unsupported runtime fails with an upgrade error. The probe does not log in or
submit a model request. Add `--install` for install checks, `--runtime` for the
bounded runtime summary/tail, and `--quota` for per-profile quota readiness.
`doctor --runtime --json` emits machine-readable diagnostics.
`doctor --bundle [PATH] --redacted` emits a redacted diagnostic bundle; omitting
PATH or using `-` writes it to stdout, while file bundles are written privately
and atomically. `--tail-bytes` defaults to 128 KiB and is capped at 8 MiB.
Repair-import-journal, full session-index repair, and runtime-policy suggestion
flags remain explicit parity gaps rather than no-op switches. Interactive human
doctor panels use Bubble Tea; non-TTY output remains line-oriented.

Available profile, account, and runtime commands:

| Command | Purpose |
| --- | --- |
| godex profile add NAME [--codex-home PATH\|--copy-from PATH\|--copy-current] [--activate] [--insecure] | Add a managed or external Codex profile. |
| godex profile list | List account-backed and standalone profiles. |
| godex profile export [-p NAME]... [--password-protect\|--no-password] [PATH] | Export a Prodex-compatible OpenAI/Anthropic/Kiro/Copilot profile bundle. |
| godex profile import PATH_OR_SOURCE [--name NAME] [--activate] [--insecure] | Import a Prodex-compatible bundle or built-in source such as `claude`. |
| godex current | Show the active profile and its `CODEX_HOME`. |
| godex use NAME | Set the active profile. |
| godex profile remove NAME [--delete-home] | Unregister a profile; managed home deletion is explicit. |
| godex profile import-current [NAME] | Import the ChatGPT login from the current Codex home. |
| godex accounts | List managed ChatGPT account identities. |
| godex account use SELECTOR | Set the preferred account for account rotation. |
| godex account remove SELECTOR | Remove a managed account and its isolated home. |
| godex quota [-p NAME] [--all] [--auth AUTH] [--provider PROVIDER] [--detail] [--raw] [--once] [--base-url URL] | Watch the provider quota pool or inspect one profile/raw OpenAI payload. |
| godex redeem PROFILE [-y|--yes] [--base-url URL] [--no-proxy] | Manually redeem one OpenAI/Codex reset credit. |
| godex ping openai [-p NAME] [--model MODEL] [--base-url URL] [--no-proxy] [--json] | Run the Prodex-compatible OpenAI application diagnostic. |
| godex update | Update the running Godex installation from the latest verified GitHub release. |
| godex info [--json] [--tokens] | Show profile/runtime/Codex information. |
| godex doctor [--install] [--runtime] [--quota] [--json] [--bundle [PATH] --redacted] | Inspect install/runtime/quota health or emit a redacted bundle. |
| godex status [--once] [--interval SECONDS] | Show or watch the runtime snapshot. |
| godex log [stream\|last\|upstream] [--json] | Follow persisted, redacted runtime request events. |

Quota follows Prodex 0.435.1's aggregate default: unless `--profile` or
`--raw` is supplied, Godex enables the all-profile detailed view automatically.
Without `--once`, that view refreshes every five seconds; `--watch` is also
accepted as the explicit hidden spelling. `-p/--profile NAME` selects one
profile, while explicit `--all` keeps the compact aggregate unless `--detail`
is also requested. `--base-url URL` is command-scoped: OpenAI uses it as the
quota backend override, DeepSeek uses it as the API base, and `local` requires it
for the OpenAI-compatible server. `--raw` prints bounded upstream OpenAI usage
JSON for one selected or active profile and cannot be combined with aggregate,
detail, watch, auth, or provider filters. Missing fields, disabled profiles, and
failed probes display conservatively; probe error bodies/secrets are never
rendered.
`--auth` supports Prodex labels such as `chatgpt`, `no-auth`, `api-key`,
`invalid-auth`, `unreadable-auth`, `quota-compatible`, and
`non-quota-compatible`. `--provider` accepts the Prodex 0.435.1 canonical names
plus aliases such as `chatgpt`/`codex` → `openai`, `google` → `gemini`,
`claude` → `anthropic`, `github` → `copilot`, `kiro-cli` → `kiro`,
`openai-compatible` → `local`, and `anti-gravity` → `agy`. Virtual provider
quota matches 0.435.1: `--provider deepseek` reads
`DEEPSEEK_API_KEYS`/ `DEEPSEEK_API_KEY` and queries `/user/balance`;
`--provider local --base-url URL` probes the OpenAI-compatible models endpoint
using `PRODEX_LOCAL_API_KEY` then `OPENAI_API_KEY`; and `--provider agy`
runs bounded `agy auth quota --format=json --detail --all-accounts`. These
virtual providers are intentionally absent from the `all` filter unless that
provider is selected explicitly, matching Prodex. In the all-profile terminal watch, Godex also renders the 0.435.1 `Quota Overview`
pool summary before the profile rows: availability count and last-update time,
plus OpenAI ready/total 5h and weekly remaining pools with earliest reset. When
no OpenAI window data is present, provider snapshots such as Copilot contribute
the generic main remaining pool from their normalized remaining-percent/reset
metadata. One-shot TSV output remains unchanged for scripting.
In an
all-profile terminal watch, the Bubble Tea UI matches the 0.435.1 control state:
`j/k` or arrows scroll, `s` cycles current/remaining/profile/auth/account/plan
sorts, `f` cycles provider filters unless an explicit non-`all` provider locked
the view, and `u` refreshes. Single-profile quota watch remains quit-only.
Configured non-OpenAI profiles remain visible and filterable. The virtual
DeepSeek/local/AGY adapters above are implemented. Imported Kiro profiles also expose profile-backed quota/status metadata from
the managed `kiro_auth.json` and optional `kiro_model_catalog.json`: auth
method, profile/region, imported model count, and readiness are rendered without
a network request. Managed Anthropic profiles now expose the 0.435.1 OAuth quota
view too: the existing OAuth refresh path is reused, account/auth-method/expiry
are reported, and `ANTHROPIC_ADMIN_KEY` (or `ANTHROPIC_ADMIN_API_KEY`) enables
the bounded organization rate-limit summary; an unavailable admin endpoint
degrades back to `Ready (OAuth)` without exposing its response body.
Managed AGY profiles also reuse the same bounded CLI quota adapter but pass the
profile account as the preferred selection and intentionally omit
`--all-accounts`; array output selects the matching account and falls back to the
first row exactly like Prodex. Managed Copilot profiles now expose the 0.435.1
user-quota view through the existing exact-account token resolver: login,
plan/access, chat/completions remaining versus monthly totals, blocked/readiness,
monthly reset date, and minimum remaining percentage are derived without storing
Copilot tokens in Godex. Profile-backed Gemini and custom-provider quota adapters
remain separate parity work.

`godex redeem PROFILE` performs the same explicit two-step manual flow as Prodex:
it checks current usage first, asks for confirmation when the nearest 5-hour or
weekly reset is within one hour, then consumes one reset credit with a stable
idempotency request ID. `--yes` skips only that confirmation; `--no-proxy`
disables environment proxy routing for the usage and consume requests.


Managed OpenAI/Codex runtime launches also support Prodex 0.435.1's
`--auto-redeem` policy. Godex first exhausts normal ready/fallback selection;
quota-blocked profiles may redeem themselves once before rotation, and when the
whole pool is exhausted Godex redeems only after every relevant OpenAI profile has
a current quota snapshot and none still has usable weekly quota. Candidate
selection uses the 0.435.1 plan/reset/order planner, excludes retired Spark models,
refetches quota before consuming a credit, sends a UUIDv7
`prodex-auto-redeem-*` idempotency key, refreshes quota after Reset or
AlreadyRedeemed, and retries only when both 5-hour and weekly windows are usable.
Hard continuation affinity redeems/retries only its owner profile. Missing quota
evidence, natural reset within five minutes, zero credits, non-quota/transient
failures, and non-OpenAI providers never spend a credit. Redemption/retry remains
pre-commit HTTP/SSE behavior; Godex still does not implement WebSocket/Realtime.

`godex ping openai` is intentionally cost-bearing: it submits the minimal `hello`
turn through official Codex for each selected OpenAI profile. It uses a private
diagnostic working directory, a 45-second per-profile timeout, up to four workers,
and strips provider API-key environment variables before launch. Human output
streams profile results as workers finish; `--json` emits one stable aggregate
object. Failure details are bounded and secret-redacted. No ping runs implicitly.

`godex update` resolves the latest stable GitHub release with a five-minute private
cache, takes an exclusive install lock, re-checks the actual running binary under
the lock, and never downgrades a newer local version. When an update is needed,
it runs the installer embedded in the current binary against that executable's
directory. The installer keeps the normal release verification path: archive
download, `checksums.txt` SHA-256 verification, staged `--version` check, then
replacement. Installer output is bounded, control-character filtered, and
secret-redacted. `GODEX_REPOSITORY` and `GODEX_RELEASE_BASE_URL` remain available
for the existing release/mirror workflow.

Like Prodex, normal interactive/operational commands also perform a best-effort
update check using that same five-minute cache. A newer release is announced on
stderr with `godex update`; a failed check never blocks the requested command.
Read-only/minimal surfaces (`info`, `log`, `ping`, `update`, help/version, raw
quota, and JSON/bundle doctor modes) suppress the notice.

Selectors match an exact account ID, friendly name, or email. Ambiguous
selectors fail. Unknown top-level commands are treated as Codex subcommands and
run through the same managed account runtime. Repeating login for an existing ChatGPT account updates its
profile credentials instead of creating a duplicate. Existing sessions, history,
and Codex configuration survive repeat login and import-current.


Interactive terminal surfaces that correspond to Prodex TUIs use
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Current Bubble Tea
surfaces include live `status`, live `quota`, interactive `log stream/upstream`,
human session lists, doctor panels, manual redeem confirmation, and profile
export/import protection/password prompts. Non-TTY and machine-readable modes
keep plain output for scripts and pipes. The remaining login/provider menu TUI
must use the same framework when multi-provider login parity lands.

## Configuration

| Variable | Default | Use |
| --- | --- | --- |
| GODEX_HOME | ~/.godex | State, profiles, locks, and staging files. |
| GODEX_CODEX_BIN | codex | Codex executable to invoke. |
| PRODEX_KIRO_BIN | auto-detect `kiro-cli-chat` / `kiro-cli` | Kiro CLI executable used for built-in import metadata. |
| COPILOT_HOME | ~/.copilot | Copilot CLI config root used by `profile import copilot`. |
| COPILOT_CACHE_HOME | platform cache | Optional Copilot package-cache override for keytar/SDK credential fallback. |
| PRODEX_COPILOT_BIN | copilot | Copilot CLI executable used by the SDK credential fallback. |
| KIRO_DATA_DIR / Q_CLI_DATA_DIR | platform Kiro local-data directory | Optional Kiro source-data override for `profile import kiro`. |
| GODEX_UPSTREAM_URL | https://chatgpt.com/backend-api | Upstream URL for compatible test environments. |
| CODEX_HOME | ~/.codex | Source profile for `profile import-current` and `profile add --copy-current`; managed launches use isolated homes. |
| PRODEX_PROFILE_EXPORT_PASSWORD | unset | Password used by `profile export --password-protect` for Prodex-compatible bundles. |
| PRODEX_PROFILE_IMPORT_PASSWORD | unset | Password used to decrypt encrypted Prodex-compatible profile bundles. |

Treat each profile's auth.json like a password. Do not copy it into source
control, backups, bug reports, or fixtures.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — package ownership and runtime design.
- [Runtime rotation and affinity](docs/ROTATION.md) — selection, retries,
  commitment, streaming, and forwarding rules.
- [Parity audit](docs/PARITY.md) — Prodex 0.435.1 implemented equivalents,
  remaining 1:1 gaps, and validation limits.
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
