package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const websocketQuotaFullContextMessage = "Previous response was not found. Retrying the full request."

func (router *Router) handleBoundWebSocketResponse(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	response *proxymodel.Response,
) (proxymodel.Forwarded, error) {
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "conversation owner response failed before commitment",
		}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	router.applyRetryOutcome(account.ID, outcome)
	if !outcome.quota {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	return router.handleBoundWebSocketQuota(
		ctx, request, accounts, account, outcome, pending,
	)
}

func (router *Router) handleBoundWebSocketQuota(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, error) {
	if !router.websocketQuotaFallbackReady(accounts, account) {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	metadata := parseWebSocketRequestMetadata(request)

	if metadata.previousResponseID != "" {
		if metadata.sessionID == "" {
			return pendingForwarded(account.ID, outcome, pending), nil
		}
		if err := router.affinity.forgetOwned(
			ctx,
			account.ID,
			affinityKeys{
				previous: metadata.previousResponseID,
				turn:     metadata.turnState,
				session:  metadata.sessionID,
			},
			router.now(),
		); err != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, err
		}
		closePendingResponse(pending)
		return proxymodel.Forwarded{
			Response:  websocketQuotaFullContextSignalResponse(),
			AccountID: account.ID,
			Failed:    true,
		}, nil
	}

	if metadata.turnState != "" {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	if metadata.sessionID == "" {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	if err := router.affinity.forgetOwned(
		ctx,
		account.ID,
		affinityKeys{session: metadata.sessionID},
		router.now(),
	); err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, err
	}
	closePendingResponse(pending)
	return router.forwardFresh(ctx, request, accounts)
}

func (router *Router) websocketQuotaFallbackReady(
	accounts []proxymodel.Account,
	failed proxymodel.Account,
) bool {
	now := router.now()
	for _, candidate := range router.recoveryAvailableAccounts(accounts, now) {
		if candidate.ID == failed.ID {
			continue
		}
		failedKind := strings.TrimSpace(failed.Provider.Kind)
		candidateKind := strings.TrimSpace(candidate.Provider.Kind)
		if failedKind != "" && candidateKind != "" &&
			!strings.EqualFold(failedKind, candidateKind) {
			continue
		}
		return true
	}
	return false
}

func (store *affinityStore) forgetOwned(
	ctx context.Context,
	accountID string,
	keys affinityKeys,
	now time.Time,
) error {
	entries := keys.entries()
	if accountID == "" || len(entries) == 0 {
		return nil
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.loadMissingLocked(ctx, keys, now); err != nil {
		return err
	}

	remove := make([]string, 0, len(entries))
	for _, entry := range entries {
		if current, ok := store.values[entry.Key]; ok && current.accountID == accountID {
			remove = append(remove, entry.Key)
		}
	}
	if len(remove) == 0 {
		return nil
	}
	if remover, ok := store.repository.(affinityBindingRemover); ok {
		if err := remover.Remove(ctx, remove); err != nil {
			return err
		}
	}
	for _, key := range remove {
		delete(store.values, key)
	}
	return nil
}

func websocketQuotaFullContextSignalResponse() *proxymodel.Response {
	payload, err := json.Marshal(map[string]any{
		"type":   "error",
		"status": http.StatusBadRequest,
		"error": map[string]any{
			"code":    "previous_response_not_found",
			"message": websocketQuotaFullContextMessage,
		},
	})
	if err != nil {
		panic("marshal static websocket quota full-context signal")
	}
	return websocketTextPayloadResponse(payload, http.StatusBadRequest)
}
