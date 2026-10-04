package routing

import (
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestParseWebSocketRequestMetadataMatchesProdex04353(t *testing.T) {
	for _, test := range []struct {
		name    string
		body    string
		headers http.Header
		want    websocketRequestMetadata
	}{
		{
			name: "direct values",
			body: `{"previous_response_id":" resp-1 ","session_id":" body-session ","x-codex-turn-state":" body-turn "}`,
			want: websocketRequestMetadata{
				previousResponseID: "resp-1",
				sessionID:          "body-session",
				turnState:          "body-turn",
			},
		},
		{
			name: "client metadata fallbacks",
			body: `{"client_metadata":{"session_id":" meta-session ","x-codex-turn-state":" meta-turn "}}`,
			want: websocketRequestMetadata{sessionID: "meta-session", turnState: "meta-turn"},
		},
		{
			name: "direct body beats client metadata",
			body: `{"session_id":"direct-session","x-codex-turn-state":"direct-turn","client_metadata":{"session_id":"meta-session","x-codex-turn-state":"meta-turn"}}`,
			want: websocketRequestMetadata{sessionID: "direct-session", turnState: "direct-turn"},
		},
		{
			name:    "explicit session header beats body",
			body:    `{"session_id":"body-session"}`,
			headers: http.Header{"Session-Id": {"header-session"}},
			want:    websocketRequestMetadata{sessionID: "header-session"},
		},
		{
			name: "session underscore header has tagged precedence",
			headers: http.Header{
				"Session_id":   {"underscore"},
				"Session-Id":   {"dash"},
				"X-Session-Id": {"x-dash"},
			},
			want: websocketRequestMetadata{sessionID: "underscore"},
		},
		{
			name:    "body turn state beats handshake header",
			body:    `{"client_metadata":{"x-codex-turn-state":"body-turn"}}`,
			headers: http.Header{"X-Codex-Turn-State": {"header-turn"}},
			want:    websocketRequestMetadata{turnState: "body-turn"},
		},
		{
			name:    "header turn state fallback",
			headers: http.Header{"X-Codex-Turn-State": {" header-turn "}},
			want:    websocketRequestMetadata{turnState: "header-turn"},
		},
		{
			name: "tool output requires locked previous response affinity",
			body: `{"previous_response_id":"resp-2","input":[{"type":"function_call_output","call_id":"call-1","output":"ok"}]}`,
			want: websocketRequestMetadata{
				previousResponseID:               "resp-2",
				requiresPreviousResponseAffinity: true,
			},
		},
		{
			name: "non tool input does not require locked affinity",
			body: `{"previous_response_id":"resp-3","input":[{"type":"message","role":"user","content":"hello"}]}`,
			want: websocketRequestMetadata{previousResponseID: "resp-3"},
		},
		{
			name: "nested response previous response is not request metadata",
			body: `{"response":{"previous_response_id":"nested-wrong"}}`,
			want: websocketRequestMetadata{},
		},
		{
			name: "wrong JSON types are ignored",
			body: `{"previous_response_id":7,"session_id":{"id":"x"},"x-codex-turn-state":false,"client_metadata":{"session_id":9}}`,
			want: websocketRequestMetadata{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := proxymodel.Request{
				Body:             []byte(test.body),
				Header:           test.headers,
				WebSocketMessage: true,
			}
			if request.Header == nil {
				request.Header = make(http.Header)
			}
			if got := parseWebSocketRequestMetadata(request); got != test.want {
				t.Fatalf("metadata = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestWebSocketRequestMetadataRejectsNestedPreviousResponseRegression(t *testing.T) {
	request := proxymodel.Request{
		Header:           make(http.Header),
		Body:             []byte(`{"response":{"previous_response_id":"resp-nested"},"previous_response_id":"resp-top"}`),
		WebSocketMessage: true,
	}
	got := parseWebSocketRequestMetadata(request)
	if got.previousResponseID != "resp-top" {
		t.Fatalf("previous response = %q, want top-level resp-top", got.previousResponseID)
	}
}

func TestWebSocketRequestMetadataToolOutputExpectationIsIndependent(t *testing.T) {
	request := proxymodel.Request{
		Header:           make(http.Header),
		Body:             []byte(`{"previous_response_id":"resp","input":[{"type":"custom_tool_call_output","call_id":"call"}]}`),
		WebSocketMessage: true,
	}
	if !parseWebSocketRequestMetadata(request).requiresPreviousResponseAffinity {
		t.Fatal("tagged _call_output input did not require previous-response affinity")
	}
}
