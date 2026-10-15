package quota

import (
	"context"
	"errors"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// A real authenticated probe must return the exact request-bound credential
// digest. Plain Fetch, cached quota DTOs and persisted snapshots cannot supply
// this evidence or cause an auth backoff to be retired.
type quotaAuthenticatedGateway043714 interface {
	FetchAuthenticatedAtPolicy(context.Context, string, string, bool) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error)
}

type quotaAuthProofKey043714 struct {
	accountID string
	selection quotamodel.Selection
}
type quotaAuthProof043714 struct {
	receipt      quotamodel.AuthenticatedReceipt
	availability quotamodel.Availability
	observedAt   time.Time
	generation   uint64
}

const maxQuotaAuthProofs043714 = 1024

// ScheduleAuthenticatedRecoveryProbe never performs network I/O inline on the
// request path. It queues at most one refresh for the profile/route key and
// publishes no proof until a real successfully authenticated quota receipt is
// available. Unknown/malformed payloads grant no recovery authority.
func (status *Status) ScheduleAuthenticatedRecoveryProbe(
	ctx context.Context, account accountentity.Account, selection quotamodel.Selection,
) error {
	if status == nil || status.accounts == nil || !account.Enabled || account.ID == "" {
		return errors.New("authenticated quota probe requires an enabled profile")
	}
	gateway, ok := status.usage.(quotaAuthenticatedGateway043714)
	if !ok {
		return errors.New("authenticated quota probe is unavailable")
	}
	home := status.accounts.CodexHome(account.ID)
	if home == "" {
		return errors.New("authenticated quota probe requires a profile home")
	}
	key := quotaAuthProofKey043714{accountID: account.ID, selection: selection}
	// Use a distinct prefix so a regular snapshot refresh cannot suppress
	// revalidation merely because the quota cache is already fresh.
	queueKey := "auth-quota-recovery\x00" + account.ID + "\x00" + home + "\x00" + string(selection.RouteKind) + "\x00" + selection.RequestedModel
	return status.ensureProbeRefreshQueue().Schedule(ctx, queueKey, func(runCtx context.Context) error {
		generation := status.probeSequence.Add(1)
		if err := status.probes.acquire(runCtx); err != nil {
			return err
		}
		usage, receipt, err := gateway.FetchAuthenticatedAtPolicy(runCtx, home, "", false)
		status.probes.release()
		if err != nil {
			return err
		}
		now := status.now()
		if !receipt.Confirmed || receipt.StartedAt.IsZero() || receipt.CredentialDigest == ([32]byte{}) ||
			!hasQuotaWindow(usage.Primary) && !hasQuotaWindow(usage.Secondary) {
			return errors.New("authenticated quota receipt contains no validated quota windows")
		}
		// A late authenticated probe may not override a later status result.
		if !status.applySuccessfulUsageProbe043714(runCtx, account.ID, home, usage, now, generation, true) {
			return nil
		}
		availability := routeAvailability(usage, selection, now)
		availability.Source = quotamodel.SourceLive
		// Publishing is serialized with subsequent quota apply operations;
		// never resurrect a proof after a newer observation was accepted.
		status.probeApplyMu.Lock()
		defer status.probeApplyMu.Unlock()
		status.usageMu.Lock()
		latest := status.probeApplied[home]
		status.usageMu.Unlock()
		if latest != generation {
			return nil
		}
		status.authProofMu.Lock()
		if status.authProofs == nil {
			status.authProofs = make(map[quotaAuthProofKey043714]quotaAuthProof043714)
		}
		if previous, found := status.authProofs[key]; !found || generation > previous.generation {
			if len(status.authProofs) >= maxQuotaAuthProofs043714 {
				// Forgetting an old proof cannot authorize recovery.
				for obsolete := range status.authProofs {
					delete(status.authProofs, obsolete)
					break
				}
			}
			status.authProofs[key] = quotaAuthProof043714{
				receipt: receipt, availability: availability, observedAt: now, generation: generation,
			}
		}
		status.authProofMu.Unlock()
		return nil
	})
}

// AuthenticatedRecoveryReceipt returns only request-verified in-process probe
// evidence, never a cache hit that happened to report ready. Older observations
// expire; a new failure will also fail router-side timestamp checks.
func (status *Status) AuthenticatedRecoveryReceipt(accountID string, selection quotamodel.Selection) (
	quotamodel.Availability, quotamodel.AuthenticatedReceipt, bool,
) {
	if status == nil || accountID == "" {
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	status.authProofMu.Lock()
	proof, found := status.authProofs[quotaAuthProofKey043714{accountID: accountID, selection: selection}]
	status.authProofMu.Unlock()
	now := status.now()
	if !found || now.Before(proof.observedAt) || now.Before(proof.receipt.StartedAt) || now.Sub(proof.observedAt) > usageCacheFreshness ||
		!proof.receipt.Confirmed || proof.receipt.CredentialDigest == ([32]byte{}) {
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	return proof.availability, proof.receipt, true
}
