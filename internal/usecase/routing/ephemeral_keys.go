package routing

import (
	"context"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func (router *Router) loadAccounts(ctx context.Context) ([]proxymodel.Account, error) {
	accounts, err := router.source(ctx)
	if err != nil {
		return nil, &proxymodel.Error{StatusCode: 503, Message: "cannot load managed accounts"}
	}
	router.observeEphemeralCredentials(accounts)
	return sortRuntimeAccounts(accounts), nil
}

// observeEphemeralCredentials removes any stale, previously persisted
// credential-only routing scores on first use. Subsequent in-process
// quarantines and penalties are preserved for safe local key rotation.
func (router *Router) observeEphemeralCredentials(accounts []proxymodel.Account) {
	router.mu.Lock()
	defer router.mu.Unlock()
	if router.ephemeralCredentialIDs == nil {
		router.ephemeralCredentialIDs = make(map[string]struct{})
	}
	for _, account := range accounts {
		if !account.EphemeralAPIKey {
			continue
		}
		if _, exists := router.ephemeralCredentialIDs[account.ID]; exists {
			continue
		}
		router.ephemeralCredentialIDs[account.ID] = struct{}{}
		delete(router.quarantine, account.ID)
		for key := range router.routeHealth {
			if key.accountID == account.ID {
				delete(router.routeHealth, key)
			}
		}
		for key := range router.routeMemory {
			if key.accountID == account.ID {
				delete(router.routeMemory, key)
			}
		}
		for key := range router.routeCircuits {
			if key.accountID == account.ID {
				delete(router.routeCircuits, key)
			}
		}
		for key := range router.transportBackoffs {
			if key.accountID == account.ID {
				delete(router.transportBackoffs, key)
			}
		}
	}
}

// Account-scoped persistence is never used for launch-local API-key pools.
// Durable managed profiles still use every existing recovery store.
func (router *Router) persistAccountState(accountID string) bool {
	if !router.persistenceWritesEnabled() {
		return false
	}
	router.mu.Lock()
	_, ephemeral := router.ephemeralCredentialIDs[accountID]
	router.mu.Unlock()
	return !ephemeral
}
