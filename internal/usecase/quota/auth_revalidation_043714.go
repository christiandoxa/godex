package quota

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// A real authenticated probe must return the exact request-bound credential
// digest. Plain Fetch, cached quota DTOs and persisted snapshots cannot supply
// this evidence or cause an auth backoff to be retired.
type quotaAuthenticatedGateway043714 interface {
	FetchAuthenticatedAtPolicy(context.Context, string, string, bool) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error)
	CurrentCredentialDigest(context.Context, string) ([32]byte, error)
}

type quotaAuthProofKey043714 struct {
	accountID    string
	selection    quotamodel.Selection
	anySelection bool
}
type quotaAuthProof043714 struct {
	receipt      quotamodel.AuthenticatedReceipt
	availability quotamodel.Availability
	usage        quotamodel.Usage
	home         string
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
	home := status.accounts.CodexHome(account.ID)
	return status.scheduleAuthenticatedRecoveryProbeAtHome043714(ctx, account.ID, home, selection, "", false, false)
}

// The startup profile's home is authoritative when a managed profile does
// not share the legacy account-store path. Keep exact path resolution scoped
// to the caller; do not infer a home from another user's profile.
func (status *Status) scheduleAuthenticatedRecoveryProbeAtHome043714(
	ctx context.Context, accountID, home string, selection quotamodel.Selection,
	baseURL string, noProxy, anySelection bool,
) error {
	if status == nil || accountID == "" || home == "" {
		return errors.New("authenticated quota probe requires a profile home")
	}
	_, ok := status.usage.(quotaAuthenticatedGateway043714)
	if !ok {
		return errors.New("authenticated quota probe is unavailable")
	}
	gateway := status.usage.(quotaAuthenticatedGateway043714)

	key := quotaAuthProofKey043714{accountID: accountID, selection: selection, anySelection: anySelection}
	// Use a distinct prefix so a regular snapshot refresh cannot suppress
	// revalidation merely because the quota cache is already fresh.
	queueKey := "auth-quota-recovery\x00" + accountID + "\x00" + home + "\x00"
	if anySelection {
		queueKey += "any"
	} else {
		queueKey += string(selection.RouteKind) + "\x00" + selection.RequestedModel
	}
	return status.ensureProbeRefreshQueue().Schedule(ctx, queueKey, func(runCtx context.Context) error {
		generation := status.probeSequence.Add(1)
		if err := status.probes.acquire(runCtx); err != nil {
			return err
		}
		usage, receipt, err := gateway.FetchAuthenticatedAtPolicy(runCtx, home, baseURL, noProxy)
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
		liveCache := strings.TrimSpace(baseURL) == "" && !noProxy
		if !status.applySuccessfulUsageProbe043714(runCtx, accountID, home, usage, now, generation, liveCache) {
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
				receipt: receipt, availability: availability, usage: usage, home: home,
				observedAt: now, generation: generation,
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
	key := quotaAuthProofKey043714{accountID: accountID, selection: selection}
	status.authProofMu.Lock()
	proof, found := status.authProofs[key]
	if !found {
		key = quotaAuthProofKey043714{accountID: accountID, anySelection: true}
		proof, found = status.authProofs[key]
	}
	status.authProofMu.Unlock()
	now := status.now()
	if !found || now.Before(proof.observedAt) || now.Before(proof.receipt.StartedAt) || now.Sub(proof.observedAt) > usageCacheFreshness ||
		!proof.receipt.Confirmed || proof.receipt.CredentialDigest == ([32]byte{}) {
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	// The probe's final accepted identity was verified when it returned.
	// Repeat the local check before selecting a previously quarantined route;
	// a later auth.json update cannot resurrect old proof by age alone.
	reader, ok := status.usage.(quotaAuthenticatedGateway043714)
	if !ok {
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	home := proof.home
	if home == "" && status.accounts != nil {
		home = status.accounts.CodexHome(accountID)
	}
	if home == "" {
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	current, err := reader.CurrentCredentialDigest(context.Background(), home)
	if err != nil || subtle.ConstantTimeCompare(current[:], proof.receipt.CredentialDigest[:]) != 1 {
		status.authProofMu.Lock()
		if latest, exists := status.authProofs[key]; exists && latest.generation == proof.generation {
			delete(status.authProofs, key)
		}
		status.authProofMu.Unlock()
		return quotamodel.Availability{}, quotamodel.AuthenticatedReceipt{}, false
	}
	availability := proof.availability
	if key.anySelection {
		availability = routeAvailability(proof.usage, selection, now)
		availability.Source = quotamodel.SourceLive
	}
	return availability, proof.receipt, true
}
