package routing

import (
	"context"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

type responseTurnState struct {
	accountID string
	value     string
	expires   time.Time
	sequence  uint64
}

type turnStateRepository interface {
	SaveResponseTurnState(context.Context, string, string, string, time.Time) error
	LoadResponseTurnState(context.Context, string, string, time.Time) (string, time.Time, error)
}

type responseTurnStateRemover interface {
	RemoveResponseTurnState(context.Context, string, string) error
}

func restorePreviousResponseTurnState(
	ctx context.Context,
	request *proxymodel.Request,
	accounts []proxymodel.Account,
	owner string,
	keys *affinityKeys,
	store *affinityStore,
	now time.Time,
) {
	if owner == "" || keys.previous == "" || keys.turn != "" {
		return
	}
	for _, account := range accounts {
		if account.ID != owner || externalProviderKind(account.Provider.Kind) {
			continue
		}
		turnState := store.responseTurnStateForHome(ctx, keys.previous, owner, account.Home, now)
		if turnState == "" {
			return
		}
		request.Header = request.Header.Clone()
		if request.Header == nil {
			request.Header = make(http.Header)
		}
		request.Header.Set("x-codex-turn-state", turnState)
		keys.turn = turnState
		return
	}
}

func responseTurnStateHome(accounts []proxymodel.Account, accountID string) string {
	for _, account := range accounts {
		if account.ID == accountID && !externalProviderKind(account.Provider.Kind) {
			return account.Home
		}
	}
	return ""
}

func responseTurnStateValue(response *proxymodel.Response) string {
	if response == nil {
		return ""
	}
	turnState := strings.TrimSpace(response.WebSocketTurnState)
	if turnState == "" && response.Header != nil {
		turnState = strings.TrimSpace(response.Header.Get("x-codex-turn-state"))
	}
	if len(turnState) > maxAffinityValue || strings.ContainsAny(turnState, "\r\n") {
		return ""
	}
	return turnState
}

func (store *affinityStore) rememberResponseTurnState(responseID, accountID, turnState string, now time.Time) {
	responseID, accountID, turnState = strings.TrimSpace(responseID), strings.TrimSpace(accountID), strings.TrimSpace(turnState)
	if responseID == "" || accountID == "" || turnState == "" ||
		len(responseID) > maxAffinityValue || len(turnState) > maxAffinityValue || strings.ContainsAny(turnState, "\r\n") {
		return
	}
	store.cacheResponseTurnState(responseID, accountID, turnState, now.Add(affinityTTL), now)
}

func (store *affinityStore) rememberResponseTurnStateForHome(
	ctx context.Context,
	responseID, accountID, profileHome, turnState string,
	now time.Time,
) {
	responseID, accountID, profileHome = strings.TrimSpace(responseID), strings.TrimSpace(accountID), strings.TrimSpace(profileHome)
	turnState = strings.TrimSpace(turnState)
	if responseID == "" || accountID == "" || turnState == "" || len(responseID) > maxAffinityValue ||
		len(turnState) > maxAffinityValue || strings.ContainsAny(turnState, "\r\n") {
		return
	}
	if repository, ok := store.repository.(turnStateRepository); ok && profileHome != "" && store.writesEnabled() {
		// The per-profile sidecar is optional; never fail a response because it could not be written.
		_ = repository.SaveResponseTurnState(
			ctx, profileHome, affinityDigest("previous", responseID), turnState, now.Add(affinityTTL),
		)
	}
	store.cacheResponseTurnState(responseID, accountID, turnState, now.Add(affinityTTL), now)
}

func (store *affinityStore) cacheResponseTurnState(
	responseID, accountID, turnState string,
	expiresAt, now time.Time,
) {
	if !expiresAt.After(now) {
		return
	}
	key := affinityDigest("previous", responseID)
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	if store.turnStates == nil {
		store.turnStates = make(map[string]responseTurnState)
	}
	store.sequence++
	store.turnStates[key] = responseTurnState{
		accountID: accountID, value: turnState, expires: expiresAt, sequence: store.sequence,
	}
	store.pruneLocked(now)
}

func (store *affinityStore) responseTurnState(responseID, accountID string, now time.Time) string {
	responseID, accountID = strings.TrimSpace(responseID), strings.TrimSpace(accountID)
	if responseID == "" || accountID == "" || len(responseID) > maxAffinityValue {
		return ""
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.pruneLocked(now)
	state, ok := store.turnStates[affinityDigest("previous", responseID)]
	if !ok || state.accountID != accountID {
		return ""
	}
	return state.value
}

func (store *affinityStore) responseTurnStateForHome(
	ctx context.Context,
	responseID, accountID, profileHome string,
	now time.Time,
) string {
	responseID, accountID = strings.TrimSpace(responseID), strings.TrimSpace(accountID)
	if responseID == "" || accountID == "" || len(responseID) > maxAffinityValue {
		return ""
	}
	if state := store.responseTurnState(responseID, accountID, now); state != "" {
		return state
	}
	repository, ok := store.repository.(turnStateRepository)
	if !ok || strings.TrimSpace(profileHome) == "" {
		return ""
	}
	key := affinityDigest("previous", responseID)
	value, expiresAt, err := repository.LoadResponseTurnState(ctx, profileHome, key, now)
	if err != nil || value == "" {
		return ""
	}
	store.cacheResponseTurnState(responseID, accountID, value, expiresAt, now)
	return value
}
