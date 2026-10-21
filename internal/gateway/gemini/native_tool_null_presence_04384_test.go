package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The tagged gemini_response_tool_call_item_with_call_id obtains the arg
// member with get("args").cloned().unwrap_or({}). Explicit serde Null
// therefore remains the JSON string "null", unlike a truly absent member.
func TestProdex04384GeminiBufferedFunctionArgsNullPresence(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    any
		hasArgs bool
		want    string
	}{
		{"absent", nil, false, `{}`},
		{"explicit-null", nil, true, `null`},
		{"empty-object", map[string]any{}, true, `{}`},
		{"array", []any{1, 2}, true, `[1,2]`},
		{"boolean", false, true, `false`},
		{"string", "", true, `""`},
		{"number", json.Number("0"), true, `0`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			function := map[string]any{"name": "read_file", "id": "call-presence"}
			if tc.hasArgs {
				function["args"] = tc.args
			}
			item := geminiNativeToolCallItem(map[string]any{}, function, "call-fallback")
			if item["arguments"] != tc.want {
				t.Fatalf("Gemini buffered argument presence %s result=%#v want=%q", tc.name, item, tc.want)
			}
		})
	}
}

func TestProdex04384GeminiBufferedNullArgsSurviveResponseEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name string
		args string
		want string
	}{
		{"explicit-null", `,"args":null`, `null`},
		{"absent", "", `{}`},
		{"empty-object", `,"args":{}`, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			native := `{"responseId":"resp-buffered-args","candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-buffered","name":"read_file"` + tc.args + `}}]},"finishReason":"STOP"}]}`
			for _, wire := range []string{native, `{"traceId":"trace-synthetic","response":` + native + `}`} {
				for _, chatCompatible := range []bool{false, true} {
					upstream := &http.Response{StatusCode: http.StatusOK,
						Header: http.Header{"Content-Type": {"application/json"}},
						Body:   io.NopCloser(strings.NewReader(wire))}
					translated, err := translateResponseWithCallFallback04380(upstream, nil, chatCompatible)
					if err != nil {
						t.Fatal(err)
					}
					var response map[string]any
					err = json.NewDecoder(translated.Body).Decode(&response)
					translated.Body.Close()
					if err != nil {
						t.Fatal(err)
					}
					items, ok := response["output"].([]any)
					if !ok || len(items) != 1 {
						t.Fatalf("unexpected output: %#v", response)
					}
					item := items[0].(map[string]any)
					if item["type"] != "function_call" || item["call_id"] != "call-buffered" || item["name"] != "read_file" || item["arguments"] != tc.want {
						t.Fatalf("buffered tool output=%#v want arguments=%q", item, tc.want)
					}
				}
			}
		})
	}
}

func TestProdex04384GeminiBufferedSpecialToolArgsPresence(t *testing.T) {
	for _, name := range []string{"tool_search", "apply_patch"} {
		for _, present := range []bool{false, true} {
			call := map[string]any{"name": name}
			if present {
				call["args"] = nil
			}
			item := geminiNativeToolCallItem(nil, call, "call-special")
			if name == "tool_search" {
				_, object := item["arguments"].(map[string]any)
				if present && item["arguments"] != nil || !present && !object {
					t.Fatalf("tool_search args presence=%v item=%#v", present, item)
				}
			} else {
				want := "{}"
				if present {
					want = "null"
				}
				if item["input"] != want {
					t.Fatalf("apply_patch args presence=%v item=%#v", present, item)
				}
			}
		}
	}
}
