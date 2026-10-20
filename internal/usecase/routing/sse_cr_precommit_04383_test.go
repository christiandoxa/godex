package routing

import (
	"net/http"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

// Tagged runtime_sse_chunk_line_offsets recognizes LF only. CRLF and
// trailing-CR padding work, but a bare CR cannot create a new data/event
// boundary and forge a quota rotation before any model output.
func TestProdex04383SSEBareCRDoesNotForgeQuotaEvent(t *testing.T) {
	const quota = `{"type":"response.failed","response":{"error":{"code":"insufficient_quota"}}}`
	const output = `{"type":"response.output_text.delta","delta":"visible"}`
	for _, tc := range []struct {
		name, wire string
		wantQuota  bool
	}{
		{"CRLF-normal", "data: " + quota + "\r\n\r\n", true},
		{"trailing-CR-before-LF", "data: " + quota + "\r\r\n\n", true},
		{"embedded-CR-must-not-split", "data: " + quota + "\r\rdata: " + output + "\n\n", false},
		{"ordinary-output", "data: " + output + "\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, chunk := range []int{1, 3, len(tc.wire)} {
				decoder := sse.NewDecoder(64 << 10)
				var result []responseOutcome
				for pos := 0; pos < len(tc.wire); pos += chunk {
					for _, data := range decoder.Feed([]byte(tc.wire[pos:min(pos+chunk, len(tc.wire))])) {
						outcome, wait := streamOutcome(data, make(http.Header), time.Unix(1_000_000_000, 0), "openai")
						if !wait {
							result = append(result, outcome)
						}
					}
				}
				for _, data := range decoder.Finish() {
					outcome, wait := streamOutcome(data, make(http.Header), time.Unix(1_000_000_000, 0), "openai")
					if !wait {
						result = append(result, outcome)
					}
				}
				if len(result) != 1 || (result[0].kind == responseRetry) != tc.wantQuota || result[0].quota != tc.wantQuota {
					t.Fatalf("bare CR quota inference: results=%+v wantQuota=%t chunk=%d", result, tc.wantQuota, chunk)
				}
			}
		})
	}
}
