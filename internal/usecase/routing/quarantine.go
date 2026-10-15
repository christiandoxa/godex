package routing

import "time"

const maxQuarantinedAccounts = 4096

type quarantineState struct {
	until       time.Time
	authFailure bool
	// Auth provenance is private to the active router. An unrelated retry
	// lease never becomes removable by an authenticated quota receipt.
	authUntil            time.Time
	otherUntil           time.Time
	authFailedAt         time.Time
	authProbeScheduledAt time.Time
	authDigest           [32]byte
	authKnown            bool
}

func (proxy *Router) isQuarantined(accountID string, now time.Time) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	state, ok := proxy.quarantine[accountID]
	if !ok || !state.until.After(now) {
		delete(proxy.quarantine, accountID)
		return false
	}
	return true
}

func (proxy *Router) quarantineRemaining(accountID string, now time.Time) time.Duration {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	state, ok := proxy.quarantine[accountID]
	if !ok || !state.until.After(now) {
		delete(proxy.quarantine, accountID)
		return 0
	}
	return state.until.Sub(now)
}

func (proxy *Router) quarantineAccount(accountID string, duration time.Duration) {
	proxy.setQuarantine(accountID, duration, false)
}

func (proxy *Router) quarantineAuthFailure(accountID string, duration time.Duration) {
	proxy.setQuarantine(accountID, duration, true)
}

func (proxy *Router) setQuarantine(accountID string, duration time.Duration, authFailure bool) {
	if duration <= 0 {
		duration = time.Second
	}
	now := proxy.now()
	proxy.mu.Lock()
	if proxy.quarantine == nil {
		proxy.quarantine = make(map[string]quarantineState)
	}
	proxy.pruneQuarantineLocked(now)
	until := now.Add(duration)
	current, exists := proxy.quarantine[accountID]
	if current.until.After(until) {
		until = current.until
	}
	if !exists && len(proxy.quarantine) >= maxQuarantinedAccounts {
		oldestID := ""
		var oldest time.Time
		for id, state := range proxy.quarantine {
			expires := state.until
			if oldestID == "" || expires.Before(oldest) || (expires.Equal(oldest) && id < oldestID) {
				oldestID, oldest = id, expires
			}
		}
		delete(proxy.quarantine, oldestID)
	}
	current.until = until
	if authFailure {
		current.authFailure = true
		if now.Add(duration).After(current.authUntil) {
			current.authUntil = now.Add(duration)
		}
		if current.authFailedAt.IsZero() {
			current.authFailedAt = now
		}
	} else if now.Add(duration).After(current.otherUntil) {
		current.otherUntil = now.Add(duration)
	}
	proxy.quarantine[accountID] = current
	proxy.mu.Unlock()
	proxy.notifySelectionChange()
}

func (proxy *Router) authFailureQuarantined(accountID string, now time.Time) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	state, ok := proxy.quarantine[accountID]
	if !ok || !state.until.After(now) {
		delete(proxy.quarantine, accountID)
		return false
	}
	return state.authFailure
}

func (proxy *Router) pruneQuarantineLocked(now time.Time) {
	for accountID, state := range proxy.quarantine {
		if !state.until.After(now) {
			delete(proxy.quarantine, accountID)
		}
	}
}

func (proxy *Router) clearQuarantine(accountID string) {
	proxy.mu.Lock()
	delete(proxy.quarantine, accountID)
	delete(proxy.quotaBlocked, accountID)
	proxy.mu.Unlock()
	proxy.notifySelectionChange()
}

func (proxy *Router) markQuotaBlocked(accountID string) {
	proxy.mu.Lock()
	if proxy.quotaBlocked == nil {
		proxy.quotaBlocked = make(map[string]bool)
	}
	proxy.quotaBlocked[accountID] = true
	proxy.mu.Unlock()
	proxy.notifySelectionChange()
}

func (proxy *Router) quotaBlockedAccount(accountID string) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.quotaBlocked[accountID]
}

func (proxy *Router) clearQuotaBlocked(accountID string) {
	proxy.mu.Lock()
	delete(proxy.quotaBlocked, accountID)
	proxy.mu.Unlock()
	proxy.notifySelectionChange()
}
