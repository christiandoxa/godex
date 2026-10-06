package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func (router *Router) recoverInvalidPreviousResponse(
	ctx context.Context,
	request proxymodel.Request,
	account proxymodel.Account,
	response *proxymodel.Response,
	keys *affinityKeys,
) (*proxymodel.Response, bool, bool, error) {
	if response == nil || request.WebSocketMessage || externalProviderKind(account.Provider.Kind) ||
		!inspectablePreviousResponse(response) {
		return response, false, false, nil
	}
	previous := requestPreviousResponseID(request.Body)
	if previous == "" {
		return response, false, false, nil
	}
	outcome, pending, err := router.classify(response, account.Provider.Kind)
	if err != nil {
		restoreResponsePrefix(response, pending)
		return response, false, false, nil
	}
	if outcome.previousResponseNotFound {
		response, outcome, pending, rotate, err := router.handleResponsesPreviousResponseNotFound(
			ctx, request, account, response, outcome, pending, keys,
		)
		if err != nil {
			return nil, false, false, err
		}
		if rotate {
			restoreResponsePrefix(response, pending)
			return response, true, true, nil
		}
		if !outcome.invalidPreviousResponseID {
			restoreResponsePrefix(response, pending)
			return response, outcome.failed, false, nil
		}
	}
	if !outcome.invalidPreviousResponseID {
		restoreResponsePrefix(response, pending)
		return response, outcome.failed, false, nil
	}
	if request.QuotaSelection.RouteKind != quotamodel.RouteKindResponses {
		restoreResponsePrefix(response, pending)
		return response, true, false, nil
	}
	owner, err := router.affinity.owner(ctx, affinityKeys{previous: previous}, router.now())
	if err != nil {
		pending.close()
		return nil, false, false, fmt.Errorf("check previous response owner: %w", err)
	}
	if owner != account.ID {
		restoreResponsePrefix(response, pending)
		return response, true, false, nil
	}
	if err := router.affinity.forgetDeadResponse(ctx, previous, owner, account.Home); err != nil {
		pending.close()
		return nil, false, false, err
	}
	if keys != nil {
		keys.previous = ""
	}

	retry, ok := fullHistoryRecoveryRequest(request)
	if !ok {
		restoreResponsePrefix(response, pending)
		return response, true, false, nil
	}
	pending.close()
	retried, err := router.execute(ctx, retry, account)
	if err != nil {
		return nil, false, false, err
	}
	if !inspectablePreviousResponse(retried) {
		return retried, false, false, nil
	}
	retryOutcome, retryPending, err := router.classify(retried, account.Provider.Kind)
	if err != nil {
		restoreResponsePrefix(retried, retryPending)
		return retried, false, false, nil
	}
	if retryOutcome.previousResponseNotFound {
		if err := router.notePreviousResponseNotFound(ctx, account, previous, request.QuotaSelection, keys); err != nil {
			retryPending.close()
			return nil, false, false, err
		}
	}
	restoreResponsePrefix(retried, retryPending)
	return retried, retryOutcome.failed, false, nil
}

func inspectablePreviousResponse(response *proxymodel.Response) bool {
	if response == nil {
		return false
	}
	if response.PrecommitFailure != nil || response.StatusCode == http.StatusBadRequest {
		return true
	}
	return response.StatusCode == http.StatusOK &&
		strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
}

func fullHistoryRecoveryRequest(request proxymodel.Request) (proxymodel.Request, bool) {
	var body map[string]json.RawMessage
	if json.Unmarshal(request.Body, &body) != nil || !requestHasSessionID(request.Header, body) {
		return proxymodel.Request{}, false
	}
	if requestPreviousResponseID(request.Body) == "" {
		return proxymodel.Request{}, false
	}
	var input []json.RawMessage
	if json.Unmarshal(body["input"], &input) != nil || !reconstructableFullHistory(input) {
		return proxymodel.Request{}, false
	}
	delete(body, "previous_response_id")
	recovered, err := json.Marshal(body)
	if err != nil {
		return proxymodel.Request{}, false
	}
	request.Body = recovered
	return request, true
}

func requestHasSessionID(headers http.Header, body map[string]json.RawMessage) bool {
	for _, name := range []string{"session_id", "session-id", "x-session-id"} {
		if strings.TrimSpace(headers.Get(name)) != "" {
			return true
		}
	}
	if sessionIDInMetadata(body) {
		return true
	}
	var turnMetadata map[string]json.RawMessage
	if json.Unmarshal([]byte(headers.Get("x-codex-turn-metadata")), &turnMetadata) == nil {
		return sessionIDInMetadata(turnMetadata)
	}
	return false
}

func sessionIDInMetadata(body map[string]json.RawMessage) bool {
	if jsonString(body["session_id"]) != "" {
		return true
	}
	var metadata map[string]json.RawMessage
	return json.Unmarshal(body["client_metadata"], &metadata) == nil && jsonString(metadata["session_id"]) != ""
}

func reconstructableFullHistory(input []json.RawMessage) bool {
	for index, raw := range input {
		var item map[string]json.RawMessage
		if json.Unmarshal(raw, &item) != nil {
			continue
		}
		if jsonString(item["type"]) == "compaction" && index+1 < len(input) {
			return true
		}
		if jsonString(item["role"]) != "user" {
			continue
		}
		for later := index + 1; later+1 < len(input); later++ {
			var next map[string]json.RawMessage
			if json.Unmarshal(input[later], &next) != nil {
				continue
			}
			if jsonString(next["role"]) == "assistant" || jsonString(next["type"]) == "function_call" {
				return true
			}
		}
	}
	return false
}

func requestPreviousResponseID(body []byte) string {
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil {
		return ""
	}
	return jsonString(request["previous_response_id"])
}

func jsonString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func invalidPreviousResponseID(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return invalidPreviousResponseValue(value)
}

func previousResponseNotFound(body []byte) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return responseHasCode(value, "previous_response_not_found")
}

func responseHasCode(value any, wanted string) bool {
	switch value := value.(type) {
	case map[string]any:
		if code, _ := value["code"].(string); strings.EqualFold(strings.TrimSpace(code), wanted) {
			return true
		}
		for _, child := range value {
			if responseHasCode(child, wanted) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if responseHasCode(child, wanted) {
				return true
			}
		}
	}
	return false
}

func invalidPreviousResponseValue(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		code, _ := value["code"].(string)
		errorType, _ := value["type"].(string)
		message, hasMessage := value["message"].(string)
		if !hasMessage {
			message, hasMessage = value["detail"].(string)
		}
		if !hasMessage {
			message, _ = value["error"].(string)
		}
		if !strings.EqualFold(strings.TrimSpace(code), "previous_response_not_found") &&
			strings.EqualFold(strings.TrimSpace(errorType), "invalid_request_error") &&
			strings.EqualFold(strings.TrimSpace(message), "invalid `previous_response_id`.") {
			return true
		}
		for _, child := range value {
			if invalidPreviousResponseValue(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if invalidPreviousResponseValue(child) {
				return true
			}
		}
	}
	return false
}

func restoreResponsePrefix(response *proxymodel.Response, pending *pendingResponse) {
	if response == nil || response.Body == nil || pending == nil || len(pending.prefix) == 0 {
		return
	}
	body := response.Body
	response.Body = &prefixedResponseBody{
		Reader: io.MultiReader(bytes.NewReader(pending.prefix), body), Closer: body,
	}
}

type prefixedResponseBody struct {
	io.Reader
	io.Closer
}
