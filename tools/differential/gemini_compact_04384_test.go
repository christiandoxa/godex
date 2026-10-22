package main

import "testing"

func TestGeminiCompactOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiCompactNegativeControl04384() {
		t.Fatal("Gemini Compact oracle accepted matching corruption")
	}
}
