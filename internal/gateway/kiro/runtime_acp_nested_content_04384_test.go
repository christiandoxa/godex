package kiro

import (
	"encoding/json"
	"strings"
	"testing"
)

// Source 0.438.4 kiro_json_write_content_value descends into a map's
// `content` member whenever a direct `text` string is absent. Arrays join
// all nested strings without injecting separators. The previous Go walker
// skipped nested content maps and silently lost legitimate model output.
func TestProdex04384KiroACPRecursiveContentMatchesCompiledMojo(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{"direct-string", `"hello"`, "hello"},
		{"direct-text-map", `{"text":"hello"}`, "hello"},
		{"nested-content-list", `{"content":[{"text":"hello"},{"text":" 世界"}]}`, "hello 世界"},
		{"nested-content-map", `{"content":{"content":{"text":"deep"}}}`, "deep"},
		{"array-of-nested-objects", `[{"text":"one "},{"content":[{"text":"two"}]}," three"]`, "one two three"},
		{"text-takes-priority", `{"content":[{"text":"ignored"}],"text":"priority"}`, "priority"},
		{"wrong-text-falls-back", `{"content":{"text":"fallback"},"text":123}`, "fallback"},
		{"ignore-other-fields", `{"other":"invisible"}`, ""},
		{"empty-nested", `{"content":[]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input any
			if err := json.Unmarshal([]byte(tc.input), &input); err != nil {
				t.Fatal(err)
			}
			if got := acpContentText(input); got != tc.want {
				t.Fatalf("Mojo Kiro recursive content=%q => %q want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestProdex04384KiroNestedACPContentPreservesBufferedAndLiveText(t *testing.T) {
	params := `{"sessionId":"s","update":{"sessionUpdate":"agent_message_chunk","content":{"content":[{"text":"hello "},{"content":[{"text":"世界"}]}]}}}`
	envelope := kiroUpdateEnvelope04384(params)
	turn := collectKiroTurnState([]acpEnvelope{envelope})
	if turn.assistantText != "hello 世界" {
		t.Fatalf("nested Kiro ACP buffered content lost: %q", turn.assistantText)
	}
	live := newKiroLiveState(10, "synthetic", "test")
	frames := live.observe(runtimeRoute{kind: routeChat}, envelope)
	if len(frames) != 1 || live.turn.assistantText != "hello 世界" || !strings.Contains(string(frames[0]), "hello 世界") {
		t.Fatalf("nested Kiro ACP live stream lost content: state=%q frames=%q", live.turn.assistantText, frames)
	}
}
