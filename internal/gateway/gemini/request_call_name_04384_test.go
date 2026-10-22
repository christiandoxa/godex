package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiToolCallsRequireNonblankName(t *testing.T) {
	for _, kind := range []string{"function_call", "custom_tool_call", "mcp_call"} {
		for _, fields := range []string{"", `,"name":null`, `,"name":7`, `,"name":" \t"`, `,"name":"","tool_name":"later"`} {
			t.Run(kind+fields, func(t *testing.T) {
				body := []byte(`{"input":[{"role":"user","content":"same request"},{"type":"` + kind + `","call_id":"synthetic-call","arguments":"{}","input":"synthetic"` + fields + `}]}`)
				_, _, err := normalizeGeminiChatInput043711(body)
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid call name accepted: kind=%s fields=%s error=%v", kind, fields, err)
				}
			})
		}
	}
	for _, fields := range []string{`,"name":"synthetic_lookup"`, `,"tool_name":"synthetic_lookup"`, `,"function":{"name":"synthetic_lookup"}`, `,"name":null,"tool_name":"synthetic_lookup"`, `,"__missing__":"synthetic_lookup"`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[{"type":"function_call","call_id":"synthetic-call","arguments":"{}"` + fields + `}]}`)); err != nil {
			t.Fatalf("tagged valid name selection rejected: %s error=%v", fields, err)
		}
	}
}
