package main

import (
	"strings"
	"testing"
)

func TestProdex04384StreamMetadataOracleRejectsRoundedAndLostSnapshots(t *testing.T) {
	body := strings.Replace(fixtureStreamWire("msg_deepseek_42"), `{"finish_reason":"stop"}`, exactStreamMetadata04384, 1)
	if _, ok := canonicalStreamMetadata04384(body); !ok || !equivalentStreamMetadata04384(body, body) {
		t.Fatal("exact metadata fixture rejected")
	}
	for _, changed := range []string{
		strings.Replace(body, "9007199254740993", "9007199254740992", 1),
		strings.Replace(body, "18446744073709551615", "18446744073709551616", 1),
		strings.Replace(body, `,"logprobs":{"token_count":9007199254740993}`, "", 1),
		strings.Replace(body, `"annotations":[{"source_index":18446744073709551615}],`, "", 1),
		strings.Replace(body, "synthetic-ok", "wrong-output", 1),
		body + body,
	} {
		if _, ok := canonicalStreamMetadata04384(changed); ok || equivalentStreamMetadata04384(body, changed) {
			t.Fatal("metadata oracle accepted rounded/lost counters or other stream changes")
		}
	}
}
