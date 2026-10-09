package routing

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04370RealSSEPrecommitInspectorPreservesNestedRetryHeader(t *testing.T) {
	event := map[string]any{
		"type": "response.failed",
		"response": map[string]any{"error": map[string]any{
			"code":    "rate_limit_exceeded",
			"message": "Please try again in 1s.",
			"headers": map[string]any{"Retry-After": "5"},
		}},
	}
	serialized, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	body := "event: response.failed\ndata: " + string(serialized) + "\n\n"
	router := &Router{now: func() time.Time { return time.Unix(1_000_000_000, 0) }}
	response := &proxymodel.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	outcome, pending, err := router.classify(response, "openai")
	if pending != nil {
		defer pending.close()
	}
	if err != nil {
		t.Fatal(err)
	}
	if outcome.kind != responseRetry || !outcome.firstEventRetry ||
		outcome.quarantine != 5*time.Second {
		t.Fatalf("real SSE inspector dropped 5s structured advice: %+v", outcome)
	}
}

func TestProdex04370HTTPDateRetryAdviceIsBounded(t *testing.T) {
	now := time.Unix(1_000_000_000, 0)
	for _, tc := range []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"future", now.Add(7 * time.Second).UTC().Format(http.TimeFormat), 7 * time.Second},
		{"past", now.Add(-time.Minute).UTC().Format(http.TimeFormat), 0},
		{"capped", now.Add(time.Hour).UTC().Format(http.TimeFormat), 5 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]any{
				"type": "response.failed",
				"response": map[string]any{"error": map[string]any{
					"code":    "rate_limit_exceeded",
					"message": "Please try again in 12s.",
					"headers": map[string]any{"Retry-After": tc.header},
				}},
			})
			outcome, _ := streamOutcome(raw, make(http.Header), now, "openai")
			if outcome.kind != responseRetry || outcome.quarantine != tc.want {
				t.Fatalf("HTTP date %q gave %s, want %s", tc.header, outcome.quarantine, tc.want)
			}
		})
	}
}

func TestProdex04370MalformedStructuredAdviceCannotOverrideOrdinaryStream(t *testing.T) {
	root := map[string]any{
		"type": "response.failed",
		"response": map[string]any{"error": map[string]any{
			"code":    "rate_limit_exceeded",
			"message": "Please try again in 12s.",
			"headers": map[string]any{"Retry-After": "5"},
		}},
	}
	encoded, _ := json.Marshal(root)
	if delay, ok := structuredStreamRetryAdvice(encoded, time.Now()); !ok || delay != 5*time.Second {
		t.Fatalf("valid advice rejected: %v %v", delay, ok)
	}
	oversized := append(append([]byte(nil), encoded...), strings.Repeat(" ", maxStreamRetryAdviceBytes)...)
	if _, ok := structuredStreamRetryAdvice(oversized, time.Now()); ok {
		t.Fatal("oversized structured advice escaped bounds")
	}
	root["type"] = "response.completed"
	encoded, _ = json.Marshal(root)
	if _, ok := structuredStreamRetryAdvice(encoded, time.Now()); ok {
		t.Fatal("committed response supplied retry advice")
	}
}
