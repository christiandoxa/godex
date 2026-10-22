package deepseek

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384BufferedSearchRuntimeDepthBoundary(t *testing.T) {
	for _, depth := range []int{124, 125, 126} {
		arguments := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		body, err := json.Marshal(map[string]any{"id": "resp-depth", "created": 1,
			"choices": []any{map[string]any{"message": map[string]any{"tool_calls": []any{
				map[string]any{"id": "call-depth", "function": map[string]any{"name": "tool_search", "arguments": arguments}},
			}}}}})
		if err != nil {
			t.Fatal(err)
		}
		store := chatcompat.NewConversationStore()
		response, err := translateResponseWithConversation(&http.Response{StatusCode: 200,
			Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))},
			nil, store, []any{map[string]any{"role": "user", "content": "synthetic"}}, 1)
		if err != nil {
			t.Fatal(err)
		}
		output, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if depth == 124 {
			if response.StatusCode != 200 || !bytes.Contains(output, []byte(`"type":"tool_search_call"`)) {
				t.Fatal("accepted public depth control lost structured output")
			}
		} else if response.StatusCode != 500 || len(output) != 0 || len(response.Header.Values("Content-Type")) != 0 {
			t.Fatalf("runtime depth %d response = status %d, body bytes %d", depth, response.StatusCode, len(output))
		}
		if store.Contains("resp-depth") != (depth == 124) {
			t.Fatalf("runtime depth %d stored the wrong conversation ownership", depth)
		}
	}
}
