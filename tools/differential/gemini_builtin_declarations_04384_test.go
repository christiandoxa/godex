package main

import "testing"

func TestGeminiBuiltinDeclarationOracleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiBuiltinDeclarationNegativeControl04384() {
		t.Fatal("Gemini builtin declaration oracle accepted matching corruption")
	}
}
