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

The Codex gateway keeps argument recognition and config scoping private to
`gateway/codex`. Managed overrides enter the innermost exec command scope;
routing and credential-store bypasses fail before process execution. A bounded
strict-config probe uses a temporary home and `exec-server` with stdio EOF, so
configuration is validated without accepting execution or model work. No shared
helper or new application boundary is needed for this protocol-specific parsing.

Runtime CLI delivery translates supported wrapper features into native config.
Codex 0.159.2's reminder interval uses seconds, unlike Prodex 0.434.2's older
request-count field. Delivery validates finite nonnegative weights and signed
TOML integer bounds before launch; percentage defaults remain correct for large
valid token limits. These flag conversions remain private to CLI runtime.

### Quota gateway and preflight

The OpenAI quota client is a narrow outbound adapter for the ChatGPT usage
endpoint. It reads Codex-owned ChatGPT auth at request time, keeps responses
bounded, and never persists credentials or quota snapshots. The quota use case
classifies ready/exhausted state. Runtime launch code consumes that policy
through a narrow interface, so it does not import the quota use-case package.

Before proxy startup, Godex probes enabled launch candidates once. Confirmed
exhaustion removes an account from fresh-work eligibility for that launch; a
transport or auth probe failure is treated as unknown and remains eligible.
Quota use cases also expose an eligibility deadline; runtime models keep it
separate from account enablement. Routing re-admits the account at the deadline
without a daemon or repeated quota probes. Unknown resets use one minute. The
quota command keeps refresh policy in CLI delivery: Prodex-compatible watch mode
refreshes every five seconds unless `--once` or `--raw` is selected. The quota
use case remains request-scoped and has no background daemon. The profile use case supplies a credential-free quota target catalog so aggregate
`--auth`/`--provider` filtering can include standalone and non-OpenAI profiles
without turning them into account identities. Provider-specific quota adapters
remain part of the 1:1 parity backlog.

