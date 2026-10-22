package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestGeminiStreamToolLifecycleRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiLifecycleNegativeControl04384() {
		t.Fatal("Gemini tool lifecycle accepted matching corruption or rejected its independent fixture")
	}
}

func TestGeminiStreamToolLifecycleFixturesAreCompleteSSE04384(t *testing.T) {
	for _, scenario := range geminiStreamToolLifecycleCases04384() {
		t.Run(scenario, func(t *testing.T) {
			var wire bytes.Buffer
			writeGeminiToolLifecycle04384(&wire, scenario)
			decoder := sse.NewStrictDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			if decoder.Err() != nil || len(frames) < 4 || !sse.JSONDone(frames[len(frames)-1]) {
				t.Fatal("tool lifecycle fixture lost complete SSE framing")
			}
			for _, frame := range frames[:len(frames)-1] {
				if !json.Valid(frame) {
					t.Fatal("tool lifecycle fixture contains malformed packet JSON")
				}
			}
		})
	}
}
