package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiUserIDOracleRejectsMatchingMutations(t *testing.T) {
	for _, suffix := range []string{"absent", "canonical", "user-alias", "safety-alias", "blank", "trim-limit", "null", "number", "number-rotation", "over-limit", "invalid-character"} {
		if !json.Valid(geminiUserIDInput04384("request-user-id-" + suffix)) {
			t.Fatal("invalid user ID fixture JSON")
		}
	}
	if !geminiUserIDNegativeControl04384() {
		t.Fatal("matching corrupted Gemini user ID traces escaped exact invariants")
	}
}
