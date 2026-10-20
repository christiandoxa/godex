#!/usr/bin/env bash
set -euo pipefail

# A release without explicit phase flags still executes the *entire* gate.
# Phases make the same clean-commit, source-verified binary comparisons runnable
# under synchronous tool deadlines. One phase is never a release certificate.
case "${1:-}" in
  '') phase='all' ;;
  --phase=preflight) phase='preflight' ;;
  --phase=preflight-tests) phase='preflight-tests' ;;
  --phase=preflight-oracles) phase='preflight-oracles' ;;
  --phase=preflight-oracles-pty) phase='preflight-oracles-pty' ;;
  --phase=preflight-oracles-profiles) phase='preflight-oracles-profiles' ;;
  --phase=preflight-oracles-history) phase='preflight-oracles-history' ;;
  --phase=deepseek) phase='deepseek' ;;
  --phase=gemini) phase='gemini' ;;
  --phase=websocket) phase='websocket' ;;
  --phase=websocket-core) phase='websocket-core' ;;
  --phase=websocket-deadline) phase='websocket-deadline' ;;
  --phase=websocket-transport) phase='websocket-transport' ;;
  --phase=websocket-transport-core) phase='websocket-transport-core' ;;
  --phase=websocket-transport-lifecycle) phase='websocket-transport-lifecycle' ;;
  --phase=websocket-transport-capacity) phase='websocket-transport-capacity' ;;
  --phase=websocket-compact-lineage) phase='websocket-compact-lineage' ;;
  --phase=http-keepalive) phase='http-keepalive' ;;
  --phase=http-compact) phase='http-compact' ;;
  --phase=http-turn-transport) phase='http-turn-transport' ;;
  --phase=http-session-conflict) phase='http-session-conflict' ;;
  --phase=http-prompt-cache) phase='http-prompt-cache' ;;
  *) echo 'usage: verify-release-parity.sh [--phase=preflight|preflight-tests|preflight-oracles|preflight-oracles-pty|preflight-oracles-profiles|preflight-oracles-history|deepseek|gemini|websocket|websocket-core|websocket-deadline|websocket-transport|websocket-transport-core|websocket-transport-lifecycle|websocket-transport-capacity|websocket-compact-lineage|http-keepalive|http-compact|http-turn-transport|http-session-conflict|http-prompt-cache]' >&2; exit 2 ;;
esac
if [ "$#" -gt 1 ]; then
  echo 'release parity accepts at most one phase option' >&2
  exit 2
fi

# Release-only proof: the public oracle is pinned by commit and binary digest.
# No live provider accounts, real credentials, or external model calls.
ref_commit='42042d765100fc06895a27e0904ffb64842092e6'
prodex_binary_sha256='5efa06fef57b0026eae7ec3cad23ac76b40c3e5d8a3afaee6c20feaeb5737923'
prodex_manifest_sha256='300a8f046041a95f9757e4cbdb72917d37927683b6e1af442492254e2f1a37da'
root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/godex-release-parity.XXXXXXXX")"
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
cd "$root"

git clone --quiet --depth 1 --branch 0.438.3 \
  https://github.com/christiandoxa/prodex.git "$tmp/prodex-source"
test "$(git -C "$tmp/prodex-source" rev-parse HEAD)" = "$ref_commit" || {
  echo "Prodex oracle source does not match 0.438.3" >&2
  exit 1
}

curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/prodex" \
  'https://github.com/christiandoxa/prodex/releases/download/0.438.3/prodex-x86_64-unknown-linux-gnu'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/SHA256SUMS" \
  'https://github.com/christiandoxa/prodex/releases/download/0.438.3/SHA256SUMS'
curl --fail --location --silent --show-error --retry 3 \
  --output "$tmp/release-manifest.json" \
  'https://github.com/christiandoxa/prodex/releases/download/0.438.3/release-manifest.json'
grep -Fqx "$prodex_binary_sha256  prodex-x86_64-unknown-linux-gnu" "$tmp/SHA256SUMS" || {
  echo "Prodex executable is absent or mismatched in published SHA256SUMS" >&2
  exit 1
}
grep -Fq '"version": "0.438.3"' "$tmp/release-manifest.json" &&
grep -Fq '"commit": "42042d765100fc06895a27e0904ffb64842092e6"' "$tmp/release-manifest.json" &&
grep -Fq '"asset": "prodex-x86_64-unknown-linux-gnu"' "$tmp/release-manifest.json" || {
  echo "Prodex release manifest does not describe the pinned oracle" >&2
  exit 1
}
echo "$prodex_binary_sha256  $tmp/prodex" | sha256sum --check --status || {
  echo "Prodex oracle executable SHA-256 mismatch" >&2
  exit 1
}
echo "$prodex_manifest_sha256  $tmp/release-manifest.json" | sha256sum --check --status || {
  echo "Prodex release manifest SHA-256 mismatch" >&2
  exit 1
}
chmod +x "$tmp/prodex"
test "$("$tmp/prodex" --version)" = "prodex 0.438.3"

# Go's build VCS metadata must identify the clean release commit, not WIP.
go build -trimpath -o "$tmp/godex" ./cmd/godex
go build -trimpath -o "$tmp/differential" ./tools/differential
go build -trimpath -o "$tmp/profileparity" ./tools/profileparity
go build -trimpath -o "$tmp/clierrorparity" ./tools/clierrorparity
go build -trimpath -o "$tmp/contextparity" ./tools/contextparity
go build -trimpath -o "$tmp/continuationparity" ./tools/continuationparity
go build -trimpath -o "$tmp/sessionfooterparity" ./tools/sessionfooterparity

if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-tests' ]; then
# Preserve the focused 0.437.10 goal-attachment recovery regressions while
# migrating the executable oracles to 0.437.11. The 0.437.11 session-release
# and full-history rotation regressions are qualified separately.
go test ./internal/gateway/codex -run 'Test(PersistSessionGoal|UpdateSessionGoal|MaintainManagedSessions.*Goal|PersistSessionGoalObjective|RunRuntimeDirectRepairsSelectedGoalBeforeChildStarts|RepairResumeSessionBeforeLaunch|CodexResumeSessionID)' -count=1 -timeout=30s

