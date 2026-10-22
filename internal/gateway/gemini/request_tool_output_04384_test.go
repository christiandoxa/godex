package gemini

import (
	"errors"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04384GeminiToolOutputsRequirePresentResultField(t *testing.T) {
	for _, kind := range []string{"function_call_output", "custom_tool_call_output", "mcp_tool_result", "mcp_call_output"} {
		t.Run(kind, func(t *testing.T) {
			body := []byte(`{"input":[{"role":"user","content":"same request"},{"type":"` + kind + `","call_id":"synthetic-call"}]}`)
			_, _, err := normalizeGeminiChatInput043711(body)
			var failure *proxymodel.Error
			if !errors.As(err, &failure) || failure.StatusCode != 502 || failure.Message != "upstream request failed" {
				t.Fatalf("missing output became synthetic null result: kind=%s error=%v", kind, err)
			}
		})
		for _, field := range []string{"output", "content", "result", "error"} {
			for _, value := range []string{"null", "false", `""`} {
				body := []byte(`{"input":[{"type":"` + kind + `","call_id":"synthetic-call","` + field + `":` + value + `}]}`)
				if _, _, err := normalizeGeminiChatInput043711(body); err != nil {
					t.Fatalf("present output incorrectly rejected: kind=%s field=%s value=%s error=%v", kind, field, value, err)
				}
			}
		}
	}
}
