package gemini

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384GeminiUnboundPreviousResponseUsesFreshInput(t *testing.T) {
	for _, fixture := range []struct{ name, previous string }{{"unknown", "resp-unknown"}, {"empty", ""}} {
		t.Run(fixture.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{"model": "gemini-3.1-pro-preview",
				"input": "keep fresh input", "previous_response_id": fixture.previous})
			normalized, err := normalizeGeminiChatInput043711(body)
			if err != nil {
				t.Fatal(err)
			}
			translated, err := chatcompat.ResponsesRequest(normalized, "gemini-3.1-pro-preview", "")
			if err != nil {
				t.Fatalf("unbound continuation was not translated as fresh input: %v", err)
			}
			var actual map[string]any
			if err := json.Unmarshal(translated, &actual); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
				"messages": []any{map[string]any{"role": "user", "content": "keep fresh input"}}}
			if !reflect.DeepEqual(actual, want) {
				t.Fatalf("unbound continuation changed fresh input: %#v", actual)
			}
		})
	}
}
