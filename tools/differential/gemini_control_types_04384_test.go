package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiControlTypeOracleRejectsMatchingCorruptions(t *testing.T) {
	for _, suffix := range geminiControlTypeSuffixes04384() {
		if !json.Valid(geminiControlTypeInput04384("request-control-" + suffix)) {
			t.Fatal("invalid control-type fixture JSON")
		}
	}
	if !geminiControlTypeNegativeControl04384() {
		t.Fatal("matching control-type failure corruptions escaped strict evidence")
	}
}
