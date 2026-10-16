package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// Codex 0.163.0 produces legitimate terminal response.incomplete values.
// Prodex 0.438.0 forwards their native reason, response ID, usage, unknown
// extensions, and stream event identity; neither path is quota recovery or a
// reason to retry a committed turn on another credential.
func TestProdex04380NativeResponseIncompletePreservesReasonAndUsageWithoutRotation(t *testing.T) {
	const buffered = `{"id":"resp_native_incomplete","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens","extra_code":"opaque"},"model":"gpt-6","usage":{"input_tokens":17,"output_tokens":8,"total_tokens":25,"input_tokens_details":{"cached_tokens":4}},"future_native_field":{"untouched":true},"output":[]}`
	const stream = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_native_incomplete\"}}\n\n" +
		"event: response.incomplete\ndata: {\"type\":\"response.incomplete\",\"response\":" + buffered + "}\n\n"
	for _, tc := range []struct{ name, mime, body string }{
		{name: "buffered native response", mime: "application/json", body: buffered},
		{name: "streamed terminal incomplete", mime: "text/event-stream", body: stream},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/backend-api/codex/responses" && r.URL.Path != "/responses" {
					t.Errorf("unexpected upstream path: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", tc.mime)
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			proxy := newTestProxy(t, upstream.URL, testRuntimeAccounts(t, "A", "synthetic-a", "B", "synthetic-b"))
			for turn := 1; turn <= 2; turn++ {
				response := doProxyJSON(t, proxy.URL+"/backend-api/godex/responses", `{"model":"gpt-6","input":"synthetic request"}`, nil)
				bytes, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
				_ = response.Body.Close()
				if err != nil || response.StatusCode != http.StatusOK {
					t.Fatalf("native incomplete incorrectly classified as error/rotation: status=%d body=%q error=%v", response.StatusCode, bytes, err)
				}
				if actual := string(bytes); actual != tc.body {
					t.Fatalf("Codex 0.163.0 native incomplete reason, usage, IDs, SSE or extension mutated: expected=%q actual=%q", tc.body, actual)
				}
				if got := strings.ToLower(response.Header.Get("Content-Type")); !strings.HasPrefix(got, tc.mime) {
					t.Fatalf("native incomplete content type changed: %q", got)
				}
				if got := calls.Load(); got != int32(turn) {
					t.Fatalf("incomplete response triggered hidden replay: upstream calls=%d turn=%d", got, turn)
				}
			}
		})
	}
}
