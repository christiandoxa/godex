package deepseek

import (
	"encoding/json"
	"testing"
)

func TestProdex04384DeepSeekBufferedChatHistoryProjection(t *testing.T) {
	for _, fixture := range []struct{ name, input, want string }{
		{"projected fields", `{"choices":[{"message":{"role":"user","content":"reply","provider_decoy":{"value":"must-not-replay"}}}]}`, `[{"content":"reply","role":"assistant"}]`},
		{"empty message", `{"choices":[{"message":{"content":""}}]}`, `null`},
		{"reasoning with unsupported content", `{"choices":[{"message":{"content":[{"text":"ignored"}],"reasoning_content":"reason"}}]}`, `[{"content":null,"reasoning_content":"reason","role":"assistant"}]`},
		{"shell wrapping", `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-synthetic","type":"function","function":{"name":"shell","arguments":"{\"cmd\":\"ls\"}"}}]}}]}`, `[{"content":"","role":"assistant","tool_calls":[{"function":{"arguments":"{\"cmd\":\"rtk ls\"}","name":"shell"},"id":"call-synthetic","type":"function"}]}]`},
		{"number snapshot", `{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-synthetic","priority":9007199254740993}]}}]}`, `[{"content":"","role":"assistant","tool_calls":[{"id":"call-synthetic","priority":9007199254740993}]}]`},
		{"missing choices", `{}`, `null`},
		{"trailing value", `{"choices":[{"message":{"content":"invalid"}}]} {}`, `null`},
		{"wrong root", `[]`, `null`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			messages := deepSeekChatAssistantMessages([]byte(fixture.input))
			body, err := json.Marshal(messages)
			if err != nil || string(body) != fixture.want {
				t.Fatalf("history=%s error=%v, want %s", body, err, fixture.want)
			}
		})
	}
}
