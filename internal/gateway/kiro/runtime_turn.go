package kiro

import (
	"encoding/json"
	"fmt"
	"github.com/christiandoxa/godex/internal/helper/jsonutil"
	"strconv"
	"strings"
	"time"
)

const maxToolActivityEvents = 128

type kiroTurnState struct {
	assistantText     string
	reasoningText     string
	usage             map[string]any
	plan              []any
	availableCommands []any
	currentModeID     string
	currentModeSet    bool
	sessionTitle      string
	sessionTitleSet   bool
	sessionUpdatedAt  string
	sessionUpdatedSet bool
	toolActivities    []any
	toolLabels        map[string]toolLabel
	truncatedTools    bool
}

type toolLabel struct {
	name string
	kind string
}

func kiroResponseFromTurn(turn acpTurn, requestID uint64, requestedModel, profileName string) map[string]any {
	state := collectKiroTurnState(turn.Notifications)
	model := "kiro-cli"
	if turn.Session.Models != nil && strings.TrimSpace(turn.Session.Models.CurrentModelID) != "" {
		model = strings.TrimSpace(turn.Session.Models.CurrentModelID)
	}
	output := make([]any, 0, 1)
	if state.assistantText != "" {
		output = append(output, map[string]any{
			"type": kiroFieldMessage, "role": "assistant",
			kiroFieldContent: []any{map[string]any{"type": "output_text", "text": state.assistantText}},
		})
	}
	response := map[string]any{
		"id":         fmt.Sprintf("resp_kiro_%d", requestID),
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      model,
		"output":     output,
	}
	if turn.Prompt.Error != nil {
		response[kiroFieldStatus] = "failed"
		response["error"] = map[string]any{"code": fmt.Sprintf("%d", turn.Prompt.Error.Code), kiroFieldMessage: turn.Prompt.Error.Message}
	} else if reason, message := kiroIncompleteDetails(turn.Prompt); reason != "" {
		response[kiroFieldStatus] = "incomplete"
		response["incomplete_details"] = map[string]any{"reason": reason, kiroFieldMessage: message}
	}
	metadata := state.metadata(kiroStopReason(turn.Prompt))
	if profileName != "" {
		kiro, _ := metadata["kiro"].(map[string]any)
		if kiro == nil {
			kiro = make(map[string]any)
			metadata["kiro"] = kiro
		}
		kiro["profile_name"] = profileName
	}
	if len(metadata) > 0 {
		response["metadata"] = metadata
	}
	if requestedModel = strings.TrimSpace(requestedModel); requestedModel != "" {
		response["requested_model"] = requestedModel
	}
	return response
}

func collectKiroTurnState(notifications []acpEnvelope) kiroTurnState {
	state := kiroTurnState{toolLabels: make(map[string]toolLabel)}
	for _, envelope := range notifications {
		if envelope.Method != "session/update" || len(envelope.Params) == 0 {
			continue
		}
		update, valid := kiroACPValidatedUpdate04384(envelope)
		if !valid {
			continue
		}
		state.applyUpdate(update)
	}
	return state
}

func (state *kiroTurnState) applyUpdate(update map[string]any) {
	// Mojo classifies the exact update discriminator before content
	// processing; padding cannot turn an unknown variant into tool_call.
	kind, _ := update["sessionUpdate"].(string)
	switch kind {
	case "agent_message_chunk":
		state.assistantText += acpContentText(update[kiroFieldContent])
	case "agent_thought_chunk":
		state.reasoningText += acpContentText(update[kiroFieldContent])
	case "usage_update":
		state.usage = kiroUsageUpdate(update)
	case "plan":
		state.plan = sanitizePlanEntries(update["entries"])
	case "available_commands_update":
		state.availableCommands = sanitizeArray(update["availableCommands"])
	case "current_mode_update":
		// Tagged Rust stores Some("") as a real update; the Mojo
		// metadata kernel serializes an explicit empty mode as "".
		state.currentModeID, _ = update["currentModeId"].(string)
		state.currentModeSet = true
	case "session_info_update":
		if title, present := update[kiroFieldTitle].(string); present {
			state.sessionTitle = title
			state.sessionTitleSet = true
		}
		if updated, present := update["updatedAt"].(string); present {
			state.sessionUpdatedAt = updated
			state.sessionUpdatedSet = true
		}
	case "tool_call", "tool_call_update":
		state.applyToolActivity(kind == "tool_call", update)
	}
}

