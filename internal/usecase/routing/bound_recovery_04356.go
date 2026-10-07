package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const responsesFullContextRetryMessage = "Previous response was not found. Retrying the full request."

func (router *Router) recoverBoundRetryableFailure(
	ctx context.Context,
	request proxymodel.Request,
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	keys *affinityKeys,
	outcome responseOutcome,
	pending *pendingResponse,
) (proxymodel.Forwarded, bool, error) {
	if request.WebSocketMessage {
		return proxymodel.Forwarded{}, false, nil
	}
	if keys == nil {
		local := requestRoutingAffinity(request)
		keys = &local
	}
	fallback := router.boundRecoveryFallbackAccounts(accounts, failed, request)
	if len(fallback) == 0 {
		return proxymodel.Forwarded{}, false, nil
	}

	if outcome.quota &&
		request.QuotaSelection.RouteKind == quotamodel.RouteKindResponses &&
		keys.previous == "" && keys.turn != "" &&
		requestHasReconstructableFullHistory(request) {
		owned, err := router.turnStateOwnedBy(ctx, keys.turn, failed.ID)
		if err != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, true, err
		}
		if owned {
			if err := router.affinity.releaseOwnedDead(
				ctx, failed.ID, affinityKeys{turn: keys.turn, session: keys.session}, router.now(),
			); err != nil {
				closePendingResponse(pending)
				return proxymodel.Forwarded{}, true, err
			}
			closePendingResponse(pending)
			replay := requestWithoutTurnState(request)
			result, err := router.forwardFresh(ctx, replay, fallback)
			return result, true, err
		}
	}

	if (request.QuotaSelection.RouteKind == quotamodel.RouteKindResponses ||
		request.QuotaSelection.RouteKind == quotamodel.RouteKindCompact) && keys.previous != "" {
		if err := router.affinity.forgetOwned(ctx, failed.ID, *keys, router.now()); err != nil {
			closePendingResponse(pending)
			return proxymodel.Forwarded{}, true, err
		}
		closePendingResponse(pending)
		return proxymodel.Forwarded{
			Response:  responsesFullContextRetryResponse(),
			AccountID: failed.ID,
			Failed:    true,
		}, true, nil
	}

	if keys.thread == "" && keys.session == "" {
		return proxymodel.Forwarded{}, false, nil
	}
	if err := router.affinity.forgetOwned(ctx, failed.ID, *keys, router.now()); err != nil {
		closePendingResponse(pending)
		return proxymodel.Forwarded{}, true, err
	}
	closePendingResponse(pending)
	result, err := router.forwardFresh(ctx, request, fallback)
	return result, true, err
}

func (router *Router) boundRecoveryFallbackAccounts(
	accounts []proxymodel.Account,
	failed proxymodel.Account,
	request proxymodel.Request,
) []proxymodel.Account {
	pool := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.ID == failed.ID || !sameRuntimeProvider(failed, account) {
			continue
		}
		pool = append(pool, account)
	}
	if len(pool) == 0 {
		return nil
	}
	return router.requestCandidatesForRequestWithoutRotation(pool, request, router.now())
}

func sameRuntimeProvider(left, right proxymodel.Account) bool {
	leftKind := strings.ToLower(strings.TrimSpace(left.Provider.Kind))
	rightKind := strings.ToLower(strings.TrimSpace(right.Provider.Kind))
	if leftKind == "" {
		leftKind = "openai"
	}
	if rightKind == "" {
		rightKind = "openai"
	}
	return leftKind == rightKind
}

func responsesFullContextRetryResponse() *proxymodel.Response {
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    "previous_response_not_found",
			"message": responsesFullContextRetryMessage,
		},
	})
	if err != nil {
		panic("marshal static full-context retry response")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &proxymodel.Response{
		StatusCode: http.StatusBadRequest,
		Header:     headers,
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
