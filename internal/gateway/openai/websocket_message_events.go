package openai

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type websocketEvent struct {
	text          bool
	kind          string
	payload       []byte
	frames        []byte
	responseID    string
	turnState     string
	retryCode     string
	terminal      bool
	terminalReset bool
}

func readWebSocketEvent(connection io.ReadWriteCloser) (websocketEvent, error) {
	var event websocketEvent
	var messageOpcode byte
	var messageBytes uint64
	messageStarted := false
	for {
		frame, err := websocketframe.ReadHeader(connection)
		if err != nil {
			return websocketEvent{}, err
		}
		if frame.Header[0]&0x70 != 0 || frame.Masked() {
			return websocketEvent{}, errors.New("invalid upstream websocket server frame")
		}
		if frame.PayloadLength > websocketUpstreamMaxFrameBytes {
			return websocketEvent{}, errors.New("upstream websocket frame exceeds protocol size limit")
		}
		if frame.Opcode >= 8 {
			if err := handleUpstreamWebSocketControl(connection, frame); err != nil {
				return websocketEvent{}, err
			}
			continue
		}
		if !messageStarted {
			if frame.Opcode != 1 && frame.Opcode != 2 {
				return websocketEvent{}, errors.New("unexpected upstream websocket continuation frame")
			}
			messageStarted, messageOpcode = true, frame.Opcode
		} else if frame.Opcode != 0 {
			return websocketEvent{}, errors.New("invalid fragmented upstream websocket message")
		}
		if messageBytes > websocketUpstreamMaxMessageBytes-frame.PayloadLength {
			return websocketEvent{}, errors.New("upstream websocket message exceeds protocol size limit")
		}
		messageBytes += frame.PayloadLength
		payload, err := frame.ReadPayload(connection, frame.PayloadLength)
		if err != nil {
			return websocketEvent{}, err
		}
		event.frames = append(event.frames, frame.Header...)
		event.frames = append(event.frames, payload...)
		event.payload = append(event.payload, payload...)
		if !frame.Final {
			continue
		}
		if messageOpcode == 1 {
			event.text = true
			if !utf8.Valid(event.payload) {
				return websocketEvent{}, errors.New("upstream websocket text message is not valid UTF-8")
			}
			event.kind, event.responseID, event.turnState = websocketEventMetadata(event.payload)
			event.retryCode = websocketRetryFailureCode(event.payload)
			event.terminal = websocketPayloadTerminal(event.kind, event.payload)
			event.terminalReset = event.terminal && websocketTerminalShouldReset(event.kind)
		}
		return event, nil
	}
}

func handleUpstreamWebSocketControl(connection io.ReadWriteCloser, frame websocketframe.Frame) error {
	if !frame.Final || frame.PayloadLength > 125 {
		return errors.New("invalid upstream websocket control frame")
	}
	control, err := frame.ReadPayload(connection, 125)
	if err != nil {
		return err
	}
	switch frame.Opcode {
	case 9:
		return websocketframe.WriteFrame(connection, 10, control, true)
	case 10:
		return nil
	default:
		return io.ErrUnexpectedEOF
	}
}

func websocketEventMetadata(payload []byte) (string, string, string) {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil {
		return "", "", ""
	}
	kind := jsonStringValue(value["type"])
	responseID := ""
	turnState := jsonStringValue(value["turn_state"])
	if response, ok := value["response"].(map[string]any); ok {
		responseID = jsonStringValue(response["id"])
		if turnState == "" {
			turnState = jsonStringValue(response["turn_state"])
		}
		if turnState == "" {
			if headers, ok := response["headers"].(map[string]any); ok {
				turnState = websocketJSONHeader(headers, "x-codex-turn-state")
			}
		}
	}
	if responseID == "" {
		responseID = jsonStringValue(value["response_id"])
	}
	if responseID == "" && strings.HasPrefix(kind, "response.") {
		responseID = jsonStringValue(value["id"])
	}
	return kind, strings.TrimSpace(responseID), strings.TrimSpace(turnState)
}

func websocketJSONHeader(headers map[string]any, wanted string) string {
	for name, raw := range headers {
		if strings.EqualFold(strings.TrimSpace(name), wanted) {
			return jsonStringValue(raw)
		}
	}
	return ""
}

func jsonStringValue(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func isWebSocketPrecommitHold(kind string) bool {
	switch kind {
	case "codex.rate_limits", "codex.response.metadata", "response.metadata", "response.created",
		"response.in_progress", "response.queued", "response.output_item.added",
		"response.content_part.added", "response.reasoning_summary_part.added":
		return true
	default:
		return false
	}
}

func isWebSocketTerminal(kind string) bool {
	switch kind {
	case "response.completed", "response.failed", "response.incomplete":
		return true
	default:
		return false
	}
}

func websocketPayloadTerminal(kind string, payload []byte) bool {
	if isWebSocketTerminal(kind) {
		return true
	}
	if kind != "error" {
		return false
	}
	if isWebSocketConnectionLimit(payload) {
		return true
	}
	return websocketPayloadStatus(payload) >= 400
}

func websocketPayloadStatus(payload []byte) int {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil {
		return 0
	}
	for _, key := range []string{"status", "status_code"} {
		switch raw := value[key].(type) {
		case float64:
			if raw >= 0 && raw <= 999 {
				return int(raw)
			}
		case json.Number:
			if parsed, err := raw.Int64(); err == nil && parsed >= 0 && parsed <= 999 {
				return int(parsed)
			}
		}
	}
	return 0
}

func websocketRetryFailureCode(payload []byte) string {
	code := strings.ToLower(strings.TrimSpace(websocketFailureCode(payload)))
	switch code {
	case "websocket_connection_limit_reached",
		"previous_response_not_found",
		"insufficient_quota",
		"credit_balance_exhausted",
		"organization_spend_limit_exceeded",
		"project_spend_limit_exceeded",
		"quota_exhausted",
		"quota_exceeded",
		"resource_exhausted",
		"usage_limit_reached",
		"usage_not_included",
		"workspace_member_credits_depleted",
		"rate_limit_exceeded",
		"rate_limit_exceeded_error",
		"slow_down",
		"deactivated_workspace",
		"server_is_overloaded":
		return code
	}
	status := websocketPayloadStatus(payload)
	if status >= 500 && status <= 599 {
		return "server_is_overloaded"
	}
	return ""
}

func websocketFailureCode(payload []byte) string {
	var event struct {
		Code  string `json:"code"`
		Error struct {
			Type string `json:"type"`
			Code string `json:"code"`
		} `json:"error"`
		Response struct {
			Error struct {
				Type string `json:"type"`
				Code string `json:"code"`
			} `json:"error"`
		} `json:"response"`
	}
	if json.Unmarshal(payload, &event) != nil {
		return ""
	}
	for _, code := range []string{event.Error.Code, event.Response.Error.Code, event.Code, event.Error.Type, event.Response.Error.Type} {
		if code != "" {
			return code
		}
	}
	return ""
}

func isWebSocketConnectionLimit(payload []byte) bool {
	return strings.EqualFold(
		strings.TrimSpace(websocketFailureCode(payload)),
		"websocket_connection_limit_reached",
	)
}