func (state *kiroTurnState) applyToolActivity(initial bool, update map[string]any) {
	if state.truncatedTools {
		return
	}
	id, _ := update["toolCallId"].(string)
	previous, previousPresent := state.toolLabels[id]
	// Tagged Mojo uses *Option field presence* to decide whether to reuse
	// an earlier label. Present empty strings suppress inheritance even
	// though the later presentation sanitizer may choose a fallback name.
	title, titlePresent := update[kiroFieldTitle].(string)
	if !titlePresent && previousPresent && len(id) <= 256 {
		title = previous.name
	}
	kind, kindPresent := update["kind"].(string)
	if !kindPresent && previousPresent && len(id) <= 256 {
		kind = previous.kind
	}
	status := runtimeString(update[kiroFieldStatus], "")
	// Tagged Rust treats raw_input/raw_output as Option<Value>: even a
	// present JSON null is Some(Null) and requires detail omission.
	// content/locations are Option<Vec<Value>> and only NONEMPTY arrays
	// trigger the same flag; empty arrays and unrelated JSON types do not.
	_, rawInputPresent := update["rawInput"]
	_, rawOutputPresent := update["rawOutput"]
	content, _ := update[kiroFieldContent].([]any)
	locations, _ := update["locations"].([]any)
	detailsOmitted := rawInputPresent || rawOutputPresent ||
		len(content) > 0 || len(locations) > 0
	if len(state.toolActivities) >= maxToolActivityEvents-1 {
		item := kiroTruncatedActivity()
		state.toolActivities = append(state.toolActivities, item)
		state.assistantText += kiroActivityText(item)
		state.truncatedTools = true
		return
	}
	item := kiroActivityItem(title, status, kind, initial, detailsOmitted)
	state.toolActivities = append(state.toolActivities, item)
	state.assistantText += kiroActivityText(item)
	// A zero-length string toolCallId remains a legitimate label key;
	// the published Mojo ledger only caps its UTF-8 byte length at 256.
	if len(id) <= 256 && (previousPresent || len(state.toolLabels) < maxToolActivityEvents) {
		name, _ := item["name"].(string)
		safeKind, _ := item["kind"].(string)
		state.toolLabels[id] = toolLabel{name: name, kind: safeKind}
	}
}

func (state kiroTurnState) metadata(stopReason string) map[string]any {
	kiro := make(map[string]any)
	if state.reasoningText != "" {
		kiro["reasoning_content"] = state.reasoningText
	}
	if state.usage != nil {
		kiro["usage_update"] = state.usage
	}
	// In Rust these are Options: Some([]) and Some("") must still be
	// serialized. Absence, not the value's length, suppresses a field.
	if state.plan != nil {
		kiro["plan"] = state.plan
	}
	if state.availableCommands != nil {
		kiro["available_commands"] = state.availableCommands
	}
	if state.currentModeSet {
		kiro["current_mode_id"] = state.currentModeID
	}
	if state.sessionTitleSet || state.sessionUpdatedSet {
		var title, updated any
		if state.sessionTitleSet {
			title = state.sessionTitle
		}
		if state.sessionUpdatedSet {
			updated = state.sessionUpdatedAt
		}
		kiro["session_info"] = map[string]any{kiroFieldTitle: title, "updated_at": updated}
	}
	if stopReason != "" {
		kiro["stop_reason"] = stopReason
	}
	if len(state.toolActivities) > 0 {
		kiro["tool_activities"] = state.toolActivities
	}
	if len(kiro) == 0 {
		return nil
	}
	return map[string]any{"kiro": kiro}
}