The CLI quota delivery package owns watch/once cadence, `--detail`, `--profile`,
`--auth`, `--provider`, and `--base-url` parsing plus rendering of exact UTC
reset timestamps and window lengths. Endpoint override reaches the OpenAI gateway through a consumed use-case
capability and does not mutate runtime preflight configuration.
Manual reset-credit redemption is a separate quota use case: it resolves only a
quota-compatible OpenAI profile, fetches usage before any side effect, applies
the one-hour confirmation policy, and sends the consume request through the same
Codex-owned auth reader. Delivery owns prompting; the OpenAI gateway owns HTTP
and no-proxy transport policy.
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
internal/delivery/cli/profile/
internal/delivery/cli/ping/
internal/delivery/cli/runtime/
internal/delivery/cli/quota/
internal/delivery/cli/session/
internal/delivery/http/proxy/
internal/entity/account/
internal/entity/profile/
internal/entity/routing/
internal/entity/session/
internal/model/account/
internal/model/profile/
internal/model/ping/
internal/model/proxy/
internal/model/quota/
internal/model/runtime/
internal/model/session/
internal/gateway/codex/
internal/gateway/openai/
internal/repository/account/
internal/repository/profile/
internal/repository/routing/
internal/repository/runtime/
internal/repository/session/
internal/usecase/account/
internal/usecase/auth/
internal/usecase/profile/
internal/usecase/ping/
internal/usecase/runtime/
internal/usecase/quota/
internal/usecase/routing/
internal/usecase/session/
internal/helper/fileutil/
internal/helper/httpheader/
internal/helper/lockfile/
internal/version/
```


OpenAI ping follows the same boundary split: `delivery/cli/ping` owns argument and
output formatting, `usecase/ping` owns target selection, bounded concurrency and
status classification, and `gateway/codex` owns the cost-bearing child process.
The gateway runs Codex in a private temporary directory, strips provider secret
environment variables, bounds captured output, and reports cleanup failure.
No diagnostic is scheduled or run implicitly.

The gateway split follows concrete integration ownership: Codex process and
profile operations live under `gateway/codex`, while outbound OpenAI HTTP
transport lives under `gateway/openai`. HTTP delivery lives under
`delivery/http/proxy`; retry and affinity policy lives under `usecase/routing`.
Account identity/rotation and profile lifecycle are separate domains. The profile
repository owns managed/external CODEX_HOME registration and leases; the profile
use case presents account-backed profiles and standalone profiles as one CLI
catalog without moving account routing policy into persistence. Runtime activity
is a separate persisted event stream consumed by info/status/log delivery. The
CLI root only dispatches to command domains; command parsing and rendering live
beside the command they serve.

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

## 1:1 expansion boundary

The verified OpenAI/Codex core remains the stability baseline while Godex expands
toward feature-for-feature Prodex 0.434.2 parity. Multi-provider bridges, Super,
gateway, richer diagnostics, self-update, provider-specific profile bundle
secrets, and built-in imports remain implementation backlog rather than permanent
exclusions. Manual redeem and `ping openai` are now implemented reference surfaces. OpenAI bundle encoding/decoding is now
owned by `repository/profile`: the repository owns private bounded file I/O and
Prodex-compatible envelope crypto, while `usecase/profile` owns profile selection,
identity matching, update/create planning, and rollback. New infrastructure is added only when required by a concrete
reference feature and must still satisfy the architecture rules in `AGENTS.md`.

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

Repeat login and auth-only import update only the existing profile's `auth.json`.
The account repository backs up credentials privately, atomically replaces them,
and restores the backup when the metadata write fails before commitment. Native
configuration, sessions, history, and Codex-owned databases remain in place.
New accounts still promote their complete staged home. No generic file-copy
helper is needed for this account-specific credential transaction.

Profile mutations write a versioned, metadata-only transaction journal before
changing a home or credentials. Reads and mutations recover an interrupted
transaction under the state lock: restore its previous files before metadata
commitment, or finish cleanup after commitment. Native OS file locks serialize
commands and release on process death; the legacy directory lock remains for
compatibility and checks process liveness before reclamation. Profile-use leases
prevent removal or credential replacement while a managed child is running.
Profile lease policy stays local to the account repository; shared OS locking
lives under `helper/lockfile`. Account IDs and backup paths are validated before
recovery; credential contents never enter the journal.

Native status/logout are auth use cases with consumed account and process ports;
CLI auth delivery parses their selectors. Runtime delivery recognizes native
session argument forms and invokes the session use case, which resolves ownership
and preserves the remaining arguments. Native local commands use runtime's local
launch path without proxy or quota orchestration. Explicit runtime selectors
restrict the entire upstream pool. No delivery package accesses an adapter.

## HTTP model request boundary

`delivery/http/proxy` owns the loopback listener, bounded input capture, HTTP
presentation, flushing, trailers, and downstream aborts. It invokes
`usecase/routing` with `model/proxy` request data. Routing owns deterministic
selection, hard affinity, quarantine, retry classification, auth reload decisions,
and the precommit attempt cycle. It calls a consumed transport port;
`gateway/openai` replaces selected-account authentication and forwards upstream
HTTP bytes. Codex authentication reading is injected at the composition root;
the OpenAI adapter does not import the Codex adapter. The quota gateway uses the
same narrow auth port. The stateless `helper/httpheader` package shares RFC
hop-header policy between the two concrete HTTP boundaries, with focused tests.
The sole integration root remains `cmd/godex`.

Fresh-stream startup quota classification belongs to `usecase/routing`; HTTP
delivery still owns commitment, byte forwarding, and incremental ownership
observation. Both reuse `helper/sse` for bounded SSE framing only. It has no
routing policy or I/O, and focused tests cover chunk boundaries, multiline data,
line endings, and recovery after oversized events. Startup failures cross the
boundary through `model/proxy.Forwarded.Failed`, preventing failed attempts from
claiming durable ownership. Read failures during bounded non-stream inspection
propagate before commitment rather than becoming successful truncated bodies.

Responses cannot re-enter routing after delivery commits headers. Truncated
upstream streams abort downstream HTTP using `http.ErrAbortHandler`; they are
not closed as successful chunked responses and are never replayed. Unexpected
WebSocket upgrades are rejected explicitly; HTTP/SSE is Godex's model transport.

## Durable upstream ownership

`repository/routing` stores a bounded, versioned `routing.json` containing only
SHA-256 continuity-key digests, account IDs, key kinds, and timestamps. Routing
use cases persist new bindings before forwarding their associated output bytes.
Known bindings avoid repeated writes, including stream chunks. In-memory expiry
reloads durable ownership; unknown opaque previous-response/turn-state tokens
fail closed. Stable thread/session bindings never expire or get evicted. The
snapshot allows 8,192 total bindings; it evicts old opaque response/turn digests
and refuses new conversations if protected bindings fill the store. Opaque
entries expire after 30 days. Conflicting concurrent ownership updates fail.
Per-conversation guards serialize first requests through downstream completion.

The routing repository also exposes a first-owner OS guard to the routing use
case. Independent processes re-read ownership under that guard before an upstream
attempt and commit the binding before releasing it. The guard is global only for
unbound conversations; established ownership remains cached. The cache loads
requested bindings instead of evicting the requested owner while importing a
larger durable snapshot. OS locking reuses `helper/lockfile`; no new helper or
background worker is needed.

Native `thread-id` identifies durable Codex ownership. Session resolution looks
up its upstream owner independently of the profile holding the rollout. Resume
keeps that rollout home while fixing upstream traffic to its owner, bypassing
fresh-work quota selection. Legacy/imported sessions without a known binding
use their containing profile; unknown opaque continuations still fail closed.
Removed or disabled upstream owners cause a continuity-preserving error.

Runtime delivery locates native commands after root config and value-taking
options. It passes explicit local-session intent through `model/session.Launch`,
so the session use case does not parse CLI command placement. Runtime's picker
launch preserves the active rollout home separately from the upstream pool and
bypasses fresh-work quota selection. Explicit account selection still restricts
the pool. Credential-mutating auth passthrough is rejected at delivery; the auth
use cases remain the owners of registered login and exclusive logout workflows.
Argument helpers stay private to CLI runtime delivery because they describe that
transport's command grammar, not a generic technical concern.

The stateless `helper/lockfile` and `helper/fileutil` packages now have two real
persistence consumers, account and routing, and own only OS locks and durable
private atomic writes. Running Codex children hold shared profile leases;
credential mutation/removal requires an exclusive lease. Concurrent native
children can share a profile without weakening mutation exclusion.

Retained deactivation belongs to the account domain: CLI enable/disable commands
invoke account use cases, and the account repository updates enablement and
repairs deterministic selection under the mutation lock. No home files move.
Existing remove remains destructive; logout remains native credential removal.

Profile transaction journal version 2 includes the previous metadata snapshot.
Recovery distinguishes an unchanged metadata update from a committed change;
normal successful operations explicitly remove the journal. Version 1 journals
remain readable under their original recovery contract. Managed account/home
paths and OS lock paths reject symlinks before credential access or mutation.