# Tagged 0.437.11 native-session shutdown must print saved-session resume
# guidance only after the owned socket closes, and restore inherited termios,
# alt-screen, keyboard, mouse, focus and bracketed-paste modes on every exit.
# These real PTY regressions fail closed when no PTY is available and verify
# queued input is never flushed during restoration.
go test ./internal/gateway/codex \
  -run '^TestProdex043711(SuccessfulSessionCompanionShowsResumeGuidanceAfterSocketClosed|SessionCompanionSuppressesGuidanceOnNonTTYAndFailure|ChildTerminalHandoffPreservesQueuedInputAndProtocols|NativeTUIRestoresTerminalAfterSuccessAndFailure|NonInteractiveChildNeverEmitsTerminalProtocols|OwnedCompanionRestoresTTYBeforeResumeGuidance)$' \
  -count=1 -timeout=75s

# Canonical intentional shutdown uses SIGTERM with bounded tree reaping;
# interactive TTY children keep the first SIGINT for native Codex.
go test ./internal/gateway/codex \
  -run '^Test(TerminalCommandCancelsNonTTYChildWithTermination|RunCancellationSendsTermination|RunTerminalCommandPreservesInteractiveFirstInterrupt|RunCancellationReapsNonTTYDescendants)$' \
  -count=1 -timeout=45s

# Optional independently verified official Codex rust-v0.163.0 app-server
# handshake. This does not alter the user's installed codex or start turns.
# Supply the verified test-only executable through the opt-in environment.
if [ -n "${GODEX_TEST_CODEX_0163_APP_SERVER_BIN:-}" ]; then
  go test ./internal/gateway/codex \
    -run '^TestProdex04380OfficialCodex0163AppServerInitializeAndThreadList$' \
    -count=1 -timeout=40s
  go test ./internal/gateway/codex \
    -run '^TestProdex04380OfficialCodex0163Loopback' \
    -count=1 -timeout=50s
fi

# Prodex 0.438.3's compiled Copilot candidate policy caps the raw logged-
# in user list plus preferred lastLoggedInUser at 256 BEFORE deduplication.
# Oversized synthetic imports must not attempt token resolution or HTTP IO.
go test -race ./internal/gateway/copilot \
  -run '^TestProdex04380CopilotCandidate' -count=1 -timeout=35s

# Tagged rich_catalog.mojo retires only three exact OpenAI models with an
# ASCII-case-only comparison after Unicode trim. A Unicode Kelvin `K` must
# never become ASCII k and falsely hide a distinct catalog model ID.
go test -race ./internal/usecase/ping \
  -run '^TestProdex04380(RetiredOpenAIModelOnlyFoldsASCII|ModelPickerRetainsNonASCIILookalikesButHidesRetired|DynamicOpenAIModelIdentityUsesASCIIOnlyFold|ASCIIModelComparisonChecksEveryUTF8Byte|DynamicModelCache|DynamicCatalog)' \
  -count=1 -timeout=35s

# Prodex 0.438.3 clap rejects Super --api-key without --provider as a
# usage error (exit 2), not a runtime error (exit 1). Keep both normal and
# `s` alias paths inside the source-pinned executable CLI differential.
# Positioned Gemini/DeepSeek aliases after leading options are also parsed
# before native Codex input; the literal -- and explicit --provider/--url
# boundary remain authoritative and must not leak aliases into Codex args.
go test ./internal/delivery/cli/runtime ./tools/clierrorparity \
  -run '^TestProdex04380Super' -count=1 -timeout=30s

# Prodex 0.438.3 can select a previously failed authenticated profile
# only after an exhausted transient pool and a successful bounded retry
# wait, preserving eligible backup order and model-specific quota checks.
# Cancellation, absent quota evidence, foreign owner and any original prompt
# replay must not become a new session launch.
go test -race ./internal/delivery/cli/runtime ./internal/usecase/runtime \
  -run '^TestProdex04380(SessionRecoveryFailedProfileFallback|FailedProfileFallbackRequires|NativeRecoveryRequiresModelSpecificQuota)' \
  -count=1 -timeout=65s

# Prodex 0.438.3: no recovery on native Unix SIGINT or Windows
# signed NTSTATUS CONTROL_C_EXIT. Keep the existing 130 policy too.
go test ./internal/delivery/cli/runtime -run '^TestProdex04380' -count=1 -timeout=45s

# Prodex 0.438.3 saved-session output must carry the launch-owned identity,
# preserve CRLF append-only boundaries, offer a picker when unknown, and only
# allocate identity hooks for interactive companion launches.
go test ./internal/gateway/codex \
  -run '^TestProdex043712' \
  -count=1 -timeout=30s

# Prodex 0.438.3 resets DEC 1047 (no DECRC) after native TUI exit,
# and the live runtime only accepts the exact terminal WebSocket idle timeout.
# Both fail-before regressions protect native output and recovery boundaries.
go test ./internal/gateway/codex \
  -run '^TestProdex043713TerminalHandoff' -count=1 -timeout=45s
go test ./internal/delivery/cli/runtime \
  -run '^TestProdex043713WebSocketIdle' -count=1 -timeout=45s

# Prodex 0.438.3 Retry-After text parser uses UInt128 bounds, rejects
# zero/signed values, rounds fractional millis, and caps at five minutes.
go test ./internal/usecase/routing -run '^TestProdex043714RetryAfter' -count=1 -timeout=30s

