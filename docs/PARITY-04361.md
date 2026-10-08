# Prodex 0.436.1 parity checkpoint

Reference: exact Prodex tag `0.436.1`, commit
`4c61dc0a84a6cf4852feb08e3a8406c1846bb5ad`. This checkpoint records the
behavior implemented in the current Godex worktree; the older
[`PARITY.md`](PARITY.md) document remains the historical 0.435.5 audit.

Closed in this checkpoint:

- DeepSeek Responses error and request shaping follows the tagged provider
  contract, including bounded native fallback behavior.
- Gemini thought-signature precedence, Gemini 3 first-call hardening, and
  streamed signature fields are preserved across native request, buffered
  response, and SSE translation.
- OpenAI 429 header classification, scalar quota errors, and 402/403 workspace
  quota classification preserve retry and pass-through decisions.
- Profile in-flight admission releases on terminal EOF/close, retains capacity
  during `previous_response_not_found` recovery, and keeps weighted limits
  deterministic under concurrent routing.
- Routing restores validated last-good affinity snapshots after restart and
  filters upstream `Content-Length` framing headers.
- Goal-aware runtime recovery, compressed rollout prefix checks, and native
  session recovery remain bounded and cancellation-aware.

Validation completed on the current worktree:

```text
go test ./...
make verify
./scripts/test-installers.sh
bash -n install.sh
make snapshot
(cd dist && sha256sum -c checksums.txt)
```

The snapshot produced cross-platform `0.2.1-next` archives and passed checksum
verification. PowerShell syntax was not run locally because `pwsh` is absent;
the Windows CI lane remains the authoritative check. No live credentials,
provider account, or upstream model call was used.

Known gaps are deliberate: Prodex's state-save/journal/probe background queues
are not invented in Godex, soft-affinity identity migration is not modeled, and
the one-scenario differential harness found unresolved header and durable-state
layout differences. These remain follow-up work rather than parity claims.
