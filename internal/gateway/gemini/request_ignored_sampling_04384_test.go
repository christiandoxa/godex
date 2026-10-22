package gemini

import (
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384GeminiChatIgnoresUnprojectedSamplingControls(t *testing.T) {
	for _, field := range []string{"presence_penalty", "frequency_penalty", "seed", "n"} {
		for _, value := range []string{"true", "false", "null", "7", `"opaque"`, `[]`, `{}`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				body, _, err := normalizeGeminiChatInput043711([]byte(`{"model":"gemini-3.1-pro-preview","input":"same request","` + field + `":` + value + `}`))
				if err == nil {
					body, err = chatcompat.ResponsesRequest(body, "gemini-3.1-pro-preview", "")
				}
				want := `{"messages":[{"content":"same request","role":"user"}],"model":"gemini-3.1-pro-preview","stream":false}`
				if err != nil || string(body) != want {
					t.Fatalf("ignored sampling control changed request: error=%v body=%s", err, body)
				}
			})
		}
		for _, invalid := range []string{`"\ud800"`, `1e309`, `{"unfinished":`} {
			if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":"same request","` + field + `":` + invalid + `}`)); err == nil {
				t.Fatal("ignored sampling control bypassed complete serde validation")
			}
		}
	}
}
