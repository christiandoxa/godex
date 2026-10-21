package deepseek

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384DeepSeekToolSearchKeepsCanonicalArgumentNumbers(t *testing.T) {
	for _, tc := range []struct{ value, canonical string }{
		{"0", "0"}, {"7", "7"}, {"9007199254740993", "9007199254740993"},
		{"9223372036854775808", "9223372036854775808"}, {"18446744073709551615", "18446744073709551615"},
		{"null", "null"}, {"[18446744073709551615]", "[18446744073709551615]"},
		{"1.0", "1.0"}, {"1e2", "100.0"}, {"-0", "-0.0"},
		{"18446744073709551616", "1.8446744073709552e+19"},
		{"1.79769313486231580e308", "1.7976931348623157e+308"},
	} {
		for _, stream := range []bool{false, true} {
			mode := "/buffered"
			if stream {
				mode = "/stream"
			}
			t.Run(tc.value+mode, func(t *testing.T) {
				arguments := `{"value":` + tc.value + `}`
				want := `{"value":` + tc.canonical + `}`
				quoted, _ := json.Marshal(arguments)
				tool := `{"id":"call-search","index":0,"function":{"name":"tool_search","arguments":` + string(quoted) + `}}`
				packet := `{"id":"chat-search","model":"deepseek-v4-pro","created":1,"choices":[{"message":{"tool_calls":[` + tool + `]},"delta":{"tool_calls":[` + tool + `]},"finish_reason":"tool_calls"}]}`
				var response map[string]any
				if stream {
					wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader("data: "+packet+"\n\ndata: [DONE]\n\n")))
					decoder := sse.NewDecoder(1 << 20)
					items := 0
					for _, raw := range append(decoder.Feed([]byte(wire)), decoder.Finish()...) {
						var event map[string]any
						valueDecoder := json.NewDecoder(bytes.NewReader(raw))
						valueDecoder.UseNumber()
						if err := valueDecoder.Decode(&event); err != nil {
							t.Fatal(err)
						}
						if event["type"] == "response.output_item.done" {
							item, _ := event["item"].(map[string]any)
							assertDeepSeekSearchArguments04384(t, item, want)
							items++
						}
						if event["type"] == "response.completed" {
							response, _ = event["response"].(map[string]any)
						}
						if event["type"] == "response.failed" {
							t.Fatal("valid tool-search argument stream failed")
						}
					}
					if items != 1 {
						t.Fatalf("tool-search done items=%d want=1", items)
					}
				} else {
					body, err := deepSeekChatResponse([]byte(packet), time.Unix(1, 0))
					if err != nil {
						t.Fatal(err)
					}
					decoder := json.NewDecoder(bytes.NewReader(body))
					decoder.UseNumber()
					if err := decoder.Decode(&response); err != nil {
						t.Fatal(err)
					}
				}
				output, _ := response["output"].([]any)
				if len(output) != 1 {
					t.Fatalf("tool-search final output missing: %#v", response)
				}
				item, _ := output[0].(map[string]any)
				assertDeepSeekSearchArguments04384(t, item, want)
			})
		}
	}
}

func assertDeepSeekSearchArguments04384(t *testing.T, item map[string]any, want string) {
	t.Helper()
	encoded, err := json.Marshal(item["arguments"])
	if err != nil || string(encoded) != want || item["type"] != "tool_search_call" || item["call_id"] != "call-search" || item["execution"] != "client" {
		t.Fatalf("tool-search argument changed: got=%s want=%s item=%#v error=%v", encoded, want, item, err)
	}
}
