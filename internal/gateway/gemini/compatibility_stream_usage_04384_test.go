package gemini

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384GeminiChatSSENormalizesPresentUsage(t *testing.T) {
	valid := `{"prompt_tokens":9007199254740993,"completion_tokens":1,"total_tokens":9007199254740994}`
	counts := `{"input_tokens":9007199254740993,"output_tokens":1,"total_tokens":9007199254740994}`
	zero := `{"input_tokens":0,"output_tokens":0,"total_tokens":0}`
	for _, fixture := range []struct{ name, first, last, want string }{
		{"empty", "", `{}`, zero},
		{"null replaces counts", valid, `null`, zero},
		{"array", "", `[]`, zero},
		{"scalar", "", `false`, zero},
		{"missing total", "", `{"prompt_tokens":9007199254740993,"completion_tokens":1}`, counts},
		{"saturating total", "", `{"prompt_tokens":18446744073709551615,"completion_tokens":1}`, `{"input_tokens":18446744073709551615,"output_tokens":1,"total_tokens":18446744073709551615}`},
		{"invalid counters", "", `{"prompt_tokens":-1,"completion_tokens":2.0,"total_tokens":"9"}`, zero},
		{"overflow input", "", `{"prompt_tokens":18446744073709551616,"completion_tokens":1}`, `{"input_tokens":0,"output_tokens":1,"total_tokens":1}`},
		{"invalid total", "", `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3.0}`, `{"input_tokens":2,"output_tokens":1,"total_tokens":3}`},
		{"zero total", "", `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":0}`, `{"input_tokens":2,"output_tokens":1,"total_tokens":0}`},
		{"cache miss only", "", `{"prompt_tokens":2,"completion_tokens":1,"prompt_cache_miss_tokens":5}`, `{"input_tokens":2,"metadata":{"gemini":{"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":5}},"output_tokens":1,"total_tokens":3}`},
		{"zero details", "", `{"prompt_tokens":2,"completion_tokens":1,"total_tokens":0,"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":0,"completion_tokens_details":{"reasoning_tokens":0}}`, `{"input_tokens":2,"input_tokens_details":{"cached_tokens":0},"metadata":{"gemini":{"prompt_cache_hit_tokens":0,"prompt_cache_miss_tokens":0}},"output_tokens":1,"output_tokens_details":{"reasoning_tokens":0},"total_tokens":0}`},
		{"unsigned details", "", `{"prompt_cache_hit_tokens":18446744073709551615,"completion_tokens_details":{"reasoning_tokens":9223372036854775808}}`, `{"input_tokens":0,"input_tokens_details":{"cached_tokens":18446744073709551615},"metadata":{"gemini":{"prompt_cache_hit_tokens":18446744073709551615,"prompt_cache_miss_tokens":0}},"output_tokens":0,"output_tokens_details":{"reasoning_tokens":9223372036854775808},"total_tokens":0}`},
		{"invalid details", "", `{"prompt_cache_hit_tokens":1e2,"completion_tokens_details":{"reasoning_tokens":false}}`, zero},
		{"valid", "", valid, counts},
		{"null then valid", `null`, valid, counts},
		{"absent retains prior", valid, "", counts},
		{"absent stays absent", "", "", ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			state := geminiChatCompatibleStream043711{id: "resp_deepseek_synthetic", created: 1}
			var wire bytes.Buffer
			for _, value := range []string{fixture.first, fixture.last} {
				packet := `{"choices":[{"delta":{"content":"synthetic"},"finish_reason":"stop"}]`
				if value != "" {
					packet += `,"usage":` + value
				}
				if err := state.accept(&wire, []byte(packet+"}")); err != nil {
					t.Fatal(err)
				}
				if state.done || strings.Contains(wire.String(), "response.completed") {
					t.Fatal("usage committed the stream before DONE")
				}
			}
			for range 2 {
				if err := state.accept(&wire, []byte("[DONE]")); err != nil {
					t.Fatal(err)
				}
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(wire.Bytes()), decoder.Finish()...)
			var usage string
			for _, frame := range frames {
				var event struct {
					Type     string                          `json:"type"`
					Response struct{ Usage json.RawMessage } `json:"response"`
				}
				if json.Unmarshal(frame, &event) == nil && event.Type == "response.completed" {
					usage = string(event.Response.Usage)
				}
			}
			if usage != fixture.want || strings.Count(wire.String(), "event: response.completed") != 1 {
				t.Fatalf("usage=%s, want %s", usage, fixture.want)
			}
		})
	}
}
