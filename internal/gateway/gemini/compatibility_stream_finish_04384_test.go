package gemini

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiAPIKeyFinishWaitsForDone(t *testing.T) {
	for _, delta := range []string{`{}`, `{"content":"last"}`} {
		t.Run(delta, func(t *testing.T) {
			state := &geminiChatCompatibleStream043711{id: "resp-owned", created: 1}
			var output bytes.Buffer
			chunks := []string{
				`{"choices":[{"delta":{"content":"first"}}]}`,
				`{"choices":[{"delta":` + delta + `,"finish_reason":"stop"}]}`,
				`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
				`{"choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
			}
			for _, chunk := range chunks {
				if err := state.accept(&output, []byte(chunk)); err != nil {
					t.Fatal(err)
				}
				if state.done || strings.Contains(output.String(), "event: response.completed") || strings.Contains(output.String(), "event: response.output_item.done") {
					t.Fatalf("finish committed before DONE/trailing usage: %s", output.String())
				}
			}
			if err := state.accept(&output, []byte("[DONE]")); err != nil {
				t.Fatal(err)
			}
			if !state.done || strings.Count(output.String(), "event: response.completed") != 1 || strings.Count(output.String(), "event: response.output_item.done") != 1 || !strings.Contains(output.String(), `"total_tokens":5`) {
				t.Errorf("DONE failed to complete once with latest usage: %s", output.String())
			}
			before := output.String()
			if err := state.accept(&output, []byte(chunks[0])); err != nil || output.String() != before {
				t.Errorf("completed stream accepted later text: err=%v output=%s", err, output.String())
			}
		})
	}
}

func TestProdex04384GeminiAPIKeyFinishWithoutDoneFailsAtEOF(t *testing.T) {
	for _, finish := range []bool{false, true} {
		wire := "data: " + `{"choices":[{"delta":{"content":"committed"}}]}` + "\n\n"
		if finish {
			wire += "data: " + `{"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\n"
		}
		response := translateGeminiChatCompatibleStream043711(&http.Response{StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(wire))}, nil, nil)
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		decoder := sse.NewDecoder(1 << 20)
		var kinds []string
		for _, raw := range append(decoder.Feed(data), decoder.Finish()...) {
			var event map[string]any
			if err := json.Unmarshal(raw, &event); err != nil {
				t.Fatal(err)
			}
			kinds = append(kinds, event["type"].(string))
		}
		if len(kinds) == 0 || kinds[len(kinds)-1] != "response.failed" || strings.Contains(string(data), "event: response.completed") || !strings.Contains(string(data), `"delta":"committed"`) {
			t.Errorf("EOF finish=%v committed an incomplete upstream stream: events=%v wire=%s", finish, kinds, data)
		}
	}
}
