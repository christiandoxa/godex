package main

import "testing"

func TestGeminiStreamFinishPresenceRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiStreamFinishPresenceNegativeControl04384() {
		t.Fatal("stream finish reason presence or state corruption escaped the oracle")
	}
}
