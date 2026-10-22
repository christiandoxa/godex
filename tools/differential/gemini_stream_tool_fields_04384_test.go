package main

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestGeminiToolFieldsRejectMatchingCorruption04384(t *testing.T) {
	if !geminiToolFieldsNegativeControl04384() {
		t.Fatal("Gemini tool fields accepted matching corruption or rejected an independent fixture")
	}
}

func TestGeminiToolFieldsFixturePacketsAreComplete04384(t *testing.T) {
	for _, scenario := range geminiToolFieldsCases04384() {
		t.Run(scenario, func(t *testing.T) {
			var wire bytes.Buffer
			writeGeminiToolFields04384(&wire, scenario)
			decoder := sse.NewStrictDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			if decoder.Err() != nil || len(frames) < 4 || !sse.JSONDone(frames[len(frames)-1]) {
				t.Fatal("tool field fixture lost full SSE framing")
			}
			for _, frame := range frames[:len(frames)-1] {
				if !json.Valid(frame) {
					t.Fatal("tool field fixture contains invalid JSON")
				}
			}
		})
	}
}
