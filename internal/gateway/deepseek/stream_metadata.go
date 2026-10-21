package deepseek

import (
	"encoding/json"
	"strconv"
	"strings"
)

func (state *deepSeekChatStreamState) applyChunkMetadata(root map[string]any) {
	if id, ok := root["id"].(string); ok && strings.HasPrefix(state.responseID, "resp_deepseek_") {
		state.responseID = id
	}
	if model, ok := root["model"].(string); ok {
		state.model, state.modelPresent = model, true
	}
	if created, ok := deepSeekStreamUint(root["created"]); ok {
		state.createdAt = created
	}
	if fingerprint, _ := root["system_fingerprint"].(string); fingerprint != "" {
		state.systemFingerprint = fingerprint
	}
	if usage, present := root["usage"]; present {
		state.usage, state.usagePresent = usage, true
	}
}

func deepSeekStreamUint(value any) (uint64, bool) {
	switch current := value.(type) {
	case float64:
		if current >= 0 && current == float64(uint64(current)) {
			return uint64(current), true
		}
	case json.Number:
		parsed, err := strconv.ParseUint(current.String(), 10, 64)
		return parsed, err == nil
	case uint64:
		return current, true
	case int:
		if current >= 0 {
			return uint64(current), true
		}
	}
	return 0, false
}
