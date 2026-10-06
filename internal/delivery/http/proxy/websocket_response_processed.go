package proxy

import "encoding/json"

func isResponseProcessedMessage(payload []byte) bool {
	var message struct {
		Type string `json:"type"`
	}
	return json.Unmarshal(payload, &message) == nil && message.Type == "response.processed"
}
