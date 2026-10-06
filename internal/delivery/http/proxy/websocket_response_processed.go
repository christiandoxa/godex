package proxy

import (
	"encoding/json"
	"strings"
)

func responseProcessedMessage(payload []byte) (string, bool) {
	var message struct {
		Type       string `json:"type"`
		ResponseID string `json:"response_id"`
	}
	if json.Unmarshal(payload, &message) != nil || message.Type != "response.processed" {
		return "", false
	}
	responseID := strings.TrimSpace(message.ResponseID)
	if responseID == "" {
		responseID = "-"
	}
	return responseID, true
}
