package openai

import "testing"

func TestWebSocketEventMetadataMatchesProdexResponseMetadataShapes(t *testing.T) {
	tests := []struct {
		name, payload, wantKind, wantID, wantTurn string
	}{
		{
			name:     "nested header array wins",
			payload:  `{"type":"response.completed","headers":{"x-codex-turn-state":"root-state"},"response":{"id":"resp-nested","headers":[{"name":"X-Codex-Turn-State","values":[" nested-state ","later"]}],"turn_state":"response-state","turnState":"camel-response"}}`,
			wantKind: "response.completed", wantID: "resp-nested", wantTurn: "nested-state",
		},
		{
			name:     "root header array and response object id",
			payload:  `{"object":"response","id":"resp-object","headers":[{"key":"X-CODEX-TURN-STATE","value":[" root-state "]}]}`,
			wantKind: "", wantID: "resp-object", wantTurn: "root-state",
		},
		{
			name:     "camel case turn state",
			payload:  `{"type":"response.completed","response":{"id":"resp-camel","turnState":" response-state "}}`,
			wantKind: "response.completed", wantID: "resp-camel", wantTurn: "response-state",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kind, responseID, turnState := websocketEventMetadata([]byte(test.payload))
			if kind != test.wantKind {
				t.Fatalf("event kind = %q, want %q", kind, test.wantKind)
			}
			if responseID != test.wantID || turnState != test.wantTurn {
				t.Fatalf("metadata = id:%q turn:%q, want id:%q turn:%q", responseID, turnState, test.wantID, test.wantTurn)
			}
		})
	}
}
