package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiExplicitMessageRolesRequireTaggedValues(t *testing.T) {
	for _, role := range []string{"null", "false", "7", "[]", "{}", `""`, `"tool"`, `"unknown"`, `" user "`} {
		t.Run(role, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"message","role":` + role + `,"content":"same request"}]}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
				t.Fatalf("invalid explicit message role became upstream work: role=%s error=%v", role, err)
			}
		})
	}
	for _, fields := range []string{"", `,"role":"user"`, `,"role":"assistant"`, `,"role":"system"`, `,"role":"developer"`, `,"role":"\u0075ser"`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[{"type":"message","content":"same request"` + fields + `}]}`)); err != nil {
			t.Fatalf("supported explicit message role rejected: fields=%s error=%v", fields, err)
		}
	}
}