# Prodex 0.438.3 completed Responses sockets release their profile execution
# permit without closing the reusable transport. Reuse waits for a new
# permit; hidden preludes retain the original precommit deadline, which
# returns a typed local HTTP 504 with no account penalty or duplicate replay.
go test -race ./internal/gateway/openai \
  -run '^TestProdex04382(CompletedIdleWebSocketReleasesPermitAndReuseWaits|HiddenWebSocketPreludeCannotPromoteIdleDeadline|RepeatedHiddenPreludeRespectsOriginalDeadlineAndFreshRequest)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 profile execution capacity can be configured via the
# published PRODEX_RUNTIME_PROXY_PROFILE_INFLIGHT_HARD_LIMIT knob. The
# canonical GODEX alias wins, invalid numbers are rejected, and the parsed
# value must reach the production Router (not just the listener queue).
go test ./internal/usecase/runtime ./cmd/godex \
  -run '^TestProdex04382(ProfileInflightHardLimitEnvironmentMatchesTaggedPolicy|RuntimeProxyLaunchPropagatesProfileInflightHardLimit|RuntimeRoutingPreservesConfiguredProfileInflightHardLimit)$' \
  -count=1 -timeout=30s

# Prodex 0.438.3 websocket/response_tracking/commit.rs restores the real
# configured stream_idle_timeout_ms on a committed upstream WebSocket.
# It must not retain Godex's historic fixed 300s when the published runtime
# stream idle knob requests 250ms/2000ms; precommit retains its own budget.
go test ./internal/gateway/openai ./internal/usecase/runtime ./cmd/godex \
  -run '^TestProdex04382(WebSocketCommittedOutputUsesConfiguredStreamIdle|RuntimeLaunchCarriesConfiguredWebSocketCommittedIdle|RuntimeOpenAITransportRetainsWebSocketStreamIdleSetting)$' \
  -count=1 -timeout=30s

# Tagged Prodex 0.438.3 request-local expiry owns admission too. A canceled
# waiter awakened by a released permit or a context canceled after admission
# bookkeeping must not dispatch a late model turn or leak the weighted slot.
# The gateway deliberately ignores context cancellation: the Router is
# required to enforce expiry before sending, including hard-affinity turns.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04382(ExpiredRequestCannotAcquireFreshProfilePermit|ExpiredDeadlineCannotDispatchBoundOrFreshTurn|CanceledWaiterAwakenedByPermitReleaseCannotDispatch|CancellationDuringPermitAdmissionBlocksUpstreamSend)$' \
  -count=1 -timeout=35s

# The original 0.438.3 deadline also owns each WebSocket retry iteration:
# auth-401 reload, bound-owner transport fallback and a late initial upgrade
# must not send another inference after cancellation/expiry. The synthetic
# upstream ignores context deliberately; Router must enforce no replay.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04382(CanceledWebSocketAuthReloadNeverStartsSecondAttempt|CanceledWebSocketOwnerTransportRetryNeverReplays|AlreadyExpiredWebSocketNeverStartsInitialAttempt|ExpiredWebSocketDeadlineNeverStartsHandshakeOrMessage)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 canceled fresh turns are NOT profile-capacity saturation.
# The admission helper may return (acquired=false, err=context canceled);
# preserve the exact original context error without queuing/retrying it.
# A truly saturated request remains eligible for the existing capacity wait.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04382(ExpiredFreshAttemptIsNotCapacitySaturation|SaturatedFreshAttemptStillReportsOnlyCapacity)$' \
  -count=1 -timeout=35s

# A provider that returns success AFTER the original request's deadline
# cannot deliver uncommitted output. The Router closes its late body and
# releases exactly one weighted permit. A genuinely committed output stream
# has a separate lifetime and must not be torn down by precommit expiry.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04382(LateUncommittedResultAfterCancelCannotBeDelivered|LatePrecommitDeadlineExpiresBeforeGatewayResult|FreshRequestAfterLatePrecommitCancellationStillRuns|CommittedResultRetainsPermitPastOriginalDeadline)$' \
  -count=1 -timeout=35s

# Tagged Prodex 0.438.3 realtime duplex must RETAIN its weighted permit
# while any live binary/audio frames flow. A saturated Responses turn waits
# on the only ready profile and wakes on the realtime session close. This
# is separate from completed-idle Responses socket permit release.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04382RealtimeDuplexRetainsWeightedPermitUntilClose$' \
  -count=1 -timeout=35s

# Shutdown closes admission before releasing active realtime capacity, so a
# saturated follower cannot dispatch a model turn while sockets are torn down.
go test -race ./internal/delivery/http/proxy \
  -run '^TestProdex04382ProxyShutdownStopsAdmissionBeforeReleasingLivePermit$' \
  -count=1 -timeout=35s

# Tagged 0.438.3 composes the original request-local precommit watchdog as
# max(configured stream idle, 30_000 ms). Go must preserve that same budget
# across hidden WebSocket frames and carry the tuning through its real launcher
# to the Router, rather than silently falling back to a fixed five minutes.
go test ./internal/usecase/runtime ./cmd/godex \
  -run '^TestProdex04382(StreamIdleTuningBoundsOriginalPrecommitWatchdog|RuntimeLaunchForwardsStreamIdleWatchdog|RouterReceivesLaunchOwnedPrecommitWatchdog)$' \
  -count=1 -timeout=30s

# The tagged Prodex 0.438.3 precommit hold planner may commit after an 8KiB
# bounded lookahead (without visible model text). Its *shared* commit.rs path
# switches to configured committed idle, just like normal visible output;
# smaller hidden preludes retain the original request precommit deadline.
go test -race ./internal/gateway/openai \
  -run '^TestProdex04382(LookaheadPromotedWebSocketUsesConfiguredCommittedIdle|HiddenWebSocketPreludeCannotPromoteIdleDeadline)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 WebSocket terminal Mojo policy checks nested and top-level
# connection-limit codes independently (ASCII casefold only), gives explicit
# `status` priority over `status_code` even if malformed, and accepts only
# integer u16 statuses. A mistaken fallback can authorize unsafe rotation.
go test ./internal/gateway/openai \
  -run '^TestProdex04380WebSocket' -count=1 -timeout=40s
