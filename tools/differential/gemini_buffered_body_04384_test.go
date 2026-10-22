package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiBufferedBodyFixtureAndIndependentMutations(t *testing.T) {
	for _, scenario := range []string{"buffered-body-under", "buffered-body-limit", "buffered-body-over", "buffered-body-over-rotation"} {
		body, known := geminiBufferedBodyResponse04384(scenario)
		size, _ := geminiBufferedBodyCase04384(scenario)
		var root map[string]any
		if !known || len(body) != size || json.Unmarshal([]byte(body), &root) != nil || len(root["candidates"].([]any)) != 1 {
			t.Fatalf("%s fixture lost exact body size or native content", scenario)
		}
	}
	if !geminiBufferedBodyNegativeControl04384() {
		t.Fatal("matching Gemini body boundary corruptions escaped the oracle")
	}
}
