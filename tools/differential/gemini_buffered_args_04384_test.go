package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiBufferedArgsOracle(t *testing.T) {
	if !geminiBufferedArgsNegativeControl04384() {
		t.Fatal("buffered tool oracle rejected positive control or accepted a mutation")
	}
	for _, scenario := range []string{"buffered-args-null", "buffered-args-wrapped-null", "buffered-args-absent", "buffered-args-empty"} {
		t.Run(scenario, func(t *testing.T) {
			wire, ok := geminiBufferedArgsResponse04384(scenario)
			var root map[string]any
			if !ok || json.Unmarshal([]byte(wire), &root) != nil {
				t.Fatal("invalid native fixture")
			}
			if scenario == "buffered-args-wrapped-null" {
				root = root["response"].(map[string]any)
			}
			candidate := root["candidates"].([]any)[0].(map[string]any)
			parts := candidate["content"].(map[string]any)["parts"].([]any)
			call := parts[1].(map[string]any)["functionCall"].(map[string]any)
			args, present := call["args"]
			if present != (scenario != "buffered-args-absent") {
				t.Fatal("fixture changed argument presence")
			}
			_, want, _ := geminiBufferedArgsFixture04384(scenario)
			if present {
				encoded, err := json.Marshal(args)
				if err != nil || string(encoded) != want {
					t.Fatalf("fixture arguments=%s want=%s err=%v", encoded, want, err)
				}
			}
		})
	}
	if _, ok := geminiBufferedArgsResponse04384("chat-request"); ok {
		t.Fatal("unrelated scenario matched args fixture")
	}
}