# Tagged Prodex 0.438.3 retries a first-event connection-limit on a *fresh*
# WebSocket exactly once on the same owner, and never retries committed output.
go test ./internal/gateway/openai \
  -run '^TestProdex04380FreshWebSocket' -count=1 -timeout=40s

# Native Codex 0.163.0 response.incomplete is an incomplete model result,
# not a 429, fake completion, or entitlement to replay. Preserve all wire
# fields and SSE ordering for both buffered and streaming responses.
go test ./internal/delivery/http/proxy \
  -run '^TestProdex04380NativeResponseIncompletePreservesReasonAndUsageWithoutRotation$' \
  -count=1 -timeout=35s
# The same terminal event must feed the existing runtime usage marker. A
# missing marker would silently discard native Codex 0.163.0 usage metadata.
go test ./internal/usecase/routing \
  -run '^TestProdex04380ObserveRecordsNativeIncompleteTokenUsage$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 classifies compaction_trigger as context-dependent, but not
# reconstructable full history after an invalid previous_response_id.
go test -race ./internal/usecase/routing \
  -run '^Test(ResponsesInvalidPreviousIDDoesNotReplayCompactionTriggerHistory|ReconstructableFullHistoryMatchesCodexRequestShapes)$' \
  -count=1 -timeout=35s

# Tagged request-shape kernels compare protocol type/role tags exactly;
# padded tags cannot authorize full-history replay after an invalid ID.
go test -race ./internal/usecase/routing \
  -run '^TestProdex04380(FullHistoryRequiresExactProtocolTags|InvalidPreviousIDDoesNotReplayPaddedCompaction)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 loopback proxy listener survives real Linux RLIMIT_NOFILE
# exhaustion and handles a queued request exactly once after FD release.
# Go's standard net/http accept loop has bounded retry; do not copy tiny_http.
go test ./internal/delivery/http/proxy \
  -run '^TestProdex04380Proxy(ListenerSurvivesActualEMFILEAndQueuedRequest|ShutdownStopsAcceptRetryDuringEMFILE)$' \
  -count=1 -timeout=45s

# Prodex 0.438.3 keeps one request-local watchdog across admission, upstream
# work and the local recovery path. Expired work must return a local 504,
# avoid a second dispatch, and leave a later request usable.
go test ./internal/delivery/http/proxy \
  -run '^TestProdex04381(AdmissionExpiryDoesNotDispatchAndFreshRequestSucceeds|ExpiredAdmissionCannotTakeReleasedPermit|StandardRequestDeadlineCoversUpstreamAndReturns504|BodyCaptureUsesRequestDeadline)$' \
  -count=1 -timeout=35s

# A profile readiness/selection change must wake a waiting recovery attempt
# before the projected cooldown expires; sleeping the old epoch hides ready
# capacity and can turn a recoverable request into a local timeout.
go test ./internal/usecase/routing \
  -run '^TestFreshRecoveryWakeOnSelectionChange$' \
  -count=1 -timeout=20s

# The live observer must distinguish an absent goal from an unavailable DB,
# preserve unread failure evidence under EMFILE, and recover after resources
# return. The Linux low-RLIMIT case runs inside a dedicated child process.
go test ./internal/repository/runtime ./internal/delivery/cli/runtime \
  -run '^TestProdex043713(GoalStoreDistinguishes|UnavailableGoalDatabase|LiveObserverDescriptor|ObserverMarker)' \
  -count=1 -timeout=65s

# Prodex 0.438.3 does not allow delayed older successful quota probes
# to overwrite later confirmed observations and durable quota snapshots.
go test -race ./internal/usecase/quota \
  -run '^TestProdex043714LateSuccessfulProbe' -count=1 -timeout=60s

# Prodex 0.438.3: only the exact successful HTTP request credential
# qualifies to clear an older auth failure. Rotated credentials, newer 401s,
# unknown quota cache, stale observations and unrelated retry leases stay blocked.
go test -race ./internal/gateway/openai ./internal/usecase/quota ./internal/usecase/routing \
  -run '^TestProdex043714(QuotaReceipt|QuotaUnauthorized|QuotaCurrentIdentity|QuotaRecoveryReceipt|FailedUpstreamResponse|AuthenticatedProbe|AuthenticatedQuotaRecovery|NewerQuotaObservation|BlockedAuthRecovery|EmptyAuthPoolWaits)' \
  -count=1 -timeout=65s

# Prodex 0.438.3: newer accepted quota observations refresh the target
# account's routing cache without poisoning other profiles or retry leases.
# Exercise the original three-profile five-hour/weekly and Pro Lite transition
# with a previously auth-quarantined profile, plus exactly-once dispatch.
go test -race ./internal/usecase/quota ./internal/usecase/routing \
  -run '^TestProdex043714(QuotaObservationInvalidates|AcceptedUsageAdvances|ThreeProfileQuotaReset)' \
  -count=1 -timeout=65s

# A successful authenticated result from a credential rotated before apply
# cannot update quota caches, durable snapshots or recovery observations.
go test -race ./internal/usecase/quota \
  -run '^TestProdex043714CredentialRotationBeforeApplyDoesNotStoreStaleQuota$' \
  -count=1 -timeout=35s

# Complete closing requests survive delayed read EOF without a timing guess.
# Ordinary cancellation and incomplete bodies remain negative controls;
# permanent listener failures must retry and still terminate on shutdown.
go test -race ./internal/delivery/http/proxy \
  -run '^Test(Prodex04380CapturedClosingRequestIgnoresDelayedReadEOF|ProxyListenerRetriesPermanentAcceptFailureUntilShutdown|ProxyForwardsCapturedBodyAfterClientHalfClose|ProxyDoesNotForwardIncompleteBodyAfterClientHalfClose|ProxyCancellationStopsUpstream)$' \
  -count=1 -timeout=45s

