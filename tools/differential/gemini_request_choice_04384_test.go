package main

import "testing"

func TestGeminiChoiceOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiChoiceNegativeControl04384() {
		t.Fatal("Gemini tool-choice oracle accepted matching corruption")
	}
}
