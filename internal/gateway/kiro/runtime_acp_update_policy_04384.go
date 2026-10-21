package kiro

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// The tagged Prodex 0.438.4 Rust turn_state builder first calls
// parse_session_notification, which delegates the mandatory field gate to
// the compiled Mojo Kiro ACP policy. Parse failures are ignored (not
// replayed, not made into synthetic tool activity, not turned into usage).
// This source-derived validator intentionally operates on the original
// notification, before the ordinary Go presentation/projector helpers.
func kiroACPValidatedUpdate04384(envelope acpEnvelope) (map[string]any, bool) {
	if envelope.Method != "session/update" || len(envelope.Params) == 0 {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Params))
	decoder.UseNumber()
	var params map[string]any
	if err := decoder.Decode(&params); err != nil || params == nil {
		return nil, false
	}
	if _, ok := params["sessionId"].(string); !ok {
		return nil, false
	}
	update, ok := params["update"].(map[string]any)
	if !ok {
		return nil, false
	}
	kind, ok := update["sessionUpdate"].(string)
	if !ok {
		return nil, false
	}
	switch kind {
	case "user_message_chunk", "agent_message_chunk", "agent_thought_chunk":
		_, ok = update["content"]
	case "tool_call":
		_, ok = update["toolCallId"].(string)
		if ok {
			_, ok = update["title"].(string)
		}
	case "tool_call_update":
		_, ok = update["toolCallId"].(string)
	case "plan":
		ok = kiroACPPlanEntries04384(update["entries"])
	case "usage_update":
		ok = kiroACPUint64Value04384(update["used"]) && kiroACPUint64Value04384(update["size"])
		if ok {
			if cost, present := update["cost"]; present && cost != nil {
				ok = kiroACPCost04384(cost)
			}
		}
	case "available_commands_update":
		_, ok = update["availableCommands"].([]any)
	case "current_mode_update":
		_, ok = update["currentModeId"].(string)
	default:
		// Unknown update kinds are carried as an opaque enum variant by
		// tagged Rust. The turn state builder performs no mutation.
		ok = true
	}
	if !ok {
		return nil, false
	}
	return update, true
}

func kiroACPUint64Value04384(value any) bool {
	number, ok := value.(json.Number)
	if !ok {
		return false
	}
	text := string(number)
	if text == "" || strings.ContainsAny(text, ".eE+-") {
		return false
	}
	_, err := strconv.ParseUint(text, 10, 64)
	return err == nil
}

func kiroACPPlanEntries04384(value any) bool {
	entries, ok := value.([]any)
	if !ok {
		return false
	}
	for _, item := range entries {
		entry, valid := item.(map[string]any)
		if !valid {
			return false
		}
		for _, field := range []string{"content", "priority", "status"} {
			if _, valid := entry[field].(string); !valid {
				return false
			}
		}
	}
	return true
}

func kiroACPCost04384(value any) bool {
	cost, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if _, ok := cost["currency"].(string); !ok {
		return false
	}
	number, ok := cost["amount"].(json.Number)
	if !ok {
		return false
	}
	_, err := strconv.ParseFloat(string(number), 64)
	return err == nil
}
