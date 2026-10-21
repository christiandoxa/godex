package deepseek

import (
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384DeepSeekUsageDistinguishesMissingFromPresent(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		present     bool
		input       float64
	}{
		{"missing", "", false, 0},
		{"object", `,"usage":{"prompt_tokens":2}`, true, 2},
		{"empty-object", `,"usage":{}`, true, 0},
		{"null", `,"usage":null`, true, 0},
		{"boolean", `,"usage":true`, true, 0},
		{"number", `,"usage":7`, true, 0},
		{"string", `,"usage":"not counters"`, true, 0},
		{"array", `,"usage":[{"prompt_tokens":9}]`, true, 0},
	} {
		for _, stream := range []bool{false, true} {
			mode := "buffered"
			if stream {
				mode = "stream"
			}
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				packet := `{"id":"chat-presence","created":1,"model":"deepseek-v4-pro","choices":[{"message":{"content":"presence-control"},"delta":{"content":"presence-control"}}]` + tc.field + `}`
				response := deepSeekUsagePresenceResponse04384(t, packet, stream)
				usage, present := response["usage"]
				want := map[string]any{"input_tokens": tc.input, "output_tokens": float64(0), "total_tokens": tc.input}
				if present != tc.present || present && !reflect.DeepEqual(usage, want) {
					t.Errorf("usage=%#v present=%v want=%#v present=%v", usage, present, want, tc.present)
				}
			})
		}
	}
}

func TestProdex04384DeepSeekPresentNullUsageReplacesPreviousSnapshot(t *testing.T) {
	for _, tc := range []struct {
		field string
		input float64
	}{
		{"", 2},
		{`,"usage":null`, 0},
		{`,"usage":false`, 0},
	} {
		packet := `{"id":"chat-presence","created":1,"choices":[{"delta":{"content":"presence-control"}}],"usage":{"prompt_tokens":2}}` + "\n\ndata: " + `{"choices":[]` + tc.field + `}`
		response := deepSeekUsagePresenceResponse04384(t, packet, true)
		usage, _ := response["usage"].(map[string]any)
		if usage["input_tokens"] != tc.input || usage["total_tokens"] != tc.input {
			t.Errorf("latest usage field %q retained stale counters: %#v", tc.field, usage)
		}
	}
}

func deepSeekUsagePresenceResponse04384(t *testing.T, packet string, stream bool) map[string]any {
	t.Helper()
	var body []byte
	if stream {
		wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader("data: "+packet+"\n\ndata: [DONE]\n\n")))
		decoder := sse.NewDecoder(1 << 20)
		for _, raw := range append(decoder.Feed([]byte(wire)), decoder.Finish()...) {
			var event struct {
				Type     string          `json:"type"`
				Response json.RawMessage `json:"response"`
			}
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			if event.Type == "response.completed" {
				body = event.Response
			}
			if event.Type == "response.failed" {
				t.Fatal("valid usage fixture failed")
			}
		}
	} else {
		var err error
		body, err = deepSeekChatResponse([]byte(packet), time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil || response == nil {
		t.Fatalf("missing response: %s error=%v", body, err)
	}
	return response
}
