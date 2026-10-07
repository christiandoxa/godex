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

type websocketQuotaFallbackKind uint8

const (
	websocketQuotaFallbackUnavailable websocketQuotaFallbackKind = iota
	websocketQuotaFallbackReady
	websocketQuotaFallbackLastChance
)

type websocketQuotaFallbackDecision struct {
	kind    websocketQuotaFallbackKind
	account proxymodel.Account
}

func (router *Router) handleBoundWebSocketPreSendQuotaBlock(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
) (proxymodel.Forwarded, bool, error) {
	if !request.WebSocketMessage || !router.boundWebSocketQuotaBlocked(account) {
		return proxymodel.Forwarded{}, false, nil
	}
	metadata := routedWebSocketRequestMetadata(request)
	decision := router.websocketQuotaFallbackDecisionWithContext(
		request, accounts, account, false,
	)
	if decision.kind == websocketQuotaFallbackUnavailable {
		return proxymodel.Forwarded{}, true, &proxymodel.Error{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "websocket quota fallback is unavailable",
		}
	}
	if metadata.previousResponseID != "" {
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
			return proxymodel.Forwarded{}, true, err
		}
		return proxymodel.Forwarded{
			Response:  websocketQuotaFullContextSignalResponse(),
			AccountID: account.ID,
			Failed:    true,
		}, true, nil
	}
	if metadata.sessionID == "" || metadata.turnState != "" {
		return proxymodel.Forwarded{}, false, nil
	}
	if err := router.affinity.forgetOwned(
		ctx, account.ID, affinityKeys{session: metadata.sessionID}, router.now(),
	); err != nil {
		return proxymodel.Forwarded{}, true, err
	}
	if decision.kind == websocketQuotaFallbackLastChance {
		result, err := router.forwardWebSocketQuotaLastChance(ctx, request, decision.account)
		return result, true, err
	}
	result, err := router.forwardFresh(ctx, request, accounts)
	return result, true, err
}

func routedWebSocketRequestMetadata(request proxymodel.Request) websocketRequestMetadata {
	metadata := parseWebSocketRequestMetadata(request)
	affinity := requestRoutingAffinity(request)
	if metadata.previousResponseID == "" {
		metadata.previousResponseID = affinity.previous
	}
	if metadata.sessionID == "" {
		metadata.sessionID = affinity.session
	}
	if metadata.turnState == "" {
		metadata.turnState = affinity.turn
	}
	return metadata
}

func (router *Router) boundWebSocketQuotaBlocked(account proxymodel.Account) bool {
	now := router.now()
	if account.EligibleAfter.After(now) {
		return true
	}
	return router.quotaBlockedAccount(account.ID) && router.isQuarantined(account.ID, now)
}

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
	router.recordRouteOutcome(ctx, account.ID, request.QuotaSelection, response, outcome)
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	router.applyRetryOutcome(ctx, account.ID, request.QuotaSelection, outcome)
	return router.handleBoundWebSocketRetryable(
		ctx, request, accounts, account, outcome, pending,
	)
}

