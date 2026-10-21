package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProdex04384GeminiStreamEmbeddedErrorGoldens(t *testing.T) {
	data, err := os.ReadFile("testdata/native_stream_error_04384.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name  string
		Input json.RawMessage
		Pair  []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			chunk := tc.Input
			if tc.Pair != nil {
				chunk = append([]byte(`{"responseId":"must-not-own","promptFeedback":{"blockReason":"SAFETY"},"candidates":[{"content":{"parts":[{"text":"must-not-emit"},{"functionCall":{"name":"read_file","args":{}}}]}}],`), tc.Input[1:]...)
			}
			state := &geminiStreamState{responseID: "resp_gemini_fixture"}
			var output bytes.Buffer
			if err := state.consume(&output, chunk, nil); err != nil {
				t.Fatal(err)
			}
			if tc.Pair == nil {
				if state.completed || strings.Contains(output.String(), "event: response.failed") {
					t.Errorf("missing error became terminal: %s", output.String())
				}
				return
			}
			want := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":0,\"response\":{\"error\":{\"code\":%s,\"message\":%s},\"id\":\"resp_gemini_fixture\"},\"sequence_number\":0,\"type\":\"response.failed\"}\r\n\r\n", jsonLiteral04384(t, tc.Pair[0]), jsonLiteral04384(t, tc.Pair[1]))
			if !state.completed || output.String() != want || state.metadata != nil || len(state.toolCalls) != 0 || state.text.Len() != 0 {
				t.Errorf("embedded error lost precedence or exact bytes: %q want=%q state=%#v", output.String(), want, state)
			}
			before := output.String()
			for _, later := range []string{`{"candidates":[{"content":{"parts":[{"text":"later"}]}}]}`, "[DONE]", `invalid`} {
				if err := state.consume(&output, []byte(later), nil); err != nil || output.String() != before {
					t.Errorf("terminal embedded error accepted later data: %q err=%v", output.String(), err)
				}
			}
		})
	}
}

func TestProdex04384GeminiPromptBlockPrecedesCandidateParts(t *testing.T) {
	for _, reason := range []string{"SAFETY", " 地域ポリシー 🚫 ", "\ufeff", "", "\u2003", "\u0085"} {
		t.Run(fmt.Sprintf("%q", reason), func(t *testing.T) {
			chunk := `{"responseId":"resp-blocked","promptFeedback":{"blockReason":` + jsonLiteral04384(t, reason) + `},"candidates":[{"content":{"parts":[{"text":"visible-control"},{"functionCall":{"name":"read_file","args":{}}}]},"finishReason":"MAX_TOKENS"}]}`
			state := &geminiStreamState{responseID: "resp_gemini_fixture"}
			var output bytes.Buffer
			if err := state.consume(&output, []byte(chunk), nil); err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(reason) == "" {
				if !strings.Contains(output.String(), "visible-control") || strings.Contains(output.String(), "gemini_prompt_blocked") {
					t.Errorf("blank prompt feedback suppressed a valid candidate: %s", output.String())
				}
				return
			}
			var event map[string]any
			payload := strings.TrimSuffix(strings.TrimPrefix(output.String(), "event: response.failed\r\ndata: "), "\r\n\r\n")
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				t.Fatalf("prompt block emitted candidate events: %q, %v", output.String(), err)
			}
			want := map[string]any{"type": "response.failed", "sequence_number": float64(0), "created_at": float64(0),
				"response": map[string]any{"id": "resp-blocked", "error": map[string]any{"code": "gemini_prompt_blocked", "message": "Gemini blocked the prompt: " + reason}}}
			if !reflect.DeepEqual(event, want) || !state.completed || state.metadata["promptFeedback"] == nil || state.text.Len() != 0 || len(state.toolCalls) != 0 {
				t.Errorf("prompt feedback lost metadata precedence/raw reason: %#v want=%#v state=%#v", event, want, state)
			}
		})
	}
}

func TestProdex04384GeminiStreamFailureRedactsProviderMessage(t *testing.T) {
	bearer := strings.Join([]string{"fixture", "bearer", "04384"}, "_")
	apiKey := "sk-" + strings.Repeat("a", 24)
	message := "Authorization: Bearer " + bearer + " url=https://example.test?api_key=" + apiKey
	for _, member := range []string{`"error":{"message":` + jsonLiteral04384(t, message) + `}`, `"promptFeedback":{"blockReason":` + jsonLiteral04384(t, message) + `}`} {
		events := geminiStreamReplay04384(t, `{`+member+`}`)
		if len(events) != 1 || events[0]["type"] != "response.failed" {
			t.Fatalf("provider error did not terminate: %#v", events)
		}
		failure := events[0]["response"].(map[string]any)["error"].(map[string]any)
		got := failure["message"].(string)
		if strings.Contains(got, bearer) || strings.Contains(got, apiKey) || !strings.Contains(got, "Authorization: Bearer <redacted>") || !strings.Contains(got, "api_key=<redacted>") {
			t.Error("provider failure did not redact synthetic credential shapes")
		}
	}
}

func TestProdex04384GeminiStreamEmbeddedFailureAfterCommittedText(t *testing.T) {
	events := geminiStreamReplay04384(t,
		`{"responseId":"resp-owned","candidates":[{"content":{"parts":[{"text":"committed"}]}}]}`,
		`{"responseId":"resp-rejected","error":{"code":503,"message":"synthetic upstream failure"},"candidates":[{"content":{"parts":[{"text":"must-not-emit"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"must-not-emit-later"}]}}]}`)
	var text []any
	failures := 0
	for _, event := range events {
		switch event["type"] {
		case "response.output_text.delta":
			text = append(text, event["delta"])
		case "response.failed":
			failures++
		case "response.completed", "response.incomplete":
			t.Fatalf("provider failure committed another terminal response: %#v", event)
		}
	}
	last := events[len(events)-1]
	want := map[string]any{"id": "resp-owned", "error": map[string]any{"code": "503", "message": "synthetic upstream failure"}}
	if failures != 1 || !reflect.DeepEqual(text, []any{"committed"}) || !reflect.DeepEqual(last["response"], want) || last["created_at"] == nil || last["created_at"].(float64) <= 0 {
		t.Errorf("committed stream lost ownership/failure: %#v", events)
	}
}

func jsonLiteral04384(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
