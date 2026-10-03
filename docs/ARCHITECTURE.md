# Godex Architecture

## Goal

Godex is a deliberately small orchestration layer around the official Codex CLI. Codex remains responsible for authentication and token refresh. Godex is responsible for isolated profiles, local routing, safe account rotation, and process lifecycle.

## Context

The behavioral reference project grew into a large multi-provider Rust workspace. Godex does not reproduce that internal design. It preserves the reference behavior through narrow provider gateways and use-case policies while keeping the verified OpenAI/Codex path as the stability baseline.

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
Godex targets the Codex 0.160.0 reminder contract in seconds, while Prodex 0.435.1
still renders its legacy request-count wrapper field. Delivery owns that compatibility
conversion and validates finite nonnegative weights plus signed
TOML integer bounds before launch; percentage defaults remain correct for large
valid token limits. These flag conversions remain private to CLI runtime.



Doctor diagnostics remain a runtime-owned orchestration surface.
`usecase/runtime.Doctor` consumes narrow Activity and quota-report ports, removes
account identifiers from runtime tail events, and returns transport-neutral
diagnostics. CLI delivery owns flag relationships plus human/JSON/bundle
formatting; terminal human panels use Bubble Tea. User-selected bundle files are
persisted through `repository/runtime.DoctorBundleStore`, which enforces a 4 MiB
ceiling, rejects non-regular targets, and writes atomically with owner-only mode.
Repair-journal, full Codex thread-index repair, and policy-suggestion flags fail
closed until their owning subsystems exist; they are never silently accepted.

### Terminal UI ownership

Bubble Tea (`github.com/charmbracelet/bubbletea`) is the mandatory event-loop
framework for every Godex UI that corresponds to a Prodex TUI. TUI models and
rendering stay in the owning `internal/delivery/cli/<domain>` package; business
state still comes from use cases. Live `status`, `quota`, and `log`, the login/provider chooser, human session
lists, doctor panels, plus redeem confirmation use Bubble Tea only when the
required terminal streams are available. Non-TTY paths retain deterministic
line-oriented output. Do not introduce direct ANSI screen-clearing loops or
another TUI framework. Profile export/import protection and masked password entry
follow this rule as well. The login chooser remains a delivery-only selector:
business actions stay in auth/profile use cases and provider gateways, while
guidance-only methods do not mutate state.


OpenAI/API-compatible API-key login follows the same separation. Delivery owns
the Bubble Tea/plain prompts and never forwards the key in child argv. The
profile use case owns name/base-URL validation and create-vs-update policy;
`repository/profile` atomically writes the private Codex-owned `auth.json` and
optional bounded `.prodex-profile.toml`, including rollback and symlink-safe
replacement. Runtime reads only the profile-local URL metadata and injects the
`prodex-openai-compatible` Codex `-c` entries before user arguments, so explicit
user `model_provider` configuration retains final precedence. The Codex process
uses the managed profile's own `auth.json`; Godex's HTTP routing gateway never
handles that API key.


Built-in provider imports use outbound gateways rather than reading external CLI
state from delivery or use cases. `gateway/claude` resolves Claude Code's config
root, reads `.credentials.json` through bounded regular-file checks, and returns
a transport-neutral provider credential model. `usecase/profile` owns identity
deduplication, Prodex-compatible profile naming, activation, and create-vs-update
policy. `repository/profile` owns private provider-secret persistence and rollback
of failed metadata updates. Bundle export/import consumes that same boundary:
Anthropic exports carry `.credentials.json` as a validated provider secret with
empty `auth_json`. Gemini migration bundles similarly carry one schema-validated
`gemini_oauth.json`, empty `auth_json`, and the tagged provider email/project
metadata; persistence still uses the generic private provider-secret boundary,
not the disabled Gemini runtime. Kiro exports follow the same adapter boundary with required
`kiro_auth.json` plus optional `kiro_model_catalog.json`; `gateway/kiro` validates
the nested auth JSON and accepted model-catalog shapes without owning profile
persistence. The same gateway owns built-in Kiro source discovery: it opens the
external `data.sqlite3` read-only through pure-Go SQLite, resolves auth/state with
Prodex key precedence, and runs bounded Kiro metadata commands. It returns only a
transport-neutral credential snapshot plus an optional warning; SQL paths, raw
command execution, and token JSON never escape the gateway. `usecase/profile`
keeps provider-specific identity matching, naming, activation, and create/update
planning outside envelope crypto/persistence, while repository rollback tracks
whether each optional secret existed before replacement. Copilot bundle handling
uses the same provider-metadata path with no provider secret files: host/login/API
and plan metadata are persisted in Godex, while the actual Copilot token remains
owned by the external Copilot config/keychain boundary. `gateway/copilot` owns
built-in Copilot discovery and is the only layer allowed to touch `config.json`,
keytar/libsecret/SDK credential fallbacks, or the authenticated user-info request;
it returns only tokenless provider metadata to `usecase/profile`.
The same gateway owns the Copilot Responses transport and launch-time runtime-auth
resolution; bearer credentials never leave that gateway. `repository/runtime`
owns private bounded runtime/final model-catalog files, while `usecase/runtime`
owns launch-model/config precedence and converts the exact 0.435.1 Copilot catalog
snapshots plus account `/models` metadata into Codex `model_catalog_json`. This
keeps provider HTTP/auth mechanics out of delivery and keeps filesystem policy
out of the gateway.


