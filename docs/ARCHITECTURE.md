# Godex Architecture

## Goal

Godex is a deliberately small orchestration layer around the official Codex CLI. Codex remains responsible for authentication and token refresh. Godex is responsible for isolated profiles, local routing, safe account rotation, and process lifecycle.

## Context

The behavioral reference project grew into a large multi-provider Rust workspace. Godex does not reproduce that design. It preserves only the OpenAI behaviors needed for a maintainable first release.

## Dependency direction

```text
+------------------+
| cmd/godex        |  composition root
+--------+---------+
         |
         v
+------------------+
| delivery/cli     |  args, terminal I/O, exit codes
+--------+---------+
         |
         v
+------------------+
| usecase          |  login, accounts, launch, proxy orchestration
+---+-----------+--+
    |           |
    v           v
+-------+   +---------+
| repo  |   | gateway |  filesystem / Codex / HTTP adapters
+---+---+   +----+----+
    |            |
    +------+-----+
           v
      +----------+
      | entity   |  pure domain rules and types
      +----------+
```

Dependencies point inward. `main` creates concrete adapters and injects them into use cases.

## Runtime components for v0.1

### Account store

The account store owns:

- versioned `state.json`;
- cross-platform mutation lock;
- atomic writes;
- profile promotion/removal;
- stable account identity and selectors;

It never parses HTTP requests or decides retry policy.

### Codex process gateway

The Codex gateway owns:

- locating the `codex` executable;
- running `codex login` in an isolated `CODEX_HOME`;
- validating the resulting profile;
- launching Codex with a generated local proxy configuration;
- preserving terminal signals and child exit status.

Godex does not implement OAuth or refresh-token exchange.

### Quota gateway and preflight

The OpenAI quota client is a narrow outbound adapter for the ChatGPT usage
endpoint. It reads Codex-owned ChatGPT auth at request time, keeps responses
bounded, and never persists credentials or quota snapshots. The quota use case
classifies ready/exhausted state. Runtime launch code consumes that policy
through a narrow interface, so it does not import the quota use-case package.

Before proxy startup, Godex probes enabled launch candidates once. Confirmed
exhaustion removes an account from fresh-work eligibility for that launch; a
transport or auth probe failure is treated as unknown and remains eligible. The
quota command is intentionally a one-shot OpenAI/Codex view. Live dashboards,
provider-wide quota catalogs, and background quota daemons remain outside
Godex's scope.

The CLI quota delivery package owns `--detail` parsing and rendering of exact
UTC reset timestamps and window lengths from the existing quota models. This
is the bounded one-shot counterpart of Prodex 0.434.2's detailed quota view.
The presentation flag stays local to delivery; account selection and quota
classification remain in `usecase/quota`, and fetching remains in
`gateway/openai`. Formatting helpers stay private to the delivery package
because no other domain consumes this table format. Failed probes expose only
their error state, never gateway error contents.

### Local proxy

The local proxy owns:

- loopback listener lifecycle;
- request parsing only to the minimum required for affinity and retry;
- selecting an account for fresh requests;
- loading current account credentials safely;
- forwarding upstream requests and streams;
- classifying pre-commit failures;
- bounded rotation;
- remembering conversation affinity.

The proxy must not become a generic API gateway.

### Selector and health state

Selection is deterministic round-robin across eligible accounts. Health is intentionally small:

```text
ready
quarantined(until)
disabled
```

A continuation bound by affinity bypasses ordinary load balancing. It either uses its owner or fails explicitly.

### Affinity index

The affinity index maps opaque continuation keys to account IDs:

- previous response ID;
- Codex turn-state token;
- session/conversation ID.

It is in-memory, bounded, expiring, and concurrency-safe. It stores no bearer token or request body.

## Commit boundary

A forwarded request starts as **uncommitted**. Rotation is legal only in this state.

For a normal response, commitment occurs immediately before downstream headers/status are written. For a streaming response, commitment occurs no later than forwarding the first response bytes. After commitment, Godex may report/propagate failure but must not replay the request on another account.

This boundary must be represented in code rather than inferred from scattered boolean checks.

Detailed selection, affinity, retry, commitment, streaming, and forwarding
behavior is documented in [Runtime rotation and affinity](ROTATION.md).

## Login transaction

```text
create staged CODEX_HOME
        |
        v
write minimal config.toml
        |
        v
run `codex login`
        |
        v
validate auth.json + read display identity
        |
        v
lock state
        |
        +--> atomically replace account profile
        |
        +--> atomically replace state.json
        |
        v
unlock and clean backup
```

A failure before the final state write leaves the previously registered profile usable.

## Current v0.1 package map

```text
cmd/godex/
internal/config/
internal/delivery/cli/                 dispatcher only
internal/delivery/cli/account/
internal/delivery/cli/auth/
internal/delivery/cli/runtime/
internal/delivery/cli/quota/
internal/entity/account/
internal/model/account/
internal/model/proxy/
internal/model/quota/
internal/gateway/codex/
internal/gateway/openai/
internal/repository/account/
internal/usecase/account/
internal/usecase/runtime/
internal/usecase/quota/
internal/version/
```

The gateway split follows concrete integration ownership: Codex process and
profile operations live under `gateway/codex`, while the OpenAI-compatible
request proxy lives under `gateway/openai`. Account metadata and persistence
remain separate domain packages. The CLI root only dispatches to command
domains; command parsing and rendering live beside the command they serve.

## Data format

`state.json` is metadata only. Example shape:

```json
{
  "version": 1,
  "active_account_id": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
  "rotation_cursor": 0,
  "accounts": [
    {
      "id": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6",
      "name": "work",
      "email": "person@example.com",
      "chatgpt_account_id": "opaque-account-id",
      "enabled": true,
      "created_at": "2026-08-13T00:00:00Z",
      "updated_at": "2026-08-13T00:00:00Z"
    }
  ]
}
```

No access token, ID token, refresh token, or API token belongs in this file.

## Deliberate exclusions

The first release has no SQL database, browser dashboard, metrics backend, remote daemon, provider abstraction matrix, enterprise policy engine, or plugin runtime. Adding any of these requires a separate product decision after the OpenAI path is stable.

## Managed sessions

`repository/session` reads Codex-owned active and archived rollout metadata and
`session_index.jsonl` thread names. Each profile scan is bounded to 4,096 files,
4 MiB per rollout, 512 KiB per JSON line, and a 16 MiB name index. Symlink roots
are rejected and symlink rollout entries are skipped. Malformed rollouts without
valid UUID metadata are skipped; conversation bodies are never returned.
`entity/session` owns the session identity invariant. `usecase/session` filters,
sorts, resolves unique IDs/prefixes, and preserves profile ownership on resume.
`model/session` carries queries and reports; `delivery/cli/session` owns flags
and text/JSON presentation. These packages follow the repository's layer-first,
domain-second layout. Session helpers remain private to their owning packages.
The composition root injects the runtime launcher through a narrow consumed
interface; session workflows do not import the runtime use-case implementation.
