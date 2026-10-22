package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiToolItemsRequireNonblankCallIdentity(t *testing.T) {
	for _, kind := range []string{"function_call", "custom_tool_call", "mcp_call", "local_shell_call", "function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output"} {
		for _, fields := range []string{"", `,"call_id":null`, `,"call_id":7`, `,"call_id":" \t"`, `,"call_id":"","id":"later"`} {
			t.Run(kind+fields, func(t *testing.T) {
				body := []byte(`{"input":[{"role":"user","content":"same request"},{"type":"` + kind + `","name":"synthetic_lookup","arguments":"{}","input":"synthetic","action":{"command":["synthetic"]},"output":"synthetic"` + fields + `}]}`)
				_, _, err := normalizeGeminiChatInput043711(body)
				var failure *proxymodel.Error
				if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
					t.Fatalf("invalid call identity accepted: kind=%s fields=%s error=%v", kind, fields, err)
				}
			})
		}
	}
	for _, fields := range []string{`,"call_id":"synthetic-call"`, `,"tool_call_id":"synthetic-call"`, `,"id":"synthetic-call"`, `,"call_id":null,"tool_call_id":"synthetic-call"`} {
		if _, _, err := normalizeGeminiChatInput043711([]byte(`{"input":[{"type":"function_call","name":"synthetic_lookup","arguments":"{}"` + fields + `}]}`)); err != nil {
			t.Fatalf("valid identity alias rejected: %s error=%v", fields, err)
		}
	}
}
