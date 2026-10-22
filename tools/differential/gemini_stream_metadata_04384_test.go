package main

import "testing"

func TestGeminiStreamMetadataRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiStreamMetadataNegativeControl04384() {
		t.Fatal("stream metadata or state corruption escaped the oracle")
	}
}
