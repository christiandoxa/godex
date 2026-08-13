# Godex

<p align="center">
  <strong>Multiple isolated ChatGPT accounts for the official Codex CLI.</strong>
</p>

<p align="center">
  <a href="https://github.com/christiandoxa/godex/actions/workflows/ci.yml"><img src="https://github.com/christiandoxa/godex/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI" /></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/go-1.26-00ADD8?logo=go&logoColor=white" alt="Go 1.26" /></a>
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
stay with their account, and an established stream is never replayed. The
runtime details are documented in [Runtime rotation and affinity](docs/ROTATION.md).

## Requirements

- The official Codex CLI available as codex.
- Linux, macOS, or Windows on amd64 or arm64 for release binaries.
- Go 1.26 or newer only when building from source.

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

Log in once for each ChatGPT account:

~~~bash
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
godex
~~~

Choose an account explicitly:

~~~bash
godex account use work
godex run --account work -- --model MODEL
~~~

Inspect the installation:

~~~bash
godex doctor
godex --version
~~~

Available account commands:

| Command | Purpose |
| --- | --- |
| godex accounts | List managed accounts. |
| godex account list | Alias for godex accounts. |
| godex account use SELECTOR | Set the preferred account. |
| godex account remove SELECTOR | Remove an account. |

Selectors match an exact account ID, friendly name, or email. Ambiguous
selectors fail. Repeating login for an existing ChatGPT account updates its
profile instead of creating a duplicate.

## Configuration

| Variable | Default | Use |
| --- | --- | --- |
| GODEX_HOME | ~/.godex | State, profiles, locks, and staging files. |
| GODEX_CODEX_BIN | codex | Codex executable to invoke. |
| GODEX_UPSTREAM_URL | https://chatgpt.com/backend-api | Upstream URL for compatible test environments. |

Treat each profile's auth.json like a password. Do not copy it into source
control, backups, bug reports, or fixtures.

## Documentation

- [Architecture](docs/ARCHITECTURE.md) — package ownership and runtime design.
- [Runtime rotation and affinity](docs/ROTATION.md) — selection, retries,
  commitment, streaming, and forwarding rules.
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
