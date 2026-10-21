package deepseek

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Published 0.438.4 response_transform.rs parses the complete upstream
// OpenAI ChatCompletion with serde_json before projecting Responses output.
// Ignored invalid JSON fields must not authorize the model's visible answer.
func TestProdex04384DeepSeekBufferedResponseRequiresCompleteSerdeJSON(t *testing.T) {
	const prefix = `{"id":"chatcmpl-synthetic","model":"deepseek-v4-pro","choices":[{"message":{"role":"assistant","content":"visible-answer"},"finish_reason":"stop"}]`
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"valid-paired-surrogate", `,"unused":"\ud83d\ude00"`, true},
		{"valid-long-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"lone-high-surrogate", `,"unused":"\ud800"`, false},
		{"lone-low-surrogate", `,"unused":"\udc00"`, false},
		{"invalid-UTF8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
		{"serde-near-max-overflow", `,"unused":1.7976931348623158e308`, false},
		{"inf-f64", `,"unused":1e309`, false},
		{"overdepth", `,"unused":` + strings.Repeat(`[`, 140) + `0` + strings.Repeat(`]`, 140), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(prefix + tc.extra + `}`)
			raw, err := deepSeekChatResponse(payload, time.Unix(1000, 0))
			if accepted := err == nil; accepted != tc.valid {
				t.Fatalf("invalid serde DeepSeek buffered completion accepted=%t want=%t error=%v raw=%q", accepted, tc.valid, err, raw)
			}
			if !tc.valid {
				return
			}
			if !strings.Contains(string(raw), "visible-answer") {
				t.Fatalf("valid DeepSeek completion lost content %s", raw)
			}
			response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(string(payload)))}
			result, err := translateResponseWithMetadata(response, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer result.Body.Close()
			translated, _ := io.ReadAll(result.Body)
			if !strings.Contains(string(translated), "visible-answer") {
				t.Fatalf("valid buffered DeepSeek translated wire lost text %s", translated)
			}
		})
	}
}

func TestProdex04384DeepSeekSSEChunkRequiresCompleteSerdeJSON(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"valid-long-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"lone-surrogate", `,"unused":"\ud800"`, false},
		{"invalid-UTF8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
		{"serde-near-max-overflow", `,"unused":1.7976931348623158e308`, false},
		{"out-of-range-number", `,"unused":1e309`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := []byte(`{"id":"chatcmpl-synthetic","choices":[{"delta":{"content":"stream-text"}}]` + tc.extra + `}`)
			state := newDeepSeekChatStreamState(42, nil, nil, deepSeekConversationStore{})
			wire, _, err := state.observe(payload)
			if err != nil {
				t.Fatal(err)
			}
			if !tc.valid {
				terminal, _, err := state.observe([]byte("[DONE]"))
				wire = append(wire, terminal...)
				if err != nil || strings.Contains(string(wire), "stream-text") || strings.Contains(string(wire), "event: response.completed") || strings.Count(string(wire), "event: response.failed") != 1 {
					t.Fatalf("invalid serde DeepSeek SSE escaped terminal validation: err=%v wire=%s", err, wire)
				}
				return
			}
			if tc.valid && !strings.Contains(string(wire), "stream-text") {
				t.Fatalf("valid DeepSeek SSE lost content: %q", wire)
			}
		})
	}
}

// A malformed provider SSE frame must stop the live bridge before emitting
// the text delta it contains. A later [DONE] cannot convert an invalid
// serde event into a successful completed assistant response.
func TestProdex04384DeepSeekInvalidSerdeSSEDoesNotCommitSyntheticText(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"valid", "", true},
		{"invalid-surrogate", `,"unused":"\ud800"`, false},
		{"invalid-f64", `,"unused":1.7976931348623158e308`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := `data: {"id":"chatcmpl-synthetic","choices":[{"delta":{"content":"no-fabricated-text"}}]` + tc.extra + `}` + "\n\ndata: [DONE]\n\n"
			result := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader(wire)))
			if tc.valid {
				if !strings.Contains(result, "event: response.completed") || !strings.Contains(result, "no-fabricated-text") {
					t.Fatalf("valid SSE lost content: %s", result)
				}
			} else {
				if !strings.Contains(result, "event: response.failed") || strings.Contains(result, "event: response.completed") || strings.Contains(result, "no-fabricated-text") {
					t.Fatalf("invalid DeepSeek serde SSE fabricated a completed message: %s", result)
				}
			}
		})
	}
}
