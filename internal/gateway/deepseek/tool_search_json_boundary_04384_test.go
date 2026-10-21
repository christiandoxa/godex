package deepseek

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04384DeepSeekToolSearchArgumentsRejectInvalidSerdeValues(t *testing.T) {
	for _, arguments := range []string{
		"", "{bad", `{} {}`, `{"unused":"\ud800"}`, `1e309`, `1.7976931348623158e308`,
		`{"text":"` + string([]byte{0xff}) + `"}`,
		strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128),
	} {
		if _, err := deepSeekToolSearchArguments(arguments); err == nil {
			t.Fatalf("invalid structured tool arguments accepted: %q", arguments)
		}
		if _, err := deepSeekResponseToolItem(map[string]any{"function": map[string]any{"name": "tool_search", "arguments": arguments}}); err == nil {
			t.Fatalf("buffered tool item ignored invalid arguments: %q", arguments)
		}
	}
	call := &deepSeekChatStreamToolCall{id: "call-search", name: "tool_search"}
	item := deepSeekStreamCompletedToolItem(call)
	encoded, err := json.Marshal(item["arguments"])
	if err != nil || string(encoded) != "{}" {
		t.Fatalf("stream's empty-argument fallback changed: output=%s error=%v", encoded, err)
	}
}