# Prodex 0.438.3 restart rehydrates authenticated 401/403 penalties even
# with a fresh quota DTO. A new receipt must retire only the exact old auth
# generation, preserving newer 401 and independent generic/transport leases.
# Invalid or public auth-backoff sidecars never silently authorize selection.
go test -race ./internal/repository/routing ./internal/usecase/routing \
  -run '^TestProdex043714Restart' -count=1 -timeout=65s

# A new restart-safe auth store creates only a private empty lock in these
# credential-free external-provider fixtures. Reject data, public modes and
# any unexpected auth sidecar rather than weakening state-integrity checks.
go test ./tools/differential -run '^TestProdex043714DifferentialAuthGuard' \
  -count=1 -timeout=30s

# The tagged 0.437.11 release-fence delta must remain fail-closed under stale
# snapshots, same-second rotation, seven-day retired-fence compaction, backup
# recovery, and invalid inputs. Only self-contained full-history Responses
# requests can repair a conflicting soft session. Native session searching
# must preserve Rust's Unicode lowercase semantics at the Mojo boundary.
# Runtime doctor must observe half-open route circuits without consuming a
# routing probe or mutating the persisted health snapshot.
go test ./internal/repository/routing ./internal/usecase/routing ./internal/usecase/session ./internal/usecase/runtime \
  -run '^TestProdex043711' -count=1 -timeout=60s

# The 0.437.11 Gemini bridge Mojo request kernel gives nested
# request.contents priority over root contents, and only a signature on a
# model part (not its nested functionCall) can suppress first-call hardening.
# Its signature presence check also includes U+001C–U+001F as whitespace.
go test ./internal/gateway/gemini \
  -run '^TestProdex043711GeminiHardening' -count=1 -timeout=40s

# Native compound histories use compiled-source golden cases covering all
# five passes, ID/name matching, nulls, stable ordering and unavailable results.
# This is local native protocol proof, not a two-executable OAuth oracle.
go test -race ./internal/gateway/gemini \
  -run '^TestProdex04380GeminiNative' -count=1 -timeout=35s

# Prodex 0.438.3 DeepSeek streamed tool arguments use tagged serde_json
# surrogate, depth and finite-number boundaries, independent of Go's more
# permissive escape and recursion handling. Verify real SSE terminal frames.
go test ./internal/gateway/deepseek \
  -run '^TestProdex043713DeepSeek' -count=1 -timeout=45s

go test ./internal/gateway/deepseek \
  -run '^TestProdex04380DeepSeek(Responses|InvalidInputIsTerminalBeforeUpstream|RequestBoundUsesNormalizedJSON)' \
  -count=1 -timeout=35s
go test ./tools/differential \
  -run '^TestProdex04380DeepSeekInvalidInput' -count=1 -timeout=30s

# Three managed profiles, authenticated quota probes, precommit owner
# rotation, restart credential revalidation and snapshot-not-auth evidence.
go test ./tools/profileparity \
  -run '^TestProdex04380QuotaRestart' -count=1 -timeout=45s

# The exact pinned executable oracle rejects unsupported typed DeepSeek message
# roles locally with a terminal 502, zero upstream attempts and no retry.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --scenario deepseek-unsupported-message-role

# Prodex 0.438.3 Anthropic ResponseOutput planner caps content at exactly
# 65536 elements, even for no-visible-text thinking blocks whose bounded
# source JSON still fits inside the 4 MiB native response limit.
go test ./internal/gateway/deepseek \
  -run '^TestProdex04380AnthropicResponsePlanRejectsTooManyContentBlocks$' \
  -count=1 -timeout=40s

# Tagged Prodex 0.438.3 Mojo Anthropic RequestKernel enforces JSON recursion
# depth <= 128 for the projected content-block array (not outer metadata).
# Deep tool-use input must fail before producing a synthetic Responses output.
go test ./internal/gateway/deepseek \
  -run '^TestProdex04380Anthropic(ResponseToolInputDepthMustStayBounded|JSONDepthIgnoresEscapedBraces|ResponseDepthIgnoresOuterExtensions)$' \
  -count=1 -timeout=40s

# The 0.438.3 streamed-argument differential compares four exact-tagged
# real SSE tool-call envelopes. Reject extra empty arguments in item.added,
# unstable invalid-JSON diagnostics, bad Unicode/numeric/depth validation,
# failed completion ordering, and any extra/missing stream event.
go test ./tools/differential \
  -run '^TestProdex043713StreamToolDifferentialFixtureRejectsMutations$' \
  -count=1 -timeout=30s

fi

# The original preflight runs BOTH halves; --phase=preflight-tests and
# --phase=preflight-oracles are independently pinned execution partitions,
# not a way to skip the other half or declare a partial release certificate.
if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-oracles' ] ||
   [ "$phase" = 'preflight-oracles-pty' ] || [ "$phase" = 'preflight-oracles-profiles' ] || [ "$phase" = 'preflight-oracles-history' ]; then
# The original preflight-oracles and default full release gate still execute
# every oracle. These mutually exclusive slices permit synchronous verified
# execution without treating a partial slice as a complete release gate.
if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-oracles' ] || [ "$phase" = 'preflight-oracles-pty' ]; then
# Independent Linux PTY footer oracle. It uses one synthetic Codex helper for
# both exact binaries and checks launch-owned identity, picker fallback, and
# all-stream TTY suppression without credentials or provider calls.
"$tmp/sessionfooterparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-oracles' ] || [ "$phase" = 'preflight-oracles-profiles' ]; then
# Independent managed-profile persistence oracle. All 29 CLI actions run in
# separate subprocesses with isolated credential-free homes; state projections
# must match across creation, active switching, rejected writes and deletion.
# The same profileparity binary also runs the 0.438.3 admission queue fixture:
# one held loopback request, one queued request, a released permit, and a
# fresh follow-up must dispatch exactly once for both executables.
"$tmp/profileparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"
# Real Linux two-executable RLIMIT_NOFILE/queued TCP recovery is now an
# additional profileparity stage; mutation sentinels must remain fail-closed.
go test ./tools/profileparity \
  -run '^TestProdex04380GatewayDescriptorPressureRequiresActualFDAndQueuedRecovery$' \
  -count=1 -timeout=30s
