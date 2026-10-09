package routing

import (
	"context"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	"time"
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

// rememberVerifiedAccountBinding keeps ephemeral provider responses local
// while preserving durable ownership for actual managed provider profiles.
func (router *Router) rememberVerifiedAccountBinding(ctx context.Context, accountID string, keys affinityKeys, now time.Time) error {
	router.mu.Lock()
	_, ephemeral := router.ephemeralCredentialIDs[accountID]
	router.mu.Unlock()
	if ephemeral {
		return router.affinity.rememberVerifiedVolatile(accountID, keys, now)
	}
	return router.affinity.rememberVerified(ctx, accountID, keys, now)
}

// A synthetic API key must not leave a durable WebSocket turn-state sidecar
// in the profile home. Cache it in memory for the current session only.
func (router *Router) rememberAccountTurnState(ctx context.Context, responseID, accountID, profileHome, turnState string, now time.Time) {
	router.mu.Lock()
	_, ephemeral := router.ephemeralCredentialIDs[accountID]
	router.mu.Unlock()
	if ephemeral {
		router.affinity.rememberResponseTurnState(responseID, accountID, turnState, now)
		return
	}
	router.affinity.rememberResponseTurnStateForHome(ctx, responseID, accountID, profileHome, turnState, now)
}
