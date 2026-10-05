package routing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	websocketStaleContinuationMessage = "Upstream no longer recognizes this conversation chain before output started. Retry from the last user message or restart the Codex turn; Prodex will not send a fresh request without the missing context."
	websocketFailureMessageMaxBytes   = 64 << 20
)

func staleWebSocketContinuationResponse(response *proxymodel.Response) *proxymodel.Response {
	payload := readWebSocketPrecommitTextPayload(response)
	closeWebSocketRoutingResponse(response)
	translated := translateStaleWebSocketPayload(payload)
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
	var event map[string]any
	if json.Unmarshal(payload, &event) == nil &&
		strings.EqualFold(strings.TrimSpace(websocketJSONText(event["type"])), "response.failed") {
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
