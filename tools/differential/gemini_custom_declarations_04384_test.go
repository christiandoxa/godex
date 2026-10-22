package main

import "testing"

func TestGeminiCustomDeclarationOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiCustomDeclarationNegativeControl04384() {
		t.Fatal("Gemini custom declaration oracle accepted matching corruption")
	}
}
