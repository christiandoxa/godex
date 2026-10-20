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
		// The tagged 0.438.3 HTTP planner fails closed when a verified
		// response chain is rejected by its owner. A fresh alternate cannot
		// safely use the same previous_response_id: that would silently
		// replay an unavailable chain on another managed account.
		verifiedOwner := false
		if request.QuotaSelection.RouteKind == quotamodel.RouteKindResponses {
			owner, lookupErr := router.affinity.owner(ctx, affinityKeys{previous: previous}, router.now())
			if lookupErr != nil {
				pending.close()
				return nil, false, false, lookupErr
			}
			verifiedOwner = owner == account.ID
		}
		response, outcome, pending, rotate, err := router.handleResponsesPreviousResponseNotFound(
			ctx, request, account, response, outcome, pending, keys,
		)
		if err != nil {
			return nil, false, false, err
		}
		if rotate {
			if verifiedOwner {
				pending.close()
				return staleResponsesContinuationResponse(), true, false, nil
			}
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
		(strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") ||
			(response.RequestedStreaming && strings.TrimSpace(response.Header.Get("Content-Type")) == ""))
}

func fullHistoryRecoveryRequest(request proxymodel.Request) (proxymodel.Request, bool) {
	// The tagged runtime computes recovery shape from a complete
	// serde_json::Value. Go's RawMessage alone admits invalid Unicode,
	// non-finite floats and too-deep JSON; none may enable a replay.
	if _, valid := decodeNativeUsageJSON(request.Body); !valid {
		return proxymodel.Request{}, false
	}
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
	rawMetadata := []byte(headers.Get("x-codex-turn-metadata"))
	if _, valid := decodeNativeUsageJSON(rawMetadata); valid &&
		json.Unmarshal(rawMetadata, &turnMetadata) == nil {
		return sessionIDInMetadata(turnMetadata)
	}
	return false
}

func sessionIDInMetadata(body map[string]json.RawMessage) bool {
	// Tagged runtime_request_semantic_plan uses member presence. A direct
	// null, blank or non-string session_id shadows client_metadata and
	// must not authorize replay of a previous-response-dependent turn.
	if direct, present := body["session_id"]; present {
		return jsonString(direct) != ""
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
		if exactJSONTag(item["type"], "compaction") && index+1 < len(input) {
			return true
		}
		if !exactJSONTag(item["role"], "user") {
			continue
		}
		for later := index + 1; later+1 < len(input); later++ {
			var next map[string]json.RawMessage
			if json.Unmarshal(input[later], &next) != nil {
				continue
			}
			if exactJSONTag(next["role"], "assistant") || exactJSONTag(next["type"], "function_call") {
				return true
			}
		}
	}
	return false
}

func exactJSONTag(raw json.RawMessage, expected string) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value == expected
}

func requestPreviousResponseID(body []byte) string {
	if _, valid := decodeNativeUsageJSON(body); !valid {
		return ""
	}
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