Copilot model fallback stays inside `gateway/copilot`, before a response returns
to generic routing. The gateway derives the exact 0.435.1 fallback chain from the
request model and buffers only intermediate non-success responses under the same
8 MiB runtime bound used for Copilot auth responses. Structured provider error
codes decide whether another model is legal; auth failures and bare 429s do not
advance the chain. When an intermediate response is not retryable, Godex rebuilds
its original status, headers, trailers, and body before returning it, while
successful/SSE responses remain live and unbuffered. Account rotation and durable
conversation affinity continue to belong exclusively to `usecase/routing`.


Copilot route planning is explicit in `gateway/copilot`: the 0.435.1 supported
surface is Responses, Responses Compact, Chat Completions, Messages, and Models.
The four model-traffic routes share the same auth/header/model-fallback transport;
GET Models list/single is answered locally from the already bounded merged runtime
catalog, so model discovery never creates a second upstream credential path.
Incoming traceparent/tracestate/baggage are the only caller trace headers copied
to Copilot passthrough requests; caller authorization is never forwarded.

Anthropic runtime follows the same account-routing boundary but uses
gateway/claude for managed Claude OAuth. Each routing account owns its own
transport and credential; a runtime pool exposes only accounts whose private
.credentials.json can be resolved. Expired OAuth is refreshed by a bounded
Claude auth-status probe with provider credential environment variables removed.
gateway/chatcompat owns the reusable Responses-to-Chat request transform and Chat
JSON/SSE-to-Responses transform. It contains no credential or routing policy.
entity/provider owns provider error classification and static fallback-chain
policy, so bare 429, structured quota/rate errors, auth failures, model-not-found,
and transient failures are interpreted consistently by Anthropic and Copilot.

Anthropic Responses exhaust legal model fallbacks inside the gateway before a
response returns to usecase/routing; only then may routing rotate credentials.
Chat Completions and Messages remain passthrough and are not model-rewritten.
Kiro runtime remains inside gateway/kiro, which already owns Kiro credential
and catalog parsing. The runtime bridge materializes a private per-profile Kiro
data store, drives the ACP JSON-RPC lifecycle, translates Responses/Chat/Messages,
serves Models locally, and performs semantic Compact without moving ACP or SQLite
details into delivery/use cases. Live Responses/Chat streaming uses a bounded
producer queue and cancels the ACP child when the consumer closes; activity
metadata is normalized/redacted before it becomes text or response metadata.
usecase/runtime owns only provider defaults/catalog launch precedence, while
usecase/profile owns selected-first pool membership and profile leases. A bounded
in-memory conversation store is scoped by profile and handles
previous_response_id/tool-call continuation; it is runtime-only and never
persists conversation content into profile state.

Anthropic raw API-key launches reuse the same routing/account abstraction without
turning secrets into profiles. `gateway/claude` resolves request/environment key
precedence, `usecase/runtime` assigns stable hashed credential IDs and keeps raw
keys only in the invocation-local proxy config, and `gateway/claude.RuntimePool`
binds one transport to each credential ID. `gateway/codex` removes all provider
API-key environment variables from external-provider child processes. This keeps
conversation affinity stable without persisting or logging raw provider secrets.

gateway/compact owns the bounded deterministic local compaction fallback used by
translated providers: at most 24 recent snippets, 768 bytes per snippet, and
24 KiB total summary, with the reference x-prodex-compact-* degradation metadata.
It performs no model call. Anthropic Models responses are local and derive from
the embedded 0.435.1 model IDs/aliases/context/endpoint metadata, while
usecase/runtime owns the Codex launch catalog and user-override precedence.


