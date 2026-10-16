package main

import "testing"

func TestProdex04380GeminiBufferedNullToolTextOracleRejectsLeakedContent(t *testing.T) {
	original := `{"id":"resp_deepseek_generated","model":"gemini-3.1-pro-preview","object":"response","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hidden beforehidden after"}]},{"type":"function_call","call_id":"call_1","name":"tool_call","arguments":"{}"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	canonical, err := geminiBufferedNullToolProjection04380(original)
	if err != nil || canonical == nil {
		t.Fatalf("public canonical null tool rejected: %v", err)
	}
	for _, payload := range []string{
		`{"id":"resp_deepseek_generated","output":[{"type":"message","content":[{"type":"output_text","text":"hidden beforehidden after"}]},{"type":"function_call","call_id":"call_1","name":"tool_call","arguments":"{}"}]}`,
		`{"id":"resp_deepseek_generated","output":[{"type":"function_call","call_id":"call_1","name":"tool_call","arguments":"{}"}]}`,
		`{"id":"resp_deepseek_generated","output":[]}`,
		`{"id":"resp_deepseek_generated","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hidden before"}]},{"type":"function_call","call_id":"call_1","name":"tool_call","arguments":"{}"}]}`,
		`{"id":"resp_deepseek_generated","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hidden beforehidden after"}]},{"type":"function_call","call_id":"call_gemini_generated","name":"tool_call","arguments":"{}"}]}`,
		`{"id":"resp_deepseek_generated","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hidden beforehidden after"}]},{"type":"function_call","call_id":"call_1","name":"tool_call","arguments":"{"unsafe":true}"}]}`,
	} {
		if _, err := geminiBufferedNullToolProjection04380(payload); err == nil {
			t.Fatalf("null functionCall public content or ID mutation escaped oracle: %s", payload)
		}
	}
}
