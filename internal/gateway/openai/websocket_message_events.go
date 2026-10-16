package openai

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/christiandoxa/godex/internal/helper/websocketframe"
)

type websocketEvent struct {
	text                      bool
	kind                      string
	payload                   []byte
	frames                    []byte
	responseID                string
	turnState                 string
	retryCode                 string
	invalidPreviousResponseID bool
	terminal                  bool
	terminalReset             bool
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
			event.invalidPreviousResponseID = websocketInvalidPreviousResponseID(event.payload)
			event.retryCode = websocketRetryFailureCode(event.payload)
			if event.invalidPreviousResponseID {
				event.retryCode = "previous_response_not_found"
			}
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
	case 8:
		control, err = websocketframe.NormalizeClosePayload(control)
		if err != nil {
			return err
		}
		if err := websocketframe.WriteFrame(connection, 8, control, true); err != nil {
			return err
		}
		return io.EOF
	default:
		return errors.New("unsupported upstream websocket control frame")
	}
}

func websocketEventMetadata(payload []byte) (string, string, string) {
	var value map[string]any
	if json.Unmarshal(payload, &value) != nil {
		return "", "", ""
	}
	kind := jsonStringValue(value["type"])
	responseID := ""
	response, _ := value["response"].(map[string]any)
	if response != nil {
		responseID = jsonStringValue(response["id"])
	}
	if responseID == "" {
		responseID = jsonStringValue(value["response_id"])
	}
	if responseID == "" {
		object := jsonStringValue(value["object"])
		if object == "response" || strings.HasSuffix(object, ".response") {
			responseID = jsonStringValue(value["id"])
		}
	}
	if responseID == "" && strings.HasPrefix(kind, "response.") {
		responseID = jsonStringValue(value["id"])
	}
	turnState := ""
	if response != nil {
		turnState = websocketJSONHeader(response["headers"], "x-codex-turn-state")
	}
	if turnState == "" {
		turnState = websocketJSONHeader(value["headers"], "x-codex-turn-state")
	}
	if turnState == "" && response != nil {
		turnState = jsonStringValue(response["turn_state"])
	}
	if turnState == "" && response != nil {
		turnState = jsonStringValue(response["turnState"])
	}
	if turnState == "" {
		turnState = jsonStringValue(value["turn_state"])
	}
	if turnState == "" {
		turnState = jsonStringValue(value["turnState"])
	}
	return kind, strings.TrimSpace(responseID), strings.TrimSpace(turnState)
}

func websocketJSONHeader(headers any, wanted string) string {
	switch typed := headers.(type) {
	case map[string]any:
		for name, raw := range typed {
			if strings.EqualFold(strings.TrimSpace(name), wanted) {
				return websocketJSONHeaderValue(raw)
			}
		}
	case []any:
		for _, item := range typed {
			var name, value any
			switch entry := item.(type) {
			case []any:
				if len(entry) >= 2 {
					name, value = entry[0], entry[1]
				}
			case map[string]any:
				name = entry["name"]
				if name == nil {
					name = entry["key"]
				}
				value = entry["value"]
				if value == nil {
					value = entry["values"]
				}
			}
			if strings.EqualFold(strings.TrimSpace(jsonStringValue(name)), wanted) {
				if result := websocketJSONHeaderValue(value); result != "" {
					return result
				}
			}
		}
	}
	return ""
}

func websocketJSONHeaderValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		for _, item := range typed {
			if result := websocketJSONHeaderValue(item); result != "" {
				return result
			}
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
	case "keepalive", "codex.rate_limits", "codex.response.metadata", "response.metadata", "response.created",
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

// Tagged Prodex 0.438.0 uses serde_json::Value::as_u64 followed by u16
// conversion. A *present but invalid* `status` must mask `status_code`; a
// fractional/string/negative/out-of-u16 value must not become an HTTP error
// or authorize an otherwise forbidden WebSocket retry.
func websocketPayloadStatus(payload []byte) int {
	var value map[string]json.RawMessage
	if json.Unmarshal(payload, &value) != nil {
		return 0
	}
	raw, present := value["status"]
	if !present {
		raw = value["status_code"]
	}
	if len(raw) == 0 {
		return 0
	}
	// ParseUint rejects fractional/exponential, quoted, signed and null
	// values rather than truncating a Go JSON float.
	status, err := strconv.ParseUint(string(raw), 10, 16)
	if err != nil {
		return 0
	}
	return int(status)
}

func websocketRetryFailureCode(payload []byte) string {
	if isWebSocketConnectionLimit(payload) {
		return "websocket_connection_limit_reached"
	}
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
	// A status-shaped field on a model output or heartbeat is not an
	// upstream error. Only untyped or explicitly failing response events
	// may fall back to a 5xx status in the absence of a retryable code.
	kind, _, _ := websocketEventMetadata(payload)
	if kind != "" && kind != "error" && kind != "response.failed" && kind != "response.incomplete" {
		return ""
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

// The tagged Mojo kernel checks nested error.code AND top-level code
// independently, without whitespace normalization and with ASCII-only byte
// case folding. A malformed Unicode lookalike is never a real limit code.
func isWebSocketConnectionLimit(payload []byte) bool {
	var value map[string]json.RawMessage
	if json.Unmarshal(payload, &value) != nil {
		return false
	}
	const limit = "websocket_connection_limit_reached"
	var top string
	if json.Unmarshal(value["code"], &top) == nil && websocketASCIIEqualFold04380(top, limit) {
		return true
	}
	// Other fields can carry arbitrary JSON. In particular, a non-object
	// error or an error.code with the wrong type cannot suppress a valid
	// top-level code. Match tagged serde_json::Value::as_str semantics.
	var nested map[string]json.RawMessage
	if json.Unmarshal(value["error"], &nested) != nil {
		return false
	}
	var code string
	return json.Unmarshal(nested["code"], &code) == nil && websocketASCIIEqualFold04380(code, limit)
}

func websocketASCIIEqualFold04380(value, expected string) bool {
	if len(value) != len(expected) {
		return false
	}
	for index := 0; index < len(value); index++ {
		ch := value[index]
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		if ch != expected[index] {
			return false
		}
	}
	return true
}