DeepSeek raw-key runtime follows the same secret-free routing boundary without
reusing the Claude gateway. `gateway/providerkey` owns provider-specific
CLI/environment key precedence; `usecase/runtime` sees only a provider name plus
a bounded list of invocation-local keys and assigns stable hashed routing IDs.
Before proxy construction, `usecase/runtime` resolves the profile-local
`[deepseek]` config (config.toml before compatibility environment values) and
writes the dedicated private DeepSeek Codex catalog; the resolved booleans/modes
are carried as provider metadata rather than rereading files in the HTTP gateway.
`gateway/deepseek` binds one transport to each key ID, owns URL/auth/local
Models/Compact routes, and owns the advanced Responses request translator
(reasoning, primitive controls, strict tools/schema, JSON mode, replay/tool
history). Shared RTK argument shaping stays in `gateway/chatcompat`. The gateway
also owns tagged buffered/SSE response translation and native Anthropic Messages
translation for DeepSeek web search. Native mode switches URL/auth atomically to
`/anthropic/v1/messages` plus `x-api-key`, performs bounded first-event
lookahead, and exposes only transport-neutral precommit failure metadata.
`usecase/routing` owns the resulting credential retry/quarantine decision and
never replays after stream commitment. The HTTP boundary supplies a monotonic
request sequence used only for stable provider-stream item identity.

Gemini raw-key runtime follows the same routing boundary. `gateway/gemini` owns
the API-key pool, Gemini OpenAI-compatible URL/auth behavior, and Responses
request and buffered/SSE response translation. `usecase/routing` selects the
bounded Gemini model chain and decides whether an uncommitted error permits the
next model; `entity/provider` supplies the chain and error classification. Error
bodies retained for that decision are capped at 8 MiB, and a final non-retryable
response keeps its status, headers, trailers, and body. Compact HTTP errors use
the same retry boundary before `gateway/compact` produces its bounded local
fallback; transport errors and invalid summaries go directly to that fallback.
`gateway/gemini` reuses `gateway/chatcompat` for shared message/history
and Chat response conversion, while Gemini-specific reasoning, metadata, tool
shapes, and thought signatures stay in the Gemini gateway. Native Messages and
Embeddings requests retain their paths and use `x-goog-api-key`; Responses and
Chat use the OpenAI-compatible endpoint with Bearer <redacted> The canonical
Gemini model catalog lives in `model/proxy`; `gateway/gemini` serves tagged GET
Models list/single requests locally and leaves non-GET requests on the upstream
route.

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
refreshes every five seconds unless `--once` or `--raw` is selected. Delivery
also owns the 0.435.1 CLI rewrite that turns a quota invocation without
`--profile`/`--raw` into the detailed aggregate view. The quota use case remains
request-scoped and has no background daemon. The profile use case supplies a
credential-free quota target catalog so aggregate `--auth`/`--provider`
filtering can include standalone and non-OpenAI profiles without turning them
into account identities.

For profile-backed quota, `usecase/quota` consumes a narrow Codex
model-provider inspector. `gateway/codex` reads bounded profile-local
`config.toml` data and reports a configured non-OpenAI provider before any
OpenAI usage request is attempted. `gateway/gemini` implements the tagged
disabled-OAuth quota surface for legacy Gemini profiles; it returns migration
guidance without credential or network access.

`gateway/quota` owns virtual providers that do not correspond to managed
profiles. DeepSeek resolves the plural/single API-key environment policy and
queries bounded `/user/balance` JSON; local quota probes the command-scoped
OpenAI-compatible `/models` endpoint; AGY executes the bounded direct
`agy auth quota --format=json --detail --all-accounts` probe. The virtual gateway
runs only for explicit DeepSeek/local/AGY filters, never for `all`, matching
0.435.1. It emits transport-neutral `ExternalInfo`; delivery maps that metadata
into provider-aware account/plan/status/remaining display and sort keys. Managed profile quota uses a narrow provider adapter interface over the
credential-free `QuotaTarget` metadata. `gateway/kiro` implements that interface
from the same bounded managed auth/catalog snapshots used by Kiro import/runtime;
the quota use case never parses Kiro secrets or model catalogs itself.
`gateway/claude` implements the same interface for managed Anthropic profiles:
it reuses the runtime OAuth refresh boundary, exposes only account/auth-method/
expiry metadata to quota, and optionally probes the bounded Anthropic
organization rate-limit endpoint when an admin key is configured. Failed admin
probes degrade to the OAuth-only view; response bodies and credential values do
not cross the gateway boundary. The existing `gateway/quota` AGY command/parser
also implements the managed-profile adapter: profile metadata supplies the
preferred account, so the command omits `--all-accounts` while the virtual
provider path retains it. `gateway/copilot` implements the same profile quota
boundary using the exact host/login account token resolver already used by runtime
metadata; the token exists only for the bounded user-info request, while quota
receives login, plan/access, chat/completions counters, reset date, and readiness.
Managed profile adapters for Gemini/custom-provider quota remain separate outbound
integrations.

