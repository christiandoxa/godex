package routing

import "time"

const maxQuarantinedAccounts = 4096

func (proxy *Router) isQuarantined(accountID string, now time.Time) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	until, ok := proxy.quarantine[accountID]
	if !ok || !until.After(now) {
		delete(proxy.quarantine, accountID)
		return false
	}
	return true
}

func (proxy *Router) quarantineAccount(accountID string, duration time.Duration) {
	if duration <= 0 {
		duration = time.Second
	}
	now := proxy.now()
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	proxy.pruneQuarantineLocked(now)
	until := now.Add(duration)
	if current := proxy.quarantine[accountID]; current.After(until) {
		until = current
	}
	if _, exists := proxy.quarantine[accountID]; !exists && len(proxy.quarantine) >= maxQuarantinedAccounts {
		oldestID := ""
		var oldest time.Time
		for id, expires := range proxy.quarantine {
			if oldestID == "" || expires.Before(oldest) || (expires.Equal(oldest) && id < oldestID) {
				oldestID, oldest = id, expires
			}
		}
		delete(proxy.quarantine, oldestID)
	}
	proxy.quarantine[accountID] = until
}

func (proxy *Router) pruneQuarantineLocked(now time.Time) {
	for accountID, until := range proxy.quarantine {
		if !until.After(now) {
			delete(proxy.quarantine, accountID)
		}
	}
}

func (proxy *Router) clearQuarantine(accountID string) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	delete(proxy.quarantine, accountID)
	delete(proxy.quotaBlocked, accountID)
}

func (proxy *Router) markQuotaBlocked(accountID string) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	if proxy.quotaBlocked == nil {
		proxy.quotaBlocked = make(map[string]bool)
	}
	proxy.quotaBlocked[accountID] = true
}

func (proxy *Router) quotaBlockedAccount(accountID string) bool {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	return proxy.quotaBlocked[accountID]
}

func (proxy *Router) clearQuotaBlocked(accountID string) {
	proxy.mu.Lock()
	defer proxy.mu.Unlock()
	delete(proxy.quotaBlocked, accountID)
}
