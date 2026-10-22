package main

import "testing"

func TestGeminiToolSnapshotRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiCompactNegativeControl04384() {
		t.Fatal("tool snapshot accepted matching owner/history/seed corruption")
	}
}
