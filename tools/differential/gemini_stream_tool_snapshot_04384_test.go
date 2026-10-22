package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestGeminiToolSnapshotRejectsMatchingCorruption04384(t *testing.T) {
	if !geminiCompactNegativeControl04384() {
		t.Fatal("tool snapshot accepted matching owner/history/seed corruption")
	}
}

func TestGeminiToolSnapshotRejectsMatchingSplitAssistant04384(t *testing.T) {
	for _, scenario := range []string{"compact-owner-tool-mixed-text", "compact-owner-tool-multiple"} {
		run := geminiCompactExpected04384(scenario)
		request := &run.Upstream[len(run.Upstream)-1]
		messages := request.Body["messages"].([]any)
		assistant := messages[2].(map[string]any)
		if scenario == "compact-owner-tool-mixed-text" {
			messages = slices.Insert(messages, len(messages)-1, any(map[string]any{"role": "assistant", "content": assistant["content"]}))
			assistant["content"] = ""
		} else {
			calls := assistant["tool_calls"].([]any)
			assistant["tool_calls"] = calls[:1]
			messages = slices.Insert(messages, 4, any(map[string]any{"role": "assistant", "content": "", "tool_calls": calls[1:]}))
		}
		request.Body["messages"] = messages
		wire, err := json.Marshal(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		request.RawBody = strings.NewReplacer(`\u003c`, "<", `\u003e`, ">", `\u0026`, "&", `\u2028`, "\u2028", `\u2029`, "\u2029").Replace(string(wire))
		if len(geminiCompare(scenario, run, run)) == 0 {
			t.Fatal("accepted matching split assistant content or selected calls")
		}
	}
}
