package gemini

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProdex04384GeminiChatSSERejectsNewToolWithoutFunction(t *testing.T) {
	for _, fixture := range []struct {
		name, calls string
		rejected    bool
	}{
		{"missing function", `[{"index":0,"id":"call_synthetic"}]`, true},
		{"null function", `[{"index":8,"function":null}]`, true},
		{"wrong function type", `[{"function":false}]`, true},
		{"empty array", `[]`, false},
		{"wrong array type", `{}`, false},
		{"absent", "", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			fields := `"content":"before"`
			if fixture.calls != "" {
				fields += `,"tool_calls":` + fixture.calls
			}
			for _, packet := range []string{
				`{"choices":[{"delta":{` + fields + `},"finish_reason":"stop"}]}`,
				`{"choices":[{"delta":{"content":"after"}}]}`,
				"[DONE]", "[DONE]",
			} {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			failures, completed := strings.Count(wire.String(), "event: response.failed"), strings.Count(wire.String(), "event: response.completed")
			if !strings.Contains(wire.String(), `"delta":"before"`) || !strings.Contains(wire.String(), `"delta":"after"`) {
				t.Fatal("tool admission lost preceding or subsequent text")
			}
			if fixture.rejected {
				if failures != 1 || completed != 0 || state.text.String() != "before" || !strings.Contains(wire.String(), `"code":"invalid_tool_call_arguments","message":"Gemini OpenAI-compatible streamed a tool call without a function object"`) {
					t.Fatal("new malformed tool did not fail exactly once without completion")
				}
			} else if failures != 0 || completed != 1 {
				t.Fatal("non-tool control did not complete exactly once")
			}
		})
	}
}

func TestProdex04384GeminiChatSSESemanticFailureKeepsReading(t *testing.T) {
	for _, first := range []string{
		`{"error":{"code":"synthetic_error","message":"synthetic semantic error"},"choices":[{"delta":{"content":"must-not-emit"}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{}]}}]}`,
	} {
		t.Run(first, func(t *testing.T) {
			source := io.NopCloser(io.MultiReader(
				strings.NewReader("data: "+first+"\n\n"),
				strings.NewReader("data: "+`{"choices":[{"delta":{"content":"after"}}]}`+"\n\n"),
				strings.NewReader("data: [DONE]\n\ndata: "+`{"choices":[{"delta":{"content":"past-done"}}]}`+"\n\n"),
			))
			upstream := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: source}
			body := translateGeminiChatCompatibleStream043711(context.Background(), upstream, nil, nil).Body
			defer body.Close()
			wire, err := io.ReadAll(body)
			if err != nil {
				t.Fatal(err)
			}
			text := string(wire)
			if strings.Count(text, "event: response.failed") != 1 || !strings.Contains(text, `"delta":"after"`) || strings.Contains(text, "must-not-emit") || strings.Contains(text, "past-done") || strings.Contains(text, "event: response.completed") || strings.Contains(text, "event: response.output_item.done") {
				t.Fatal("semantic failure lost subsequent content or terminal/EOF ownership")
			}
		})
	}
}

func TestProdex04384GeminiChatSSEKnownToolIndexMayOmitFunction(t *testing.T) {
	for _, fixture := range []struct {
		first, next string
		rejected    bool
	}{
		{"0", "0", false},
		{"0", "null", false},
		{"0", "false", false},
		{"0", "1.0", false},
		{"0", "-1", false},
		{"0", "1", true},
		{"7", "0", true},
		{"18446744073709551615", "18446744073709551615", false},
	} {
		t.Run(fixture.first+" to "+fixture.next, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			for _, packet := range []string{
				`{"choices":[{"delta":{"tool_calls":[{"index":` + fixture.first + `,"function":{}}]}}]}`,
				`{"choices":[{"delta":{"content":"later","tool_calls":[{"index":` + fixture.next + `}]}}]}`,
			} {
				if err := state.accept(&wire, []byte(packet)); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Contains(wire.String(), "event: response.failed") != fixture.rejected || !strings.Contains(wire.String(), `"delta":"later"`) {
				t.Fatal("tool admission lost existing index ownership or serde index defaults")
			}
		})
	}
}

func TestProdex04384GeminiChatSSEBoundsRetainedToolIndices(t *testing.T) {
	state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1, toolIndices: make(map[uint64]struct{}, geminiStreamMetadataMaxBytes04384/128)}
	for index := range geminiStreamMetadataMaxBytes04384 / 128 {
		state.toolIndices[uint64(index)] = struct{}{}
	}
	var wire bytes.Buffer
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"tool_calls":[{"function":{}}]}}]}`)); err != nil || state.done {
		t.Fatal("known tool index consumed another ownership slot")
	}
	if err := state.accept(&wire, []byte(`{"choices":[{"delta":{"tool_calls":[{"index":18446744073709551615,"function":{}}]}}]}`)); err != nil {
		t.Fatal(err)
	}
	if !state.done || !state.eof || len(state.toolIndices) != geminiStreamMetadataMaxBytes04384/128 || strings.Count(wire.String(), "event: response.failed") != 1 || !strings.Contains(wire.String(), "provider tool state exceeded safe size limit") {
		t.Fatal("tool index retention exceeded its budget or lost the explicit failure")
	}
}
