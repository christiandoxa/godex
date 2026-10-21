package main

import (
	"strings"
	"testing"
	"time"
)

func TestProdex04384ProviderBOMOracleRejectsMatchingWrongFailure(t *testing.T) {
	wire := providerBOMWire04384(time.Now().Unix(), "resp_deepseek_01900000-0000-7000-8000-000000000001")
	if !equivalentProviderBOMSSE04384(wire, wire) {
		t.Fatal("canonical provider BOM failure rejected")
	}
	for _, mutation := range []struct{ from, to string }{
		{"incomplete JSON segment", "unexpected end of stream"},
		{"provider_stream_error", "invalid_json"}, {`"sequence_number":0`, `"sequence_number":1`},
		{"response.failed", "response.completed"}, {"\r\n", "\n"},
		{"resp_deepseek_01900000-0000-7000-8000-000000000001", "chat-bom"},
	} {
		changed := strings.Replace(wire, mutation.from, mutation.to, 1)
		if changed == wire || equivalentProviderBOMSSE04384(changed, changed) {
			t.Fatalf("BOM failure mutation escaped: %v", mutation)
		}
	}
	if equivalentProviderBOMSSE04384(wire+wire, wire+wire) {
		t.Fatal("duplicate failure events accepted")
	}
}
