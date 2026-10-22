package main

import "testing"

func TestGeminiRequestMetadataOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiRequestMetadataNegativeControl04384() {
		t.Fatal("Gemini request metadata oracle accepted matching corruption")
	}
}
