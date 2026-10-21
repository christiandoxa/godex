package sse

import (
	"bytes"
	"testing"
)

func TestProdex04384StrictProviderDecoderPreservesPayloadBOM(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	for _, payload := range []string{bom + `{}`, bom + bom + `{}`, bom + "[DONE]"} {
		wire := "data: " + payload + "\n\n"
		for _, chunkSize := range []int{1, 3, len(wire)} {
			decoder := NewStrictDecoder(1024)
			var frames [][]byte
			for offset := 0; offset < len(wire); offset += chunkSize {
				frames = append(frames, decoder.Feed([]byte(wire[offset:min(len(wire), offset+chunkSize)]))...)
			}
			frames = append(frames, decoder.Finish()...)
			if decoder.Err() != nil || len(frames) != 1 || !bytes.Equal(frames[0], []byte(payload)) {
				t.Fatalf("provider data BOM changed: frames=%q error=%v want=%q", frames, decoder.Err(), payload)
			}
		}
	}
}
