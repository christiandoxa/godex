package deepseek

import (
	"net/http"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04355DeepSeekConversationScopePrecedence(t *testing.T) {
	for _, test := range []struct {
		name   string
		header http.Header
		body   string
		want   string
	}{
		{
			name: "explicit session header wins",
			header: http.Header{
				"Session_Id":                        []string{" header-session "},
				"X-Codex-Turn-Metadata":             []string{`{"session_id":"turn-session"}`},
				deepSeekConversationNamespaceHeader: []string{"namespace-session"},
			},
			body: `{"session_id":"body-session"}`, want: "header-session",
		},
		{
			name: "turn metadata wins over body",
			header: http.Header{
				"X-Codex-Turn-Metadata":             []string{`{"client_metadata":{"session_id":"turn-session"}}`},
				deepSeekConversationNamespaceHeader: []string{"namespace-session"},
			},
			body: `{"session_id":"body-session"}`, want: "turn-session",
		},
		{
			name:   "nested body session",
			header: http.Header{deepSeekConversationNamespaceHeader: []string{"namespace-session"}},
			body:   `{"client_metadata":{"session_id":"body-session"}}`, want: "body-session",
		},
		{
			name:   "internal namespace fallback",
			header: http.Header{deepSeekConversationNamespaceHeader: []string{" namespace-session "}},
			body:   `{}`, want: "namespace-session",
		},
		{name: "gateway fallback", body: `{}`, want: "gateway"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &RuntimeTransport{conversations: newDeepSeekConversationStore()}
			got := transport.conversationsForRequest(proxymodel.Request{Header: test.header, Body: []byte(test.body)})
			want := transport.conversations.scoped(test.want)
			if got.scopePrefix != want.scopePrefix {
				t.Fatalf("scope = %q, want %q", got.scopePrefix, want.scopePrefix)
			}
		})
	}
}

func TestProdex04355DeepSeekConversationEstimateMatchesTaggedSerdeNodeAccounting(t *testing.T) {
	message := map[string]any{
		"role":    "user",
		"content": "x",
		"ok":      true,
		"n":       float64(2),
	}
	// Tagged Rust build: Value=32 bytes, Number=16, Bool=1.
	// Object = 32 + (4 + 32+4) + (7 + 32+1) + (2 + 32+1) + (1 + 32+16) = 196.
	// Storage key contributes its raw byte length.
	const want = 200 // len("resp") + 196
	if got := deepSeekConversationEstimate("resp", []any{message}); got != want {
		t.Fatalf("conversation estimate = %d, want %d", got, want)
	}
}
