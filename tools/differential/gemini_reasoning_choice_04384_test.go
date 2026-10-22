package main

import "testing"

func TestGeminiReasoningOracleRejectsMatchingOmissionCorruption04384(t *testing.T) {
	if !geminiReasoningChoiceNegativeControl04384() {
		t.Fatal("Gemini reasoning oracle accepted matching omission metadata corruption")
	}
}
