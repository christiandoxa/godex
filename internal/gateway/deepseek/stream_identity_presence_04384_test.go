package deepseek

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384DeepSeekStreamKeepsIdentityAndModelPresence(t *testing.T) {
	for _, tc := range []struct {
		name, id, firstModel, lastModel, wantModel string
		wantPresent                                bool
	}{
		{"empty", "", `,"model":""`, "", "", true},
		{"whitespace", "  supplied-id  ", `,"model":"  supplied-model  "`, "", "  supplied-model  ", true},
		{"ordinary", "chat-supplied", `,"model":"deepseek-v4-pro"`, "", "deepseek-v4-pro", true},
		{"missing-model", "chat-supplied", "", "", "", false},
		{"null-model", "chat-supplied", `,"model":null`, "", "", false},
		{"late-empty-model", "chat-supplied", `,"model":"old"`, `,"model":""`, "", true},
		{"late-null-model", "chat-supplied", `,"model":"old"`, `,"model":null`, "old", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, _ := json.Marshal(tc.id)
			first := `{"id":` + string(id) + `,"created":1,"choices":[]` + tc.firstModel + `}`
			last := `{"id":"must-not-replace","choices":[{"delta":{"content":"identity-control"}}]` + tc.lastModel + `}`
			wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader("data: "+first+"\n\ndata: "+last+"\n\ndata: [DONE]\n\n")))
			decoder := sse.NewDecoder(1 << 20)
			completed := 0
			for _, raw := range append(decoder.Feed([]byte(wire)), decoder.Finish()...) {
				var event map[string]any
				if err := json.Unmarshal(raw, &event); err != nil {
					t.Fatal(err)
				}
				if response, ok := event["response"].(map[string]any); ok && response["id"] != tc.id {
					t.Errorf("response identity changed: %#v", event)
				}
				if value, present := event["response_id"]; present && value != tc.id {
					t.Errorf("event identity changed: %#v", event)
				}
				if event["type"] == "response.completed" {
					completed++
					response, _ := event["response"].(map[string]any)
					model, present := response["model"]
					if present != tc.wantPresent || present && model != tc.wantModel {
						t.Errorf("model=%#v present=%v want=%q present=%v", model, present, tc.wantModel, tc.wantPresent)
					}
				}
			}
			if completed != 1 {
				t.Fatalf("completion count=%d stream=%s", completed, wire)
			}
		})
	}
}
