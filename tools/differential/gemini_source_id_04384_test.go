package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiSourceIDOracle(t *testing.T) {
	for _, scenario := range []string{"buffered-source-id-string", "buffered-source-id-empty", "buffered-source-id-wrapped", "buffered-source-id-null"} {
		wire, ok := geminiSourceIDResponse04384(scenario)
		var root map[string]any
		if !ok || json.Unmarshal([]byte(wire), &root) != nil {
			t.Fatalf("invalid identity fixture: %s", scenario)
		}
		_, want, _ := geminiSourceIDFixture04384(scenario)
		if want == "" {
			want = "resp_deepseek_01900000-0000-7000-8000-000000000001"
		}
		body := map[string]any{"id": want}
		if err := geminiSourceIDInvariant04384(scenario, body); err != nil {
			t.Fatalf("canonical source identity rejected: %s, %v", scenario, err)
		}
		body["id"] = "source-id"
		if geminiSourceIDInvariant04384(scenario, body) == nil {
			t.Fatalf("oracle accepted original source id as an alias: %s", scenario)
		}
		body["id"] = want
		body["created_at"] = 1
		if geminiSourceIDInvariant04384(scenario, body) == nil {
			t.Fatalf("oracle accepted a native timestamp: %s", scenario)
		}
	}
	if _, ok := geminiSourceIDResponse04384("chat-request"); ok {
		t.Fatal("unrelated scenario matched source identity fixture")
	}
}