# Native Codex 0.163.0 buffered and streamed incomplete terminal frames
# must survive real gateway forwarding unchanged across both executables.
go test ./tools/profileparity \
  -run '^TestProdex04380IncompleteGatewayOracleRejectsResponseAndReplayMutations$' \
  -count=1 -timeout=30s
go test ./tools/profileparity -run '^TestProdex043714Startup' -count=1 -timeout=30s

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-oracles' ] || [ "$phase" = 'preflight-oracles-history' ]; then
# Establish response ownership with real seed requests before checking exact
# history-tag replay decisions against both source-verified executables.
GODEX_TEST_HISTORY_PRODEX_BIN="$tmp/prodex" \
GODEX_TEST_HISTORY_PRODEX_SOURCE="$tmp/prodex-source" \
GODEX_TEST_HISTORY_GODEX_BIN="$tmp/godex" \
GODEX_TEST_HISTORY_GODEX_SOURCE="$root" \
GODEX_TEST_HISTORY_GODEX_COMMIT="$(git rev-parse HEAD)" \
  go test ./tools/continuationparity \
    -run '^TestProdex04380(ExecutableHistoryReplayBoundary|HistoryOracle)' \
    -count=1 -timeout=90s

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'preflight' ] || [ "$phase" = 'preflight-oracles' ] || [ "$phase" = 'preflight-oracles-pty' ]; then
# CLI argument rejection oracle. This catches delivery paths that accidentally
# discard user arguments before reaching their owning command parser.
"$tmp/clierrorparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

# Native Codex context forwarding must match the tagged executable. This
# isolated fake-Codex fixture rejects cached maximum-window promotion and
# checks the child argv rather than only the parent CLI's accepted options.
"$tmp/contextparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)"

# Python standard-library oracle: exact-tagged native Super launches must pass
# current/recent workspace trust through argv without modifying profile config.
python3 tools/workspace-oracle.py \
  --prodex "$tmp/prodex" --prodex-source "$tmp/prodex-source" \
  --godex "$tmp/godex" --godex-source "$root" \
  --godex-commit "$(git rev-parse HEAD)"

fi
fi

if [ "$phase" = 'all' ] || [ "$phase" = 'deepseek' ]; then
# Separate real-provider DeepSeek mock/oracle, still fail-closed on all material
# mismatches including the private durable-file layout.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --scenario all --scenario-workers 3

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'gemini' ]; then
# Gemini API-key Super uses OpenAI-compatible Chat Completions transport,
# but native GenerateContent response semantics. Qualify both the outbound
# request and accepted/invalid native response, and require tagged native
# SSE chunks on that Chat-compatible route not to become false text deltas.
# Genuine Chat Completions SSE chunks must preserve created, item-added,
# text delta, item-done and completed events with exact usage/finish metadata.
# Multi-turn Gemini chat requests must reorder paired tool results ahead of
# later user content and drop unanswered calls, never duplicating an output.
# Gemini plaintext 429 must pass through without rotation (child exit 2),
# while structured quota JSON must exhaust the exact five-model first-key
# chain before the second key's successful initial-model response (exit 0).
# The response UUIDv7 namespace and absent created_at are mandatory.
# Independent route, auth, model, usage, framing, tool-call adjacency,
# key-rotation, exit-code, item ID and response mutation sentinels fail closed.
"$tmp/differential" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --gemini

# Tagged 0.438.3 rejects invalid wrapped/flat Gemini function tools
# *locally*, before any API-key upstream request. Both products must
# return HTTP 400 with the same indexed invalid_request_error JSON envelope,
# exit 2, and zero upstream POSTs; cache/retry/rotation cannot mask 400.
go test -race ./internal/gateway/gemini ./internal/usecase/routing ./internal/delivery/http/proxy ./tools/differential \
  -run '^TestProdex04380(GeminiAPIKeyChat|LocalGeminiTool|GeminiInvalidFunctionTool|GeminiInvalidTool)' \
  -count=1 -timeout=55s

# Prodex 0.438.3 public Gemini API-key Chat-compatible buffered responses
# preserve text around `functionCall:null` and emit fallback `call_1`.
# The separate provider-core internal buffered normalizer suppresses text:
# never confuse that source-only entry point with the actual public wire.
go test -race ./internal/gateway/gemini ./tools/differential \
  -run '^TestProdex04380Gemini(Buffered|APIKeyBuffered|NativeGenerateContent)' \
  -count=1 -timeout=55s

# Tagged 0.438.3 operation-3 role decisions happen on RAW first/last rows
# before rejecting invalid contents. Guard null/empty/model edge cases,
# metadata ownership, custom roles and output ordering before API-key tests.
# Tagged Gemini Mojo operations 1 and 2 synthesize unavailable results for
# unanswered function calls and signature-bearing calls for orphan results.
# ID *and* name must match; duplicates and null values must remain bounded.
# Operation 4 orders immediate tool responses by call ID, ahead of plain
# text/media, without reordering duplicate IDs or synthesizing execution.
# Compound same-role histories use the full tagged order 0/1/2/4/3 and
# exact golden outputs from the independently compiled tagged Mojo kernel.
# Synthetic repair never executes a tool or fabricates a successful result;
# final role shaping must not create an extra model turn or trailing prompt.
go test ./internal/gateway/gemini \
  -run '^TestProdex04380GeminiNative' -count=1 -timeout=35s

# The 0.438.3 Gemini native envelope must unwrap `response` with or
# without traceId. The API-key buffered bridge must generate its own UUIDv7
# and must not expose stale native/trace IDs. These cases are mandatory parts
# of the full --gemini differential above.
go test ./internal/gateway/gemini ./tools/differential \
  -run '^TestProdex043713Gemini' -count=1 -timeout=50s