The CLI quota delivery package owns watch/once cadence, `--detail`, `--profile`,
`--auth`, `--provider`, and `--base-url` parsing plus rendering of exact UTC
reset timestamps and window lengths. It also owns Bubble Tea watch-only presentation
state: scroll offset, the 0.435.1 report-sort cycle, and provider-filter cycle/lock.
Changing an unlocked filter issues a new quota use-case request with the canonical
provider label; sort and scroll stay purely local and never mutate quota/domain
state. The same delivery boundary owns the `Quota Overview` aggregate because it
is presentation policy over already-normalized reports: OpenAI windows contribute
summed remaining percentages and earliest resets, while external snapshots may
contribute provider-neutral main remaining-percent/reset metadata (currently
Copilot). Provider gateways never import TUI/pool-layout policy. Endpoint override
reaches the OpenAI gateway through a consumed use-case capability and does not
mutate runtime preflight configuration.
Manual reset-credit redemption is a separate quota use case: it resolves only a
quota-compatible OpenAI profile, fetches usage before any side effect, applies
the one-hour confirmation policy, and sends the consume request through the same
Codex-owned auth reader. Delivery owns prompting; the OpenAI gateway owns HTTP
and no-proxy transport policy.


Runtime auto-redeem reuses those same outbound quota/consume gateway operations but
keeps policy in a separate `usecase/quota.AutoRedeemer`. Runtime delivery only
propagates the opt-in flag; runtime preflight preserves exhausted accounts in the
proxy pool only when that flag is enabled. `usecase/routing` decides when the
redeemer may run: same-profile quota failures are attempted before rotation,
fresh whole-pool redemption is considered only after normal selection is
exhausted, hard affinity restricts the candidate to its owner, and request-local
exclusions prevent repeated redemption attempts. The quota use case owns the
0.435.1 credit planner, live-before/live-after quota probes, Spark exclusion,
natural-reset guard, and UUIDv7 idempotency key. The router never consumes a
credit for auth/transient failures or external providers, and delivery never owns
credit policy.
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
internal/delivery/cli/update/
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
internal/model/update/
internal/gateway/codex/
internal/gateway/github/
internal/gateway/update/
internal/gateway/openai/
internal/repository/account/
internal/repository/profile/
internal/repository/routing/
internal/repository/runtime/
internal/repository/session/
internal/repository/update/
internal/usecase/account/
internal/usecase/auth/
internal/usecase/profile/
internal/usecase/ping/
internal/usecase/runtime/
internal/usecase/quota/
internal/usecase/routing/
internal/usecase/session/
internal/usecase/update/
internal/helper/fileutil/
internal/helper/httpheader/
internal/helper/lockfile/
internal/version/
```



Self-update keeps release discovery, state, execution, and presentation separate.
`gateway/github` resolves the latest GitHub redirect with short timeouts;
`repository/update` owns the private five-minute cache and OS locks;
`usecase/update` owns semver/no-downgrade and double-check-under-lock policy;
`gateway/update` probes the real executable and runs the embedded release installer.
The embedded installer is the same root `install.sh`/`install.ps1` shipped with
releases, so self-update does not create a second archive/checksum implementation.
CLI dispatch owns notice eligibility; it calls the non-mutating updater status path
best-effort and deliberately ignores check failures so startup cannot be blocked
by GitHub availability.

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

Local-provider launches deliberately bypass the HTTP routing gateway. The CLI
parses `--url` plus model/context/compact overrides, `usecase/runtime` renders
the bounded `prodex-local` Codex `-c` entries, and the Codex process runs in the
selected/active home directly against that endpoint. This keeps local endpoint
traffic out of quota preflight, managed-account rotation, durable upstream
affinity, and provider credential state while preserving the same Codex home for
rollout/session ownership.

## 1:1 expansion boundary

The verified OpenAI/Codex core remains the stability baseline while Godex expands
toward feature-for-feature Prodex 0.435.1 parity. Remaining multi-provider
bridges, Super, gateway, richer diagnostics, provider runtime bridges, and any
still-missing provider-specific import/bundle surfaces remain implementation backlog
rather than permanent exclusions.
Manual redeem, `ping openai`, explicit self-update, and the best-effort cached
update notice are now implemented reference surfaces. OpenAI/Anthropic/Kiro/Copilot bundle
encoding/decoding is now owned by `repository/profile`: the repository owns private bounded file I/O and
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

Native Antigravity launch policy stays in `usecase/runtime`; its CLI process
adapter lives in `gateway/antigravity`. The same adapter serves global
`agy auth login` through `usecase/auth`. This path owns no Godex profile, quota,
or Gemini API-key proxy state.

## HTTP model request boundary

`delivery/http/proxy` owns the loopback listener, bounded input capture, HTTP
presentation, flushing, trailers, and downstream aborts. It invokes
`usecase/routing` with `model/proxy` request data. Routing owns deterministic
selection, hard affinity, quarantine, retry classification, auth reload decisions,
the opt-in precommit auto-redeem hook, and the precommit attempt cycle. It calls a
consumed transport port;
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
