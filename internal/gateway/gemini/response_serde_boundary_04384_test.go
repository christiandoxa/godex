package gemini

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Prodex 0.438.4 response_transform.rs calls serde_json::from_slice::<Value>
// for the ENTIRE upstream buffered GenerateContent response. A valid text
// candidate cannot survive if some unrelated member is rejected by serde.
func TestProdex04384GeminiBufferedResponseRequiresCompleteSerdeJSON(t *testing.T) {
	prefix := `{"candidates":[{"content":{"parts":[{"text":"visible-answer"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.1-pro-preview"`
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"control", ``, true},
		{"valid-surrogate-pair", `,"unused":"\ud83d\ude00"`, true},
		{"valid-long-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"lone-high-surrogate", `,"unused":"\ud800"`, false},
		{"lone-low-surrogate", `,"unused":"\udc00"`, false},
		{"invalid-UTF8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
		{"finite-Go-invalid-serde", `,"unused":1.7976931348623158e308`, false},
		{"infinite-number", `,"unused":1e309`, false},
		{"excess-JSON-depth", `,"unused":` + strings.Repeat(`[`, 140) + `0` + strings.Repeat(`]`, 140), false},
	} {
		for _, chat := range []bool{false, true} {
			name := tc.name + "/native"
			if chat {
				name = tc.name + "/chat"
			}
			t.Run(name, func(t *testing.T) {
				response := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
					Body: io.NopCloser(strings.NewReader(prefix + tc.extra + `}`))}
				var resultErr error
				var body []byte
				if chat {
					out, err := translateGeminiChatOrNativeResponse043711(response)
					resultErr = err
					if out != nil {
						body, _ = io.ReadAll(out.Body)
						out.Body.Close()
					}
				} else {
					out, err := translateResponse(response, nil)
					resultErr = err
					if out != nil {
						body, _ = io.ReadAll(out.Body)
						out.Body.Close()
					}
				}
				if accepted := resultErr == nil; accepted != tc.valid {
					t.Fatalf("serde-invalid upstream Gemini response accepted=%t want=%t error=%v body=%s", accepted, tc.valid, resultErr, body)
				}
				if tc.valid && !bytes.Contains(body, []byte("visible-answer")) {
					t.Fatalf("valid Gemini response lost assistant text: %s", body)
				}
			})
		}
	}
}

func TestProdex04384GeminiSSEChunkRequiresCompleteSerdeJSON(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		valid       bool
	}{
		{"canonical", "", true},
		{"valid-long-mantissa", `,"unused":1.79769313486231580e308`, true},
		{"invalid-unpaired-surrogate", `,"unused":"\ud800"`, false},
		{"serde-near-max-overflow", `,"unused":1.7976931348623158e308`, false},
		{"invalid-utf8", `,"unused":"` + string([]byte{0xff}) + `"`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunk := []byte(`{"candidates":[{"content":{"parts":[{"text":"stream-ok"}]}}]` + tc.extra + `}`)
			state := &geminiStreamState{responseID: "resp-fixture"}
			var writer bytes.Buffer
			err := state.consume(&writer, chunk, nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.valid && !strings.Contains(writer.String(), "stream-ok") {
				t.Fatalf("valid Gemini SSE lost text: %q", writer.String())
			}
			if !tc.valid {
				if strings.Contains(writer.String(), "stream-ok") || state.text.Len() != 0 {
					t.Fatalf("serde-invalid Gemini SSE emitted candidate text: %q", writer.String())
				}
				if err := state.consume(&writer, []byte("[DONE]"), nil); err != nil {
					t.Fatal(err)
				}
				if !state.completed || strings.Count(writer.String(), "event: response.failed") != 1 || strings.Contains(writer.String(), "event: response.completed") {
					t.Fatalf("serde-invalid Gemini SSE did not fail exactly once at DONE: %q", writer.String())
				}
			}
		})
	}
}
