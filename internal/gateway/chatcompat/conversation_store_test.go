package chatcompat

import (
	"fmt"
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
				"Session_Id":                []string{" header-session "},
				"X-Codex-Turn-Metadata":     []string{`{"session_id":"turn-session"}`},
				conversationNamespaceHeader: []string{"namespace-session"},
			},
			body: `{"session_id":"body-session"}`, want: "header-session",
		},
		{
			name: "turn metadata wins over body",
			header: http.Header{
				"X-Codex-Turn-Metadata":     []string{`{"client_metadata":{"session_id":"turn-session"}}`},
				conversationNamespaceHeader: []string{"namespace-session"},
			},
			body: `{"session_id":"body-session"}`, want: "turn-session",
		},
		{
			name:   "nested body session",
			header: http.Header{conversationNamespaceHeader: []string{"namespace-session"}},
			body:   `{"client_metadata":{"session_id":"body-session"}}`, want: "body-session",
		},
		{
			name:   "internal namespace fallback",
			header: http.Header{conversationNamespaceHeader: []string{" namespace-session "}},
			body:   `{}`, want: "namespace-session",
		},
		{name: "gateway fallback", body: `{}`, want: "gateway"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := NewConversationStore()
			got := store.ForRequest(proxymodel.Request{Header: test.header, Body: []byte(test.body)})
			want := store.Scoped(test.want)
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
	if got := conversationEstimate("resp", []any{message}); got != want {
		t.Fatalf("conversation estimate = %d, want %d", got, want)
	}
}

func TestProdex04355DeepSeekConversationScopesDoNotLeak(t *testing.T) {
	store := NewConversationStore()
	left := store.Scoped("tenant-a")
	right := store.Scoped("tenant-b")
	left.Insert("resp-1", []any{map[string]any{"role": "user", "content": "secret-a"}})
	if got := right.History("resp-1"); len(got) != 0 {
		t.Fatalf("cross-scope history leaked = %#v", got)
	}
}

func TestProdex04355DeepSeekConversationStoreEvictsOldestPerScope(t *testing.T) {
	store := NewConversationStore().Scoped("tenant-a")
	for index := 0; index <= conversationsPerScope; index++ {
		store.Insert(fmt.Sprintf("resp-%d", index), []any{map[string]any{"role": "user", "content": index}})
	}
	if got := store.History("resp-0"); len(got) != 0 {
		t.Fatalf("oldest conversation survived scope cap = %#v", got)
	}
	if got := store.History(fmt.Sprintf("resp-%d", conversationsPerScope)); len(got) == 0 {
		t.Fatal("newest conversation was evicted")
	}
}
