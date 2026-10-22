package main

import "testing"

func TestGeminiReasoningStreamRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiReasoningStreamNegativeControl04384() {
		t.Fatal("reasoning stream lifecycle or state corruption escaped the oracle")
	}
}