func acpContentText(value any) string {
	return acpContentTextDepth04384(value, 0)
}

// Published 0.438.4 kiro_json_write_content_value accepts direct strings,
// arrays of values, or nested `content` objects. A direct string `text`
// wins over the nested member when both are present. Its Mojo walker caps
// recursion at 128 steps, so a malformed deeply nested structure cannot
// grow Go's call stack without bound.
func acpContentTextDepth04384(value any, depth int) string {
	if depth > 128 {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return text
		}
		if nested, present := typed["content"]; present {
			return acpContentTextDepth04384(nested, depth+1)
		}
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			builder.WriteString(acpContentTextDepth04384(item, depth+1))
		}
		return builder.String()
	}
	return ""
}

func kiroUsageUpdate(update map[string]any) map[string]any {
	used := runtimeUint(update["used"])
	size := runtimeUint(update["size"])
	remaining := uint64(0)
	if size > used {
		remaining = size - used
	}
	// Tagged kiro_provider_core_acp_usage_update_json always serializes
	// its optional cost field: None becomes `"cost":null`, not omission.
	result := map[string]any{"used": used, "size": size, "remaining": remaining, "cost": nil}
	if cost, ok := update["cost"].(map[string]any); ok {
		projected := sanitizeMap(cost, "amount", "currency")
		if amount, ok := cost["amount"].(json.Number); ok {
			// Prodex decodes the ACP cost as a typed f64 and emits its
			// canonical serde_json spelling, NOT the original JSON number
			// lexeme (5 must become 5.0, 1e16 must become 1e+16).
			if parsed, valid := jsonutil.ParseSerdeDefaultFloat(string(amount)); valid {
				projected["amount"] = json.Number(jsonutil.FormatSerdeDefaultFloat(parsed))
			}
		}
		result["cost"] = projected
	}
	return result
}

func sanitizePlanEntries(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]any, 0, len(items))
	for _, raw := range items {
		if item, ok := raw.(map[string]any); ok {
			result = append(result, sanitizeMap(item, kiroFieldContent, "priority", kiroFieldStatus))
		}
	}
	return result
}

func sanitizeArray(value any) []any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	return append([]any{}, items...)
}

func sanitizeMap(value map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if current, found := value[key]; found {
			result[key] = current
		}
	}
	return result
}

func kiroStopReason(envelope acpEnvelope) string {
	var result map[string]any
	if json.Unmarshal(envelope.Result, &result) != nil {
		return ""
	}
	for _, key := range []string{"stopReason", "stop_reason", kiroFieldStatus} {
		// Tagged Mojo KIRO_ACP_STOP_REASON selects the first PRESENT member.
		// A non-string value returns null; it does not authorize falling
		// back to a lower-priority alias and fabricating an incomplete turn.
		if value, present := result[key]; present {
			text, _ := value.(string)
			return text
		}
	}
	return ""
}

func kiroIncompleteDetails(envelope acpEnvelope) (string, string) {
	switch kiroStopReason(envelope) {
	case "max_tokens":
		return "max_output_tokens", "Kiro stopped before end_turn because the model hit its output limit."
	case "max_turn_requests":
		return "max_turn_requests", "Kiro stopped before end_turn because the turn hit its request limit."
	case "refusal":
		return "refusal", "Kiro refused to continue the turn."
	case "cancelled":
		return "cancelled", "Kiro cancelled the turn before completion."
	default:
		return "", ""
	}
}

func runtimeUint(value any) uint64 {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 {
			return uint64(typed)
		}
	case json.Number:
		// Tagged serde Value::as_u64 supports the entire unsigned range.
		// json.Number.Int64 rejects valid [2^63, 2^64-1] usage and
		// would silently turn a real Kiro usage update into zero.
		if parsed, err := strconv.ParseUint(string(typed), 10, 64); err == nil {
			return parsed
		}
	case int:
		if typed >= 0 {
			return uint64(typed)
		}
	}
	return 0
}

func nilOrText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