fi

# A complete WebSocket gate includes BOTH the existing 56-model-attempt + idle
# capacity suite and the new ~30-second per-product deadline differential.
# Mutually exclusive slices fit synchronous command deadlines; the existing
# --phase=websocket and default all mode retain every assertion.
if [ "$phase" = 'all' ] || [ "$phase" = 'websocket' ] || [ "$phase" = 'websocket-core' ]; then
# Exact-tagged Responses WebSocket executable differential. A synthetic
# heartbeat followed by capacity error must remain precommit; actual output
# must never be replayed; trusted previous_response_id outranks soft session.
# The fixture verifies that Godex was built from this clean commit.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket

# The committed Responses body has the SAME full-message progress rule:
# receiving a raw header, split payload or nonfinal continuation is not
# fresh upstream progress. Only the final complete text/binary message or
# complete control Ping/Pong refreshes the committed idle clock; this keeps
# configured timeout and the 0.438.3 queued-terminal readiness invariant.
go test -race ./internal/gateway/openai \
  -run '^TestProdex04383(IncompleteCommittedPayloadCannotRenewProgressDeadline|CompleteCommittedControlFrameCountsAsProgress)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 liveness advances only on a FULL Tungstenite message,
# never raw TCP/TLS bytes from an incomplete header or fragmented precommit
# payload. A peer cannot defeat the upstream progress timeout by trickling
# unusable partial frames. Conversely, a valid complete fragmented message
# or complete queued terminal still advances the original progress clock.
go test -race ./internal/gateway/openai \
  -run '^TestProdex04383(IncompletePrecommitPayloadCannotRenewProgressDeadline|CompletedPrecommitFrameRenewsProgressDeadline|FragmentedMessageAdvancesProgressOnlyAfterFinalContinuation|PartialPrecommitFrameStillTimesOut)$' \
  -count=1 -timeout=35s

# Tagged 0.438.3 polls an upstream socket for already-queued terminal frames
# before an elapsed idle clock can declare the stream dead. Preserve an
# already-buffered completed/failed frame after a slow synchronous commit,
# but still time out a genuinely silent connection (including no active
# reader), without labeling precommit expiry as committed-stream idle.
go test -race ./internal/gateway/openai \
  -run '^TestProdex04383(QueuedTerminalBeatsExpiredCommittedIdle|GenuinelySilentCommittedStreamStillTimesOut|ExpiredQuietSocketWithoutActiveReaderStillTimesOut|ExpiredQuietSocketBeforeCommitIsNotCommittedIdle)$' \
  -count=1 -timeout=35s

# Prodex 0.438.3 sends EMPTY local-client Ping control frames at bounded
# 1000ms intervals while safely holding precommit metadata or waiting on a
# quiet committed Responses stream. Pong is masked client-side and must not
# become model output, renew upstream progress, extend request deadlines or
# authorize any extra inference. The two-executable WebSocket core/deadline
# oracles also require these control frames plus the old no-replay/close rules.
go test -race ./internal/delivery/http/proxy ./tools/continuationparity \
  -run '^TestProdex04383QuietResponsesTransportPingsBeforeCommitWithoutReplay$|^TestProdex04382(WebSocketDeadlineOracleRejectsMaterialMutations|WebSocketCommittedIdleOracleRejectsWireMutations|LookaheadIdleOracleRejectsMutations)$' \
  -count=1 -timeout=40s

# The 0.438.3 local-client Ping is advisory. If a visible model/close
# frame holds the shared writer mutex, the heartbeat must skip this tick;
# blocking behind that lock would deadlock response teardown and retain a
# weighted profile permit after a peer stops reading. The real Ping/Pong
# positive fixture above must continue to pass with no lock contention.
go test -race ./internal/delivery/http/proxy \
  -run '^TestProdex04383Heartbeat(SkipsContendedModelWriter|ShutdownDoesNotWaitForBlockedClientWriter)$' \
  -count=1 -timeout=20s

# Tagged 0.438.3 returns typed precommit connection/read/close failures
# to the outer affinity-aware owner. A failed turn-state-only request with
# reconstructable full history may move to a Ready profile ONLY after
# authenticated owner verification and safe turn-state removal. Delta-only,
# previous-response, strict compact and committed turns never cross owners.
go test -race ./internal/gateway/openai ./internal/usecase/routing \
  -run '^TestProdex04383' -count=1 -timeout=40s

# Prodex 0.438.3 idle-completed Responses sockets must release the weighted
# profile permit yet remain reusable. Five local client WebSockets stay OPEN
# with hard profile capacity=2, then the first is reused without an extra
# upstream handshake. This is a new independent exact-tagged two-executable
# differential; the previous 56-model-attempt fixture does not prove idle release.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-capacity
go test ./tools/continuationparity \
  -run '^TestProdex04382WebSocketCapacityOracleRejectsMaterialMutations$' \
  -count=1 -timeout=30s

# The tagged 0.438.3 committed-stream timeout comes from the effective
# stream_idle_timeout_ms, separate from the 30-second precommit watchdog.
# A visible delta followed by stalled upstream I/O must reach the client,
# then emit the official EMPTY WebSocket close control frame around 2000ms.
# An abrupt TCP EOF is a material observable protocol difference.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-committed-idle
go test ./tools/continuationparity \
  -run '^TestProdex04382WebSocketCommittedIdleOracleRejectsWireMutations$' \
  -count=1 -timeout=30s

# Tagged Prodex 0.438.3 can commit 8KiB of held `response.in_progress`
# prelude even before visible model text. Shared commit.rs MUST switch to the
# configured 2s committed idle, forwarding the full frame then sending an
# EMPTY WebSocket close control frame (no 8s precommit timeout or replay).
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-lookahead-idle
go test ./tools/continuationparity \
  -run '^TestProdex04382LookaheadIdleOracleRejectsMutations$' \
  -count=1 -timeout=30s

