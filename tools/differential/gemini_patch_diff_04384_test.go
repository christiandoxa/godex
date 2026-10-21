package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiPatchOracleRejectsMutations(t *testing.T) {
	for _, scenario := range []string{"buffered-patch-unified-update", "buffered-patch-unified-files", "buffered-patch-literal-crlf"} {
		input, want, ok := geminiPatchFixture04384(scenario)
		wire, valid := geminiPatchResponse04384(scenario)
		if !ok || !valid || !json.Valid([]byte(wire)) {
			t.Fatalf("invalid patch fixture %s", scenario)
		}
		item := map[string]any{"type": "custom_tool_call", "name": "apply_patch", "call_id": "call-patch", "input": want}
		body := map[string]any{"output": []any{map[string]any{"type": "message"}, item}}
		if err := geminiPatchInvariant04384(scenario, body); err != nil {
			t.Fatalf("canonical patch control rejected: %v", err)
		}
		for _, changed := range []string{want + "\n", "", input + "\n"} {
			item["input"] = changed
			if geminiPatchInvariant04384(scenario, body) == nil {
				t.Fatalf("patch oracle missed altered input in %s", scenario)
			}
		}
	}
	if _, ok := geminiPatchResponse04384("chat-request"); ok {
		t.Fatal("unrelated scenario matched patch fixture")
	}
}
