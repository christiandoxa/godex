package deepseek

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384DeepSeekUsageKeepsFullUnsignedCounters(t *testing.T) {
	for _, tc := range []struct{ name, count, total, wantTotal string }{
		{"ordinary", "2", "3", "3"},
		{"above-f64-exact", "9007199254740993", "9007199254740994", "9007199254740994"},
		{"above-signed", "9223372036854775808", "9223372036854775809", "9223372036854775809"},
		{"unsigned-max", "18446744073709551615", "18446744073709551615", "18446744073709551615"},
		{"saturating-total", "18446744073709551615", "", "18446744073709551615"},
	} {
		for _, stream := range []bool{false, true} {
			name := tc.name + "/buffered"
			if stream {
				name = tc.name + "/stream"
			}
			t.Run(name, func(t *testing.T) {
				usage := `{"prompt_tokens":` + tc.count + `,"completion_tokens":1,"prompt_cache_hit_tokens":` + tc.count
				if tc.total != "" {
					usage += `,"total_tokens":` + tc.total
				}
				usage += `}`
				packet := `{"id":"chat-usage","model":"deepseek-v4-pro","created":1,"choices":[{"message":{"role":"assistant","content":"usage-control"},"delta":{"content":"usage-control"},"finish_reason":"stop"}],"usage":` + usage + `}`
				var response map[string]any
				if stream {
					wire := readDeepSeekChatSSEForTest(t, io.NopCloser(strings.NewReader("data: "+packet+"\n\ndata: [DONE]\n\n")))
					decoder := sse.NewDecoder(1 << 20)
					for _, raw := range append(decoder.Feed([]byte(wire)), decoder.Finish()...) {
						var event map[string]any
						value := json.NewDecoder(bytes.NewReader(raw))
						value.UseNumber()
						if err := value.Decode(&event); err != nil {
							t.Fatal(err)
						}
						if event["type"] == "response.completed" {
							response, _ = event["response"].(map[string]any)
						}
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
				got, _ := response["usage"].(map[string]any)
				details, _ := got["input_tokens_details"].(map[string]any)
				if got["input_tokens"] != json.Number(tc.count) || got["output_tokens"] != json.Number("1") || got["total_tokens"] != json.Number(tc.wantTotal) || details["cached_tokens"] != json.Number(tc.count) {
					t.Errorf("unsigned usage changed: %#v want input/cache=%s total=%s", got, tc.count, tc.wantTotal)
				}
			})
		}
	}
}

func TestProdex04384DeepSeekStreamCreationEpochKeepsUnsignedRange(t *testing.T) {
	for _, created := range []string{"0", "9223372036854775808", "18446744073709551615"} {
		state := newDeepSeekChatStreamState(1, nil, nil, chatcompat.ConversationStore{})
		data, _, err := state.observe([]byte(`{"created":` + created + `,"choices":[{"delta":{"content":"epoch-control"}}]}`))
		if err != nil {
			t.Fatal(err)
		}
		decoder := sse.NewDecoder(1 << 20)
		for _, raw := range append(decoder.Feed(data), decoder.Finish()...) {
			var event map[string]any
			value := json.NewDecoder(bytes.NewReader(raw))
			value.UseNumber()
			if err := value.Decode(&event); err != nil {
				t.Fatal(err)
			}
			if event["type"] == "response.created" || event["type"] == "response.output_text.delta" {
				if event["created_at"] != json.Number(created) {
					t.Errorf("stream creation epoch narrowed: got=%v want=%s", event["created_at"], created)
				}
			}
		}
	}
}
