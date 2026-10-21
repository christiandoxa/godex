package deepseek

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/christiandoxa/godex/internal/helper/sse"
)

func TestProdex04384DeepSeekEmbeddedErrorPreservesPresenceAndIntegerType(t *testing.T) {
	const fallback = "provider_stream_error"
	const message = "Provider stream returned an embedded error"
	for _, tc := range []struct{ name, errorJSON, code, message string }{
		{"null", `null`, fallback, message},
		{"object", `{}`, fallback, message},
		{"array", `[]`, fallback, message},
		{"string", `"boom"`, fallback, "boom"},
		{"empty-string", `""`, fallback, ""},
		{"primary-null", `{"type":null,"code":"SECONDARY"}`, fallback, message},
		{"primary-false", `{"type":false,"code":"SECONDARY"}`, fallback, message},
		{"primary-empty", `{"type":"","code":"SECONDARY"}`, "", message},
		{"primary-string", `{"type":" E ","code":"SECONDARY"}`, " E ", message},
		{"secondary-string", `{"code":"SECONDARY"}`, "SECONDARY", message},
		{"integer", `{"code":-7}`, "-7", message},
		{"signed-max", `{"code":9223372036854775807}`, "9223372036854775807", message},
		{"signed-min", `{"code":-9223372036854775808}`, "-9223372036854775808", message},
		{"unsigned-overflow", `{"code":9223372036854775808}`, fallback, message},
		{"integer-float", `{"code":1.0}`, fallback, message},
		{"exponent", `{"code":1e0}`, fallback, message},
		{"negative-zero", `{"code":-0}`, fallback, message},
		{"message-null", `{"message":null,"detail":"SECONDARY"}`, fallback, message},
		{"message-empty", `{"message":"","detail":"SECONDARY"}`, fallback, ""},
		{"message-wrong-type", `{"message":7,"detail":"SECONDARY"}`, fallback, message},
		{"secondary-message", `{"detail":"详情 🚀"}`, fallback, "详情 🚀"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newDeepSeekChatStreamState(1, nil, nil, deepSeekConversationStore{})
			ownedID := state.responseID
			wire := `{"id":"must-not-own","model":"must-not-adopt","error":` + tc.errorJSON + `,"Error":{"type":"WRONG_CASE","message":"must-not-emit"},"choices":[{"delta":{"content":"must-not-emit"}}]}`
			output, supported, err := state.observe([]byte(wire))
			if err != nil || !supported || !state.completed || state.created {
				t.Fatalf("embedded error did not fail before creation: supported=%v completed=%v created=%v err=%v", supported, state.completed, state.created, err)
			}
			decoder := sse.NewDecoder(1 << 20)
			frames := append(decoder.Feed(output), decoder.Finish()...)
			if len(frames) != 1 {
				t.Fatalf("error emitted nonterminal or duplicate events: %s", output)
			}
			var event map[string]any
			if err := json.Unmarshal(frames[0], &event); err != nil {
				t.Fatal(err)
			}
			response := event["response"].(map[string]any)
			failure := response["error"].(map[string]any)
			if event["type"] != "response.failed" || event["sequence_number"] != float64(0) || response["id"] != ownedID || failure["code"] != tc.code || failure["message"] != tc.message || strings.Contains(string(output), "must-not-") {
				t.Errorf("error presence/type changed: event=%#v want code=%q message=%q", event, tc.code, tc.message)
			}
			if later, supported, err := state.observe([]byte("[DONE]")); err != nil || supported || len(later) != 0 {
				t.Errorf("error accepted a later completion: %s supported=%v err=%v", later, supported, err)
			}
		})
	}
}
