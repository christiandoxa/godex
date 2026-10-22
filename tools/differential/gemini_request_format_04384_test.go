package main

import "testing"

func TestGeminiFormatOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiFormatNegativeControl04384() {
		t.Fatal("Gemini format oracle accepted matching corruption")
	}
}
