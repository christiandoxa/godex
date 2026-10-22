package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestGeminiFailedTextSnapshotRejectsMatchingCorruption04384(t *testing.T) {
	for _, scenario := range geminiFailedTextSnapshotCases04384() {
		t.Run(scenario, func(t *testing.T) {
			good := geminiCompactExpected04384(scenario)
			if differences := geminiCompare(scenario, good, good); len(differences) != 0 {
				t.Fatal(differences)
			}
			bad := geminiCompactExpected04384(scenario)
			bad.Client.Seed.Body = strings.ReplaceAll(bad.Client.Seed.Body, "after", "forged")
			if len(geminiCompare(scenario, bad, bad)) == 0 {
				t.Fatal("accepted matching post-failure seed text corruption")
			}
			if geminiToolSnapshotCached04384(scenario) {
				bad = geminiCompactExpected04384(scenario)
				request := &bad.Upstream[len(bad.Upstream)-1]
				request.RawBody = strings.ReplaceAll(request.RawBody, "after", "forged")
				if json.Unmarshal([]byte(request.RawBody), &request.Body) != nil || len(geminiCompare(scenario, bad, bad)) == 0 {
					t.Fatal("accepted matching post-failure cached history corruption")
				}
			}
			var wire bytes.Buffer
			writeGeminiFailedTextSnapshot04384(&wire, scenario)
			decoder := sse.NewStrictDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			if decoder.Err() != nil || len(frames) < 4 {
				t.Fatal("failed-text fixture lost complete packets")
			}
			for _, frame := range frames {
				if !sse.JSONDone(frame) && !json.Valid(frame) {
					t.Fatal("failed-text fixture has invalid JSON")
				}
			}
		})
	}
}
