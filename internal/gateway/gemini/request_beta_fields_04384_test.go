package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiRejectsPresentBetaCompletionFields(t *testing.T) {
	for _, field := range []string{"prefix", "suffix", "prompt"} {
		for _, value := range []string{"null", "false", `""`, `{}`} {
			t.Run(field+"/"+value, func(t *testing.T) {
				body := []byte(`{"model":"gemini-3.1-pro-preview","input":"same request","` + field + `":` + value + `}`)
				_, _, err := normalizeGeminiChatInput043711(body)
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("present beta completion field accepted: field=%s value=%s error=%v", field, value, err)
				}
			})
		}
	}
	for _, fields := range []string{`"prefix_hint":false`, `"unused":{"prefix":null,"suffix":null,"prompt":null}`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":"same request",` + fields + `}`)); err != nil {
			t.Fatalf("unrelated completion-like metadata rejected: %v", err)
		}
	}
}
