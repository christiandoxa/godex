package gemini

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384GeminiChatIgnoresParallelToolControl(t *testing.T) {
	for _, value := range []string{"true", "false", "null", "7", `"opaque"`, `[]`, `{}`} {
		t.Run(value, func(t *testing.T) {
			input := []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","parallel_tool_calls":` + value + `}`)
			body, _, err := normalizeGeminiChatInput043711(input)
			if err == nil {
				body, err = chatcompat.ResponsesRequest(body, "gemini-3.1-pro-preview", "")
			}
			if err != nil {
				t.Fatalf("ignored Gemini control was rejected: %v", err)
			}
			var request map[string]any
			if err := json.Unmarshal(body, &request); err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"model": "gemini-3.1-pro-preview", "stream": false,
				"messages": []any{map[string]any{"role": "user", "content": "same request"}}}
			if !reflect.DeepEqual(request, want) {
				t.Fatal("ignored parallel-tool control reached the provider or changed fresh input")
			}
		})
	}
	for _, invalid := range []string{`"\ud800"`, `1e309`, `{"unfinished":`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":"same request","parallel_tool_calls":` + invalid + `}`)); err == nil {
			t.Fatal("ignored parallel control bypassed complete serde validation")
		}
	}
}
