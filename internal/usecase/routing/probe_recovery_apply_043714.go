package routing

import (
	"context"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

// Optional quota adapter: legacy preflights remain untouched and no cache or
// persisted quota snapshot can masquerade as credential-bound HTTP evidence.
type authenticatedQuotaRecovery043714 interface {
	ScheduleAuthenticatedRecoveryProbe(context.Context, accountentity.Account, quotamodel.Selection) error
	AuthenticatedRecoveryReceipt(string, quotamodel.Selection) (quotamodel.Availability, quotamodel.AuthenticatedReceipt, bool)
}

func (router *Router) recordAuthFailureEvidence043714(accountID string, response *proxymodel.Response) {
	if router == nil || response == nil || accountID == "" {
		return
	}
	now := router.now()
	// Tagged Prodex 0.437.14 production auth-health decay: score 12 for
	// 401, score 2 for 403; each unit decays over 300 seconds. Ordinary
	// retry/transport quarantine still has independent deadlines.
	authDuration := 10 * time.Minute
	if response.StatusCode == 401 {
		authDuration = time.Hour
	}
	// Create the standard bounded auth quarantine first; a missing/untrusted
	// request fingerprint still blocks normal selection as it did previously.
	router.quarantineAuthFailure(accountID, authDuration)
	router.mu.Lock()
	state := router.quarantine[accountID]
	state.authFailedAt = now
	state.authProbeScheduledAt = time.Time{}
	state.authKnown = response.AuthCredentialKnown && response.AuthCredentialDigest != ([32]byte{}) &&
		!response.AuthRequestStartedAt.IsZero() && !response.AuthRequestStartedAt.After(now)
	state.authDigest = [32]byte{}
	if state.authKnown {
		state.authDigest = response.AuthCredentialDigest
	}
	router.quarantine[accountID] = state
	router.mu.Unlock()
	router.persistAuthBackoff043714(accountID, state)
}

const authRecoveryProbeRetryInterval043714 = 2 * time.Second

func (router *Router) scheduleAuthRecoveryProbe043714(ctx context.Context, account proxymodel.Account, selection quotamodel.Selection) {
	if router == nil || ctx == nil || account.EphemeralAPIKey || !account.Enabled || account.Home == "" ||
		externalProviderKind(account.Provider.Kind) || ctx.Err() != nil {
		return
	}
	probe, ok := router.quota.(authenticatedQuotaRecovery043714)
	if !ok {
		return
	}
	now := router.now()
	router.mu.Lock()
	state, found := router.quarantine[account.ID]
	if !found || !state.authFailure || !state.authKnown || !state.until.After(now) ||
		!state.authProbeScheduledAt.IsZero() && now.Sub(state.authProbeScheduledAt) < authRecoveryProbeRetryInterval043714 {
		router.mu.Unlock()
		return
	}
	// Retry scheduling is bounded across requests; a cached quota-ready DTO
	// is not authenticated evidence, and a queue job never runs inline here.
	state.authProbeScheduledAt = now
	router.quarantine[account.ID] = state
	router.mu.Unlock()
	_ = probe.ScheduleAuthenticatedRecoveryProbe(ctx, accountentity.Account{ID: account.ID, Enabled: true}, selection)
}

func (router *Router) scheduleBlockedAuthRecoveryProbes043714(accounts []proxymodel.Account, selection quotamodel.Selection) {
	if router == nil {
		return
	}
	for _, account := range accounts {
		router.scheduleAuthRecoveryProbe043714(context.Background(), account, selection)
	}
}

func (router *Router) recoverConfirmedAuthQuarantines043714(accounts []proxymodel.Account, selection quotamodel.Selection, now time.Time) {
	if router == nil {
		return
	}
	probe, ok := router.quota.(authenticatedQuotaRecovery043714)
	if !ok {
		return
	}
	for _, account := range accounts {
		if !account.Enabled || account.EphemeralAPIKey || account.ID == "" || account.Home == "" || externalProviderKind(account.Provider.Kind) {
			continue
		}
		availability, receipt, exists := probe.AuthenticatedRecoveryReceipt(account.ID, selection)
		if !exists || !availability.Ready || availability.Source != quotamodel.SourceLive ||
			!receipt.Confirmed || receipt.CredentialDigest == ([32]byte{}) ||
			receipt.StartedAt.IsZero() || receipt.StartedAt.After(now) {
			continue
		}
		router.mu.Lock()
		state, found := router.quarantine[account.ID]
		if !found || !state.until.After(now) || !state.authFailure || !state.authKnown ||
			state.authFailedAt.IsZero() || !receipt.StartedAt.After(state.authFailedAt) ||
			router.quotaBlocked[account.ID] {
			router.mu.Unlock()
			continue
		}
		// The successful receipt is tied to the profile's CURRENT credential
		// by quota.Status, not necessarily the obsolete bearer that received
		// the earlier 401. A rotated-and-verified token may retire the older
		// auth penalty; new failures and separate retry/transport leases stay.
		// The 0.437.14 compiler-owned planner retires only causally older
		// auth failure evidence. We keep other leases and all transport and
		// previous-response ownership state entirely untouched.
		plan := probeRecoveryPolicy043714(probeRecoveryFacts043714{
			authenticatedSuccess: receipt.Confirmed, sameCredentials: true,
			startedAt: receipt.StartedAt.Unix(), now: now.Unix(),
			authFailurePresent: true, authFailureScore: 1, authFailureUpdatedAt: state.authFailedAt.Unix(),
			quotaReady: availability.Ready,
		})
		if !plan.clearAuthFailure {
			router.mu.Unlock()
			continue
		}
		router.mu.Unlock()
		router.authBackoffMu.Lock()
		router.mu.Lock()
		current, stillPresent := router.quarantine[account.ID]
		if !stillPresent || !current.authFailure || current.authFailedAt != state.authFailedAt ||
			current.authUntil != state.authUntil {
			router.mu.Unlock()
			router.authBackoffMu.Unlock()
			continue
		}
		state = current
		router.mu.Unlock()
		if !router.clearInheritedAuthBackoff043714(account.ID, state, now) {
			router.authBackoffMu.Unlock()
			continue
		}
		router.mu.Lock()
		current, stillPresent = router.quarantine[account.ID]
		if !stillPresent || !current.authFailure || current.authFailedAt != state.authFailedAt ||
			current.authUntil != state.authUntil {
			router.mu.Unlock()
			router.authBackoffMu.Unlock()
			continue
		}
		state = current
		if state.otherUntil.After(now) {
			state.authFailure = false
			state.authKnown = false
			state.authDigest = [32]byte{}
			state.authFailedAt = time.Time{}
			state.authUntil = time.Time{}
			state.until = state.otherUntil
			router.quarantine[account.ID] = state
		} else {
			delete(router.quarantine, account.ID)
		}
		router.mu.Unlock()
		router.authBackoffMu.Unlock()
		router.notifySelectionChange()
	}
}

// The child request can briefly wait for a previously queued, bounded quota
// probe to release its reservation. This does not issue network I/O inline or
// grant eligibility from progress alone: reranking still requires the exact
// authenticated credential receipt and the causal Mojo policy above.
type authRecoveryProgress043714 interface {
	ProbeRefreshRevision() uint64
	WaitProbeRefresh(context.Context, uint64) bool
}

func (router *Router) authRecoveryProgressRevision043714() (uint64, bool) {
	if router == nil {
		return 0, false
	}
	progress, ok := router.quota.(authRecoveryProgress043714)
	if !ok {
		return 0, false
	}
	return progress.ProbeRefreshRevision(), true
}

func (router *Router) waitForAuthRecoveryProgress043714(ctx context.Context, accounts []proxymodel.Account, observed uint64) bool {
	if router == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	progress, ok := router.quota.(authRecoveryProgress043714)
	if !ok {
		return false
	}
	now := router.now()
	router.mu.Lock()
	blocked := false
	for _, account := range accounts {
		state, found := router.quarantine[account.ID]
		if found && account.Enabled && state.authFailure && state.authKnown && state.until.After(now) {
			blocked = true
			break
		}
	}
	router.mu.Unlock()
	if !blocked {
		return false
	}
	bounded, cancel := context.WithTimeout(ctx, 850*time.Millisecond)
	defer cancel()
	return progress.WaitProbeRefresh(bounded, observed)
}
