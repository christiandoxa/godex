package kiro

import (
	"encoding/json"
	"fmt"
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
	sessionTitle      string
	sessionUpdatedAt  string
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
		var params struct {
			Update map[string]any `json:"update"`
		}
		if json.Unmarshal(envelope.Params, &params) != nil || params.Update == nil {
			continue
		}
		state.applyUpdate(params.Update)
	}
	return state
}

func (state *kiroTurnState) applyUpdate(update map[string]any) {
	kind := runtimeString(update["sessionUpdate"], "")
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
		state.currentModeID = runtimeString(update["currentModeId"], "")
	case "session_info_update":
		if title := runtimeString(update[kiroFieldTitle], ""); title != "" {
			state.sessionTitle = title
		}
		if updated := runtimeString(update["updatedAt"], ""); updated != "" {
			state.sessionUpdatedAt = updated
		}
	case "tool_call", "tool_call_update":
		state.applyToolActivity(kind == "tool_call", update)
	}
}

func (state *kiroTurnState) applyToolActivity(initial bool, update map[string]any) {
	if state.truncatedTools {
		return
	}
	id := runtimeString(update["toolCallId"], "")
	previous := state.toolLabels[id]
	title := runtimeString(update[kiroFieldTitle], previous.name)
	kind := runtimeString(update["kind"], previous.kind)
	status := runtimeString(update[kiroFieldStatus], "")
	detailsOmitted := update["rawInput"] != nil || update["rawOutput"] != nil ||
		update[kiroFieldContent] != nil || update["locations"] != nil
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
	if id != "" && len(id) <= 256 && len(state.toolLabels) < maxToolActivityEvents {
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
	if len(state.plan) > 0 {
		kiro["plan"] = state.plan
	}
	if len(state.availableCommands) > 0 {
		kiro["available_commands"] = state.availableCommands
	}
	if state.currentModeID != "" {
		kiro["current_mode_id"] = state.currentModeID
	}
	if state.sessionTitle != "" || state.sessionUpdatedAt != "" {
		kiro["session_info"] = map[string]any{kiroFieldTitle: nilOrText(state.sessionTitle), "updated_at": nilOrText(state.sessionUpdatedAt)}
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
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		if text, ok := typed["text"].(string); ok {
			return text
		}
	case []any:
		var builder strings.Builder
		for _, item := range typed {
			builder.WriteString(acpContentText(item))
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
	result := map[string]any{"used": used, "size": size, "remaining": remaining}
	if cost, ok := update["cost"].(map[string]any); ok {
		result["cost"] = sanitizeMap(cost, "amount", "currency")
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
	return append([]any(nil), items...)
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
		if value, ok := result[key].(string); ok {
			return value
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
		if parsed, err := typed.Int64(); err == nil && parsed >= 0 {
			return uint64(parsed)
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
