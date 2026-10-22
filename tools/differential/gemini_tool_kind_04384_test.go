package main

import "testing"

func TestGeminiToolKindOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiToolKindNegativeControl04384() {
		t.Fatal("Gemini tool kind oracle accepted matching corruption")
	}
}
