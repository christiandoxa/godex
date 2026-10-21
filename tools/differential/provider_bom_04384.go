package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

func providerBOMUpstream04384(repeated bool) string {
	bom := "\xef\xbb\xbf"
	if repeated {
		bom += bom
	}
	return "data: " + bom + `{"id":"chat-bom","created":1,"choices":[{"delta":{"content":"must-not-emit"}}]}` + "\n\ndata: [DONE]\n\n"
}

func providerBOMWire04384(epoch int64, id string) string {
	return fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":%d,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":\"incomplete JSON segment\"},\"id\":%q},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", epoch, id)
}

func validProviderBOMWire04384(wire string) bool {
	if _, err := canonicalFixtureStreamFailure04384(wire, "provider_stream_error", "incomplete JSON segment"); err != nil {
		return false
	}
	var event struct {
		CreatedAt int64 `json:"created_at"`
		Response  struct{ ID string }
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(wire, "event: response.failed\r\ndata: "), "\r\n\r\n")
	return json.Unmarshal([]byte(payload), &event) == nil && wire == providerBOMWire04384(event.CreatedAt, event.Response.ID)
}

func equivalentProviderBOMSSE04384(left, right string) bool {
	return validProviderBOMWire04384(left) && validProviderBOMWire04384(right)
}

func geminiProviderBOMScenario04384(scenario string) bool {
	return scenario == "stream-chat-bom" || scenario == "stream-chat-bom-repeat"
}
