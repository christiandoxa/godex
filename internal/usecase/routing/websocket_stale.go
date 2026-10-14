package routing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	staleContinuationMessage           = "Upstream no longer recognizes this conversation chain before output started. Retry from the last user message or restart the Codex turn; Prodex will not send a fresh request without the missing context."
	websocketStaleContinuationMessage  = staleContinuationMessage
	websocketFailureMessageMaxBytes    = 64 << 20
	websocketPreviousResponseScanLimit = 2048
)

func staleWebSocketContinuationResponse(response *proxymodel.Response) *proxymodel.Response {
	return staleWebSocketContinuationWithEnvelope(response, true)
}

// Prodex 0.437.9 sends a generic 409 error for a missing unbound chain
// or for a locked tool-output continuation, even when the owner is known.
// Ordinary verified response continuations retain their original envelope.
func staleGenericWebSocketContinuationResponse(response *proxymodel.Response) *proxymodel.Response {
	return staleWebSocketContinuationWithEnvelope(response, false)
}

func staleWebSocketContinuationWithEnvelope(response *proxymodel.Response, preserveEnvelope bool) *proxymodel.Response {
	payload := readWebSocketPrecommitTextPayload(response)
	closeWebSocketRoutingResponse(response)
	translated := translateStaleWebSocketPayload(payload)
	if !preserveEnvelope {
		translated = marshalStaleWebSocketPayload(map[string]any{
			"type":   "error",
			"status": http.StatusConflict,
			"error":  staleWebSocketErrorObject(),
		})
	}
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	return &proxymodel.Response{
		StatusCode: http.StatusConflict,
		Header:     headers,
		Body:       io.NopCloser(bytes.NewReader(translated)),
		PrecommitFailure: &proxymodel.PrecommitFailure{
			Code: "previous_response_not_found", StaleContinuation: true,
		},
		FirstEventCommitted: true,
	}
}

func readWebSocketPrecommitTextPayload(response *proxymodel.Response) []byte {
	if response == nil || response.Body == nil {
		return nil
	}
	var payload []byte
	var messageBytes uint64
	started := false
	for {
		frame, err := websocketframe.ReadHeader(response.Body)
		if err != nil {
			return nil
		}
		if frame.Masked() || frame.Header[0]&0x70 != 0 || frame.PayloadLength > 16<<20 {
			return nil
		}
		switch {
		case !started && frame.Opcode == 1:
			started = true
		case started && frame.Opcode == 0:
		default:
			return nil
		}
		if messageBytes > websocketFailureMessageMaxBytes-frame.PayloadLength {
			return nil
		}
		messageBytes += frame.PayloadLength
		chunk, err := frame.ReadPayload(response.Body, frame.PayloadLength)
		if err != nil {
			return nil
		}
		payload = append(payload, chunk...)
		if frame.Final {
			return payload
		}
	}
}

func translateStaleWebSocketPayload(payload []byte) []byte {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		if !utf8.Valid(payload) || !websocketPreviousResponseMarkerText(string(payload)) {
			return payload
		}
		return marshalStaleWebSocketPayload(map[string]any{
			"type":   "error",
			"status": http.StatusConflict,
			"error":  staleWebSocketErrorObject(),
		})
	}
	if !websocketPreviousResponseMarkerValue(value) {
		return payload
	}
	event, _ := value.(map[string]any)
	if strings.EqualFold(strings.TrimSpace(websocketJSONText(event["type"])), "response.failed") {
		if _, nested := event["response"]; nested {
			return marshalStaleWebSocketPayload(map[string]any{
				"type":   "response.failed",
				"status": http.StatusConflict,
				"response": map[string]any{
					"error": staleWebSocketErrorObject(),
				},
			})
		}
		return marshalStaleWebSocketPayload(map[string]any{
			"type":   "response.failed",
			"status": http.StatusConflict,
			"error":  staleWebSocketErrorObject(),
		})
	}
	return marshalStaleWebSocketPayload(map[string]any{
		"type":   "error",
		"status": http.StatusConflict,
		"error":  staleWebSocketErrorObject(),
	})
}

// Prodex only rewrites a pre-commit frame after its payload identifies a
// missing previous response. Forward unrelated upstream errors byte-for-byte.
func websocketPreviousResponseMarkerValue(value any) bool {
	stack := []any{value}
	for visited := 0; len(stack) > 0 && visited < websocketPreviousResponseScanLimit; visited++ {
		index := len(stack) - 1
		current := stack[index]
		stack = stack[:index]
		switch current := current.(type) {
		case map[string]any:
			if code, ok := current["code"].(string); ok &&
				strings.EqualFold(code, "previous_response_not_found") {
				return true
			}
			errorType, _ := current["type"].(string)
			invalidRequest := strings.EqualFold(errorType, "invalid_request_error")
			message := ""
			for _, key := range []string{"message", "detail", "error"} {
				if candidate, ok := current[key].(string); ok {
					message = candidate
					break
				}
			}
			if websocketPreviousResponseMarkerMessage(message, current["param"], invalidRequest) {
				return true
			}
			for _, child := range current {
				stack = append(stack, child)
			}
		case []any:
			stack = append(stack, current...)
		}
	}
	return false
}

func websocketPreviousResponseMarkerText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "previous_response_not_found") ||
		(strings.HasPrefix(lower, "previous response") && strings.Contains(lower, "not found")) ||
		strings.EqualFold(trimmed, "invalid `previous_response_id`.") ||
		(strings.Contains(lower, "no tool call found") &&
			(strings.HasPrefix(lower, "invalid_request_error:") || strings.HasPrefix(lower, "no tool call found"))) ||
		(strings.Contains(lower, "no function call found") &&
			(strings.HasPrefix(lower, "invalid_request_error:") || strings.HasPrefix(lower, "no function call found")))
}

func websocketPreviousResponseMarkerMessage(message string, param any, invalidRequest bool) bool {
	trimmed := trimWebSocketASCIIWhitespace(message)
	if strings.EqualFold(trimmed, "invalid `previous_response_id`.") {
		return true
	}
	lower := strings.ToLower(message)
	toolContext := strings.Contains(lower, "no tool call found") ||
		strings.Contains(lower, "no function call found")
	if !toolContext || (!invalidRequest && param == nil) {
		return false
	}
	parameter, _ := param.(string)
	return invalidRequest || strings.EqualFold(parameter, "input")
}

func trimWebSocketASCIIWhitespace(value string) string {
	return strings.Trim(value, " \t\n\r")
}

func staleWebSocketErrorObject() map[string]any {
	return map[string]any{
		"code":    "stale_continuation",
		"message": websocketStaleContinuationMessage,
	}
}

func marshalStaleWebSocketPayload(value map[string]any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(errors.New("marshal static stale websocket payload"))
	}
	return payload
}

func websocketJSONText(value any) string {
	text, _ := value.(string)
	return text
}