# Native 0.438.3 connection-limit frame may be top-level or nested. A fresh
# WebSocket is retryable once before commitment, but never after visible output.
go test ./tools/continuationparity \
  -run '^TestProdex04380FreshConnectionLimitOracleRejectsOwnerAndReplayDrift$' \
  -count=1 -timeout=30s

# Native realtime uses Prodex's registry-advertised duplex sidecar. Background
# usage probes are counted separately from handshakes, controls and model turns.
# A saturated follower waits for client/upstream close or cancellation; shutdown
# cancels it without dispatch. Every case uses one ready/two exhausted profiles.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-realtime
go test -race ./tools/continuationparity \
  -run '^TestProdex04382RealtimeOracleRejectsMutations$' \
  -count=1 -timeout=30s

fi

if [ "$phase" = 'all' ] || [ "$phase" = 'websocket' ] || [ "$phase" = 'websocket-deadline' ]; then
# Tagged Prodex 0.438.3 bounds EVERY precommit upstream read by the ORIGINAL
# 30-second request deadline (max(configured stream idle 2000ms, 30s)).
# Repeated hidden keepalives cease before expiry but leave the old upstream
# open: local client must receive exact 504 request_timeout near 30s, with
# no second inference or auth/quota rotation. Then the SAME local socket must
# send a fresh turn successfully against the same synthetic owner.
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-deadline
# Independent mutations prove this fixture would fail on a renewed/early
# deadline, wrong 504/code/message, replay, owner change or followup loss.
go test ./tools/continuationparity \
  -run '^TestProdex04382WebSocketDeadlineOracleRejectsMaterialMutations$' \
  -count=1 -timeout=30s
fi

if [ "$phase" = 'all' ] || [ "$phase" = 'websocket' ] ||
   [ "$phase" = 'websocket-transport' ] ||
   [ "$phase" = 'websocket-transport-core' ] ||
   [ "$phase" = 'websocket-transport-lifecycle' ] ||
   [ "$phase" = 'websocket-transport-capacity' ]; then
# Managed run admits both authenticated profiles; public gateway fixes its
# pool to the active owner. Check the actual Ready fallback, opaque history,
# stale token removal, exact client frames and durable owner snapshot.
# Synchronous phase slices keep all twenty canonical scenarios. Default,
# native and --phase=websocket-transport still run the complete mode.
transport_mode='--websocket-transport'
if [ "$phase" = 'websocket-transport-core' ]; then
  transport_mode='--websocket-transport-core'
elif [ "$phase" = 'websocket-transport-lifecycle' ]; then
  transport_mode='--websocket-transport-lifecycle'
elif [ "$phase" = 'websocket-transport-capacity' ]; then
  transport_mode='--websocket-transport-capacity'
fi
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  "$transport_mode"
go test -race ./tools/continuationparity \
  -run '^TestProdex04383(Transport|WebSocketTransportSlices)' -count=1 -timeout=40s
fi

# Strict Compact provenance is independent of the quota route used by the
# socket. A verified Compact turn lineage on second must override the active
# main profile, while the unbound negative control must not rotate.
if [ "$phase" = 'all' ] || [ "$phase" = 'websocket-compact-lineage' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --websocket-compact-lineage
go test -race ./tools/continuationparity \
  -run '^TestProdex04383WebSocket(CompactLineageOracleRejectsMutations|ShadowedDirectMetadataOracleRejectsMutations|ToolSuffixOracleRejectsMutations)$' -count=1 -timeout=30s
fi

if [ "$phase" = 'all' ] || [ "$phase" = 'http-keepalive' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --http-keepalive
go test -race ./tools/continuationparity \
  -run '^TestProdex04383HTTPKeepAliveOracleRejectsMutations$' -count=1 -timeout=30s
fi

if [ "$phase" = 'all' ] || [ "$phase" = 'http-compact' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --http-compact
go test -race ./tools/continuationparity \
  -run '^TestProdex04383HTTPCompactOracleRejectsMutations$' -count=1 -timeout=30s
fi

if [ "$phase" = 'all' ] || [ "$phase" = 'http-turn-transport' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --http-turn-transport
go test -race ./tools/continuationparity \
  -run '^TestProdex04383HTTP(ResponsesTurn|RecoverySession|SSEInvalidError|PreviousText)OracleRejectsMutations$' -count=1 -timeout=30s
fi

# HTTP Responses may ignore a deliberately conflicted *soft* session as an
# owner preference. Do not confuse this with hard turn/response conflicts.
if [ "$phase" = 'all' ] || [ "$phase" = 'http-session-conflict' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --http-session-conflict
go test -race ./tools/continuationparity \
  -run '^TestProdex04383(HTTP(SoftSessionConflict|PreviousSession|DirectShadow|NestedPrevious|DirectTurnShadow|UnsupportedThread|ResponseSession|SSETurnLineage)|HTTPTurnSession)OracleRejectsMutations$' -count=1 -timeout=30s
fi

# Native cached-token affinity must prefer the highest verified cache-hit
# owner, without accepting legacy-only, zero-shadowed, or invalid cache usage.
if [ "$phase" = 'all' ] || [ "$phase" = 'http-prompt-cache' ]; then
"$tmp/continuationparity" \
  --prodex "$tmp/prodex" --godex "$tmp/godex" \
  --prodex-source "$tmp/prodex-source" \
  --godex-source "$root" --godex-commit "$(git rev-parse HEAD)" \
  --http-prompt-cache
go test -race ./tools/continuationparity \
  -run '^TestProdex04383HTTPPromptCacheOracleRejectsMutations$' -count=1 -timeout=30s
fi

# Printed only after all stages *selected for this invocation* succeeded.
# --phase=... must never be mistaken for the default complete release gate.
printf 'PASS: Prodex 0.438.3 parity phase %s (Godex %s)\n' "$phase" "$(git rev-parse HEAD)"
