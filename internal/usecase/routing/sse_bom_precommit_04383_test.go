package routing

import (
	"net/http"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// The 0.438.3 Mojo SSE line classifier treats BOM on the wire field name as
// opaque, but runtime_sse_payload_join strips all leading BOMs from the
// first `data:` value before serde_json and quota/rate inspection.
func TestProdex04383SSEBOMPlacementChangesPrecommitRotation(t *testing.T) {
	const bom = "\xef\xbb\xbf"
	quota := `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`
	output := `{"type":"response.output_text.delta","delta":"visible"}`
	for _, tc := range []struct {
		name, wire string
		wantQuota  bool
	}{
		{"data-payload-once", "data: " + bom + quota + "\n\n", true},
		{"data-payload-twice", "data: " + bom + bom + quota + "\n\n", true},
		{"data-raw-control", "data: " + quota + "\n\n", true},
		{"wire-field-bom-ignored", bom + "data: " + quota + "\n\ndata: " + output + "\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decoder := sse.NewDecoder(64 << 10)
			var got []byte
			for start := 0; start < len(tc.wire); start += 3 {
				for _, payload := range decoder.Feed([]byte(tc.wire[start:min(start+3, len(tc.wire))])) {
					if outcome, wait := streamOutcome(payload, make(http.Header), time.Unix(1_000_000_000, 0), "openai"); !wait {
						if tc.wantQuota && (outcome.kind != responseRetry || !outcome.quota) {
							t.Fatalf("data-BOM quota was lost: %+v", outcome)
						}
						if !tc.wantQuota && (outcome.kind != responsePass || outcome.quota) {
							t.Fatalf("wire-field BOM forged quota rotation: %+v", outcome)
						}
						got = append(got, payload...)
					}
				}
			}
			if len(got) == 0 {
				t.Fatal("valid SSE content was not processed")
			}
		})
	}
}