func (router *Router) handleBoundWebSocketRetryable(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	account proxymodel.Account,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, error) {
	metadata := routedWebSocketRequestMetadata(request)

	if metadata.previousResponseID != "" {
		decision := router.websocketQuotaFallbackDecisionWithContext(
			request, accounts, account, false,
		)
		if decision.kind == websocketQuotaFallbackUnavailable {
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

	decision := router.websocketQuotaFallbackDecision(request, accounts, account, metadata)
	if outcome.quota && metadata.previousResponseID == "" && metadata.turnState != "" &&
		decision.kind == websocketQuotaFallbackReady && requestHasReconstructableFullHistory(request) {
		owned, err := router.turnStateOwnedBy(ctx, metadata.turnState, account.ID)
		if err != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, err
		}
		if owned {
			if err := router.affinity.releaseOwnedDead(
				ctx, account.ID,
				affinityKeys{turn: metadata.turnState, session: metadata.sessionID},
				router.now(),
			); err != nil {
				closePendingResponse(pending)
				return proxymodel.Forwarded{}, err
			}
			closePendingResponse(pending)
			return router.forwardFresh(ctx, requestWithoutTurnState(request), accounts)
		}
	}
	if metadata.turnState != "" {
		return pendingForwarded(account.ID, outcome, pending), nil
	}
	if metadata.sessionID == "" || decision.kind == websocketQuotaFallbackUnavailable {
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
	if decision.kind == websocketQuotaFallbackLastChance {
		if !outcome.quota {
			return pendingForwarded(account.ID, outcome, pending), nil
		}
		return router.forwardWebSocketQuotaLastChance(ctx, request, decision.account)
	}
	return router.forwardFresh(ctx, request, accounts)
}

func (router *Router) websocketQuotaFallbackDecision(
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	metadata websocketRequestMetadata,
) websocketQuotaFallbackDecision {
	hasContextConstraint := metadata.previousResponseID != "" ||
		metadata.requiresPreviousResponseAffinity ||
		metadata.turnState != ""
	return router.websocketQuotaFallbackDecisionWithContext(
		request, accounts, failed, hasContextConstraint,
	)
}

func (router *Router) websocketQuotaFallbackDecisionWithContext(
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	hasContextConstraint bool,
) websocketQuotaFallbackDecision {
	now := router.now()

	var lastChance *proxymodel.Account
	for _, candidate := range accounts {
		if candidate.ID == failed.ID ||
			candidate.ID == "" ||
			candidate.Home == "" ||
			!candidate.Enabled ||
			candidate.EligibleAfter.After(now) ||
			!sameWebSocketProvider(failed, candidate) ||
			router.authFailureQuarantined(candidate.ID, now) ||
			router.quotaBlockedAccount(candidate.ID) ||
			router.profileInflightHardLimitedForRequest(candidate.ID, request) {
			continue
		}
		if !router.isQuarantined(candidate.ID, now) {
			return websocketQuotaFallbackDecision{
				kind: websocketQuotaFallbackReady, account: candidate,
			}
		}
		if !hasContextConstraint && lastChance == nil {
			copy := candidate
			lastChance = &copy
		}
	}
	if lastChance != nil {
		return websocketQuotaFallbackDecision{
			kind: websocketQuotaFallbackLastChance, account: *lastChance,
		}
	}
	return websocketQuotaFallbackDecision{kind: websocketQuotaFallbackUnavailable}
}

func (router *Router) websocketQuotaReplayLastChance(
	request proxymodel.Request,
	accounts []proxymodel.Account,
) (proxymodel.Account, bool) {
	if !request.WebSocketMessage {
		return proxymodel.Account{}, false
	}
	metadata := parseWebSocketRequestMetadata(request)
	if metadata.sessionID == "" || metadata.previousResponseID != "" || metadata.turnState != "" {
		return proxymodel.Account{}, false
	}
	now := router.now()
	for _, blocked := range accounts {
		if blocked.ID == "" || blocked.Home == "" || !blocked.Enabled {
			continue
		}
		if !blocked.EligibleAfter.After(now) && !router.quotaBlockedAccount(blocked.ID) {
			continue
		}
		decision := router.websocketQuotaFallbackDecisionWithContext(
			request, accounts, blocked, false,
		)
		if decision.kind == websocketQuotaFallbackLastChance {
			return decision.account, true
		}
	}
	return proxymodel.Account{}, false
}

func sameWebSocketProvider(left, right proxymodel.Account) bool {
	leftKind := strings.TrimSpace(left.Provider.Kind)
	rightKind := strings.TrimSpace(right.Provider.Kind)
	return leftKind == "" || rightKind == "" || strings.EqualFold(leftKind, rightKind)
}

func (router *Router) profileInflightHardLimitedForRequest(
	accountID string,
	request proxymodel.Request,
) bool {
	weight := requestProfileInflightWeight(request)
	router.mu.Lock()
	current := router.inflight[accountID]
	limit := effectiveProfileInflightHardLimit(router.profileInflightHardLimit, weight)
	router.mu.Unlock()
	return current+weight > limit
}

func (router *Router) forwardWebSocketQuotaLastChance(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
) (proxymodel.Forwarded, error) {
	router.recordRouteDecisionSelected(ctx, request, account)
	response, acquired, err := router.tryExecuteWithProfileInflight(ctx, request, account, false)
	if err != nil {
		if ctx.Err() != nil {
			return proxymodel.Forwarded{}, ctx.Err()
		}
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "websocket quota last-chance profile could not be reached",
		}
	}
	if !acquired {
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "websocket quota last-chance profile reached its hard in-flight limit",
		}
	}
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, &proxymodel.Error{
			StatusCode: http.StatusBadGateway,
			Message:    "websocket quota last-chance response failed before commitment",
		}
	}
	if outcome.kind == responsePass {
		router.clearQuotaBlocked(account.ID)
	} else {
		router.applyRetryOutcome(ctx, account.ID, request.QuotaSelection, outcome)
	}
	return pendingForwarded(account.ID, outcome, pending), nil
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
	if remover, ok := store.repository.(affinityBindingRemover); ok && store.writesEnabled() {
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
