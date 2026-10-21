package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiToolIdentityOracle(t *testing.T) {
	for _, scenario := range []string{"buffered-tool-dotted", "buffered-tool-padded", "buffered-tool-empty", "buffered-tool-mcp-empty-prefix", "buffered-tool-trailing-separator"} {
		fixture, ok := geminiToolIdentityCase04384(scenario)
		wire, valid := geminiToolIdentityResponse04384(scenario)
		if !ok || !valid || !json.Valid([]byte(wire)) {
			t.Fatalf("invalid tool identity fixture %s", scenario)
		}
		item := map[string]any{"type": "function_call", "name": fixture.short, "call_id": fixture.callID, "arguments": "{}"}
		if fixture.namespace != "" {
			item["namespace"] = fixture.namespace
		}
		body := map[string]any{"output": []any{map[string]any{"type": "message"}, item}}
		if err := geminiToolIdentityInvariant04384(scenario, body); err != nil {
			t.Fatalf("tool identity positive control rejected: %v", err)
		}
		for _, field := range []string{"name", "call_id", "namespace", "arguments"} {
			original, present := item[field]
			item[field] = "mutated"
			if geminiToolIdentityInvariant04384(scenario, body) == nil {
				t.Errorf("tool identity oracle accepted mutated %s in %s", field, scenario)
			}
			if present {
				item[field] = original
			} else {
				delete(item, field)
			}
		}
	}
	if _, ok := geminiToolIdentityResponse04384("chat-request"); ok {
		t.Fatal("unrelated scenario matched tool identity fixture")
	}
}
