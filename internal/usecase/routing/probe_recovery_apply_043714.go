package routing

import (
	"context"
	"crypto/subtle"
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
	// Create the standard bounded auth quarantine first; a missing/untrusted
	// request fingerprint still blocks normal selection as it did previously.
	router.quarantineAuthFailure(accountID, time.Minute)
	router.mu.Lock()
	state := router.quarantine[accountID]
	state.authFailedAt = now
	state.authKnown = response.AuthCredentialKnown && response.AuthCredentialDigest != ([32]byte{}) &&
		!response.AuthRequestStartedAt.IsZero() && !response.AuthRequestStartedAt.After(now)
	state.authDigest = [32]byte{}
	if state.authKnown {
		state.authDigest = response.AuthCredentialDigest
	}
	router.quarantine[accountID] = state
	router.mu.Unlock()
}

func (router *Router) scheduleAuthRecoveryProbe043714(ctx context.Context, account proxymodel.Account, selection quotamodel.Selection) {
	if router == nil || account.EphemeralAPIKey || !account.Enabled || account.Home == "" ||
		externalProviderKind(account.Provider.Kind) || ctx.Err() != nil {
		return
	}
	probe, ok := router.quota.(authenticatedQuotaRecovery043714)
	if !ok {
		return
	}
	// Scheduling is bounded and non-blocking; no network I/O enters the
	// provider request hotpath, and a failed probe never unlocks a profile.
	_ = probe.ScheduleAuthenticatedRecoveryProbe(ctx, accountentity.Account{ID: account.ID, Enabled: true}, selection)
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
			subtle.ConstantTimeCompare(state.authDigest[:], receipt.CredentialDigest[:]) != 1 ||
			router.quotaBlocked[account.ID] {
			router.mu.Unlock()
			continue
		}
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
		router.notifySelectionChange()
	}
}
