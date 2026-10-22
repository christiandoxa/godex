package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiRequestNumberOracleRejectsLossyMatchingRequests(t *testing.T) {
	for _, scenario := range []string{"request-tool-numbers-buffered", "request-tool-numbers-stream"} {
		if !json.Valid(geminiRequestNumberInput04384(scenario)) || !json.Valid([]byte(geminiRequestNumberWire04384(scenario))) {
			t.Fatal("numeric fixture lost valid original or expected JSON")
		}
	}
	if !geminiRequestNumberNegativeControl04384() {
		t.Fatal("matching lossy raw requests escaped exact numeric invariants")
	}
}
