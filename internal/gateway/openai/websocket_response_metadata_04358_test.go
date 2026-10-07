package openai

import "testing"

func TestWebSocketEventMetadataMatchesProdexResponseMetadataShapes(t *testing.T) {
	tests := []struct {
		name, payload, wantID, wantTurn string
	}{
		{
			name:    "nested header array wins",
			payload: `{"type":"response.completed","headers":{"x-codex-turn-state":"root-state"},"response":{"id":"resp-nested","headers":[{"name":"X-Codex-Turn-State","values":[" nested-state ","later"]}],"turn_state":"response-state","turnState":"camel-response"}}`,
			wantID:  "resp-nested", wantTurn: "nested-state",
		},
		{
			name:    "root header array and response object id",
			payload: `{"object":"response","id":"resp-object","headers":[{"key":"X-CODEX-TURN-STATE","value":[" root-state "]}]}`,
			wantID:  "resp-object", wantTurn: "root-state",
		},
		{
			name:    "camel case turn state",
			payload: `{"type":"response.completed","response":{"id":"resp-camel","turnState":" response-state "}}`,
			wantID:  "resp-camel", wantTurn: "response-state",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, responseID, turnState := websocketEventMetadata([]byte(test.payload))
			if kind != "response.completed" && test.name != "root header array and response object id" {
				t.Fatalf("event kind = %q", kind)
			}
			if responseID != test.wantID || turnState != test.wantTurn {
				t.Fatalf("metadata = id:%q turn:%q, want id:%q turn:%q", responseID, turnState, test.wantID, test.wantTurn)
			}
		})
	}
}
