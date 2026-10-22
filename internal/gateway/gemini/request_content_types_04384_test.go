package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiExplicitMessageContentRequiresSupportedShape(t *testing.T) {
	for _, content := range []string{"null", "false", "true", "7", "-3", "1.0"} {
		t.Run(content, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"message","role":"user","content":` + content + `},{"role":"user","content":"same request"}]}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
				t.Fatalf("invalid message content disappeared before upstream: content=%s error=%v", content, err)
			}
		})
	}
	for _, content := range []string{`"same request"`, `[{"type":"input_text","text":"same request"}]`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[{"type":"message","role":"user","content":` + content + `}]}`)); err != nil {
			t.Fatalf("supported content shape rejected: content=%s error=%v", content, err)
		}
	}
}
