package main

import "testing"

func TestGeminiStreamUsageRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiStreamUsageNegativeControl04384() {
		t.Fatal("stream usage, request, attempt or durable state corruption escaped the oracle")
	}
}
