package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"

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

func invalidPreviousResponseID(body []byte) bool {
	// Tagged runtime_proxy_body_is_invalid_previous_response_id only
	// accepts structured error evidence from a complete serde_json::Value.
	// Go's json.Unmarshal otherwise silently repairs invalid Unicode and
	// admits over-depth JSON before releasing a hard previous owner.
	value, valid := decodeNativeUsageJSON(body)
	return valid && invalidPreviousResponseValue(value)
}

func previousResponseNotFound(body []byte) bool {
	// Tagged extract_runtime_proxy_previous_response_message tries the
	// complete serde_json::Value first, then falls back to a narrowly
	// recognized *plain-text* previous-response error. Malformed JSON
	// cannot manufacture structured ownership evidence, but a literal
	// `previous_response_not_found` upstream text error remains meaningful.
	if value, valid := decodeNativeUsageJSON(body); valid && previousResponseMessageInJSON04383(value) {
		return true
	}
	if !utf8.Valid(body) {
		return false
	}
	return previousResponseMessageInText04383(strings.TrimSpace(string(body)))
}

func previousResponseMessageInText04383(value string) bool {
	lower := strings.ToLower(value)
	if lower == "invalid `previous_response_id`." || strings.HasPrefix(lower, "previous_response_not_found") {
		return true
	}
	if strings.HasPrefix(lower, "previous response") && strings.Contains(lower, "not found") {
		return true
	}
	if (strings.Contains(lower, "no tool call found") || strings.Contains(lower, "no function call found")) &&
		(strings.HasPrefix(lower, "invalid_request_error:") || strings.HasPrefix(lower, "no tool call found") ||
			strings.HasPrefix(lower, "no function call found")) {
		return true
	}
	return false
}

const previousResponseJSONScanLimit04383 = 2048

func previousResponseMessageInJSON04383(value any) bool {
	// Rust runtime_json_find searches no more than 2048 JSON nodes and
	// serde_json::Map visits keys lexically. Preserve its first-candidate
	// traversal rather than scanning an unbounded randomized Go map.
	stack := []any{value}
	for visited := 0; len(stack) > 0 && visited < previousResponseJSONScanLimit04383; visited++ {
		index := len(stack) - 1
		current := stack[index]
		stack = stack[:index]
		switch current := current.(type) {
		case map[string]any:
			code, _ := current["code"].(string)
			if strings.EqualFold(code, "previous_response_not_found") {
				return true
			}
			errorType, _ := current["type"].(string)
			parameter, _ := current["param"].(string)
			message, _ := current["message"].(string)
			if _, ok := current["message"].(string); !ok {
				message, _ = current["detail"].(string)
				if _, ok := current["detail"].(string); !ok {
					message, _ = current["error"].(string)
				}
			}
			if strings.EqualFold(errorType, "invalid_request_error") && strings.EqualFold(strings.TrimSpace(message), "invalid `previous_response_id`.") {
				return true
			}
			lower := strings.ToLower(message)
			if (strings.Contains(lower, "no tool call found") || strings.Contains(lower, "no function call found")) &&
				(strings.EqualFold(errorType, "invalid_request_error") || strings.EqualFold(parameter, "input")) {
				return true
			}
			keys := make([]string, 0, len(current))
			for key := range current {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for i := len(keys) - 1; i >= 0; i-- {
				stack = append(stack, current[keys[i]])
			}
		case []any:
			for i := len(current) - 1; i >= 0; i-- {
				stack = append(stack, current[i])
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
