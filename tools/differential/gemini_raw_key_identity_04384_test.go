package main

import "testing"

func TestProdex04384GeminiRawKeyIdentityOracleRejectsMatchingMutations(t *testing.T) {
	if !geminiRawKeyIdentityNegativeControl04384() {
		t.Fatal("raw-key identity oracle rejected a control or accepted matching corruption")
	}
}
