package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiRejectsUnsupportedExplicitInputTypes(t *testing.T) {
	for _, kind := range []string{"null", "false", "7", "[]", "{}", `""`, `"unknown"`, `"input_text"`, `"output_text"`, `"input_image"`, `"reasoning"`} {
		t.Run(kind, func(t *testing.T) {
			body := []byte(`{"input":[{"type":` + kind + `,"role":"user","content":"same request","text":"same request"}]}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
				t.Fatalf("unsupported explicit item type accepted: type=%s error=%v", kind, err)
			}
		})
	}
	for _, item := range []string{`{"role":"user","content":"same request"}`, `{"type":"message","role":"user","content":"same request"}`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[` + item + `]}`)); err != nil {
			t.Fatalf("supported implicit/explicit message rejected: %v", err)
		}
	}
}
