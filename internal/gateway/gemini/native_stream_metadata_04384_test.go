package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestProdex04384GeminiStreamMetadataUsesLatestProviderFields(t *testing.T) {
	for _, tc := range []struct {
		name, first, next, want string
	}{
		{"finish", `"finishReason":"FIRST"`, `"finishReason":"STOP"`, `"finishReason":"STOP"`},
		{"empty-string", `"finishMessage":"old"`, `"finishMessage":""`, `"finishMessage":""`},
		{"empty-array", `"safetyRatings":[{"category":"old"}]`, `"safetyRatings":[]`, `"safetyRatings":[]`},
		{"scalar", `"avgLogprobs":1`, `"avgLogprobs":2`, `"avgLogprobs":2`},
		{"false", `"logprobsResult":true`, `"logprobsResult":false`, `"logprobsResult":false`},
		{"nested-snapshot", `"groundingMetadata":{"keep":1,"nested":{"keep":1,"replace":1}}`, `"groundingMetadata":{"nested":{"replace":2}}`, `"groundingMetadata":{"nested":{"replace":2}}`},
		{"scalar-to-object", `"groundingMetadata":7`, `"groundingMetadata":{"new":2}`, `"groundingMetadata":{"new":2}`},
		{"object-to-scalar", `"groundingMetadata":{"old":1}`, `"groundingMetadata":7`, `"groundingMetadata":7`},
		{"null-does-not-clear", `"groundingMetadata":{"keep":1}`, `"groundingMetadata":null`, `"groundingMetadata":{"keep":1}`},
		{"absent-retains", `"groundingMetadata":{"keep":1}`, ``, `"groundingMetadata":{"keep":1}`},
		{"new-field-retains-others", `"groundingMetadata":{"keep":1}`, `"citationMetadata":{"new":2}`, `"groundingMetadata":{"keep":1},"citationMetadata":{"new":2}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := `{"candidates":[{"content":{"parts":[{"text":"metadata-control"}]},` + tc.first + `}]}`
			next := `{"candidates":[{` + tc.next + `}]}`
			events := geminiStreamReplay04384(t, first, next)
			response := events[len(events)-1]["response"].(map[string]any)
			metadata := response["metadata"].(map[string]any)["gemini"]
			var want map[string]any
			if err := json.Unmarshal([]byte(`{`+tc.want+`}`), &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(metadata, want) {
				t.Errorf("stream retained stale provider metadata: %#v want=%#v", metadata, want)
			}
		})
	}
}

func TestProdex04384GeminiStreamUsageMetadataReplacesWholeSnapshot(t *testing.T) {
	first := `{"usageMetadata":{"promptTokenCount":1,"totalTokenCount":1,"oldField":"drop"},"promptFeedback":{"first":1},"candidates":[{"content":{"parts":[{"text":"ok"}]}}]}`
	next := `{"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":3,"totalTokenCount":5},"promptFeedback":{"next":2}}`
	wire := "data: " + first + "\n\ndata: " + next + "\n\ndata: [DONE]\n\n"
	response, err := translateResponse(&http.Response{StatusCode: http.StatusOK,
		Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))},
		map[string]any{"request_id": "req-retained", "client_metadata": map[string]any{"client": "fixture"}})
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var final map[string]any
	for _, line := range bytes.Split(body, []byte("\n")) {
		if !bytes.HasPrefix(line, []byte("data: ")) {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(line[len("data: "):]), &event); err != nil {
			t.Fatal(err)
		}
		if event["type"] == "response.completed" {
			final = event["response"].(map[string]any)
		}
	}
	metadata, _ := final["metadata"].(map[string]any)
	provider, _ := metadata["gemini"].(map[string]any)
	want := map[string]any{"usageMetadata": map[string]any{"promptTokenCount": float64(2), "candidatesTokenCount": float64(3), "totalTokenCount": float64(5)}, "promptFeedback": map[string]any{"next": float64(2)}}
	usage, _ := final["usage"].(map[string]any)
	if !reflect.DeepEqual(provider, want) || usage["total_tokens"] != float64(5) || metadata["request_id"] != "req-retained" || !reflect.DeepEqual(metadata["client_metadata"], map[string]any{"client": "fixture"}) {
		t.Errorf("stream usage metadata disagrees with latest usage/request ownership: %#v", final)
	}
}
