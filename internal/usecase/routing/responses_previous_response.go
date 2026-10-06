package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

var responsesPreviousResponseRetryDelays = [...]time.Duration{
	75 * time.Millisecond,
	200 * time.Millisecond,
	500 * time.Millisecond,
}

func (router *Router) handleResponsesPreviousResponseNotFound(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	response *proxymodel.Response,
	outcome responseOutcome,
	pending *pendingResponse,
	keys *affinityKeys,
) (*proxymodel.Response, responseOutcome, *pendingResponse, bool, error) {
	previous := requestPreviousResponseID(request.Body)
	if previous == "" || request.QuotaSelection.RouteKind != quotamodel.RouteKindResponses {
		return response, outcome, pending, false, nil
	}
	for retryIndex := 0; outcome.previousResponseNotFound; retryIndex++ {
		turnState := responseTurnStateValue(response)
		if turnState != "" && retryIndex < len(responsesPreviousResponseRetryDelays) {
			pending.close()
			if err := router.wait(ctx, responsesPreviousResponseRetryDelays[retryIndex]); err != nil {
				return nil, responseOutcome{}, nil, false, err
			}
			retry := request
			retry.Header = request.Header.Clone()
			if retry.Header == nil {
				retry.Header = make(http.Header)
			}
			retry.Header.Set("x-codex-turn-state", turnState)
			retried, err := router.execute(ctx, retry, account)
			if err != nil {
				return nil, responseOutcome{}, nil, false, err
			}
			if !inspectablePreviousResponse(retried) {
				return retried, responseOutcome{kind: responsePass}, &pendingResponse{response: retried}, false, nil
			}
			retryOutcome, retryPending, err := router.classify(retried, account.Provider.Kind)
			if err != nil {
				restoreResponsePrefix(retried, retryPending)
				return retried, responseOutcome{kind: responsePass}, &pendingResponse{response: retried}, false, nil
			}
			response, outcome, pending = retried, retryOutcome, retryPending
			continue
		}
		if err := router.notePreviousResponseNotFound(ctx, account, previous, request.QuotaSelection, keys); err != nil {
			pending.close()
			return nil, responseOutcome{}, nil, false, err
		}
		pending.previousResponseNotFound = true
		return response, outcome, pending, true, nil
	}
	return response, outcome, pending, false, nil
}

func staleResponsesContinuationResponse() *proxymodel.Response {
	body, err := json.Marshal(map[string]any{
		"error": map[string]any{
			"code":    "stale_continuation",
			"message": staleContinuationMessage,
		},
	})
	if err != nil {
		panic("marshal static stale continuation response")
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &proxymodel.Response{
		StatusCode:       http.StatusConflict,
		Header:           headers,
		Body:             io.NopCloser(bytes.NewReader(body)),
		PrecommitFailure: &proxymodel.PrecommitFailure{Code: "previous_response_not_found", StaleContinuation: true},
	}
}

func previousResponseFallbackAccounts(accounts []proxymodel.Account, failedAccountID string) []proxymodel.Account {
	result := make([]proxymodel.Account, 0, len(accounts))
	for _, account := range accounts {
		if account.ID != failedAccountID && account.Enabled {
			result = append(result, account)
		}
	}
	return result
}
