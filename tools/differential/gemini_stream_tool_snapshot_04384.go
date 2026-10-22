package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

func geminiToolSnapshotCases04384() []string {
	return append([]string{"compact-owner-tool-simple", "compact-owner-tool-shell", "compact-owner-tool-finish-eof", "compact-owner-tool-done-only", "compact-owner-tool-unfinished", "compact-owner-tool-invalid"}, geminiRepairedSnapshotCases04384()...)
}

func geminiToolSnapshotCase04384(scenario string) bool {
	return slices.Contains(geminiToolSnapshotCases04384(), scenario)
}

func geminiToolSnapshotCached04384(scenario string) bool {
	return scenario != "compact-owner-tool-unfinished" && scenario != "compact-owner-tool-invalid" && scenario != "compact-owner-tool-error-valid-done"
}

func geminiToolSnapshotLifecycle04384(scenario string) string {
	switch scenario {
	case "compact-owner-tool-shell":
		return "stream-chat-tool-shell"
	case "compact-owner-tool-invalid":
		return "stream-chat-tool-invalid-arguments"
	default:
		return "stream-chat-tool-simple"
	}
}

func writeGeminiToolSnapshot04384(writer io.Writer, scenario string) {
	if geminiRepairedSnapshotCase04384(scenario) {
		writeGeminiRepairedSnapshot04384(writer, scenario)
		return
	}
	var wire bytes.Buffer
	writeGeminiToolLifecycle04384(&wire, geminiToolSnapshotLifecycle04384(scenario))
	blocks := strings.Split(strings.TrimSuffix(wire.String(), "\n\n"), "\n\n")
	for _, block := range blocks {
		if strings.Contains(block, `"finish_reason"`) && (scenario == "compact-owner-tool-done-only" || scenario == "compact-owner-tool-unfinished") {
			continue
		}
		if block == "data: [DONE]" && (scenario == "compact-owner-tool-finish-eof" || scenario == "compact-owner-tool-unfinished") {
			continue
		}
		_, _ = io.WriteString(writer, block+"\n\n")
	}
}

func geminiToolSnapshotSeedWire04384(scenario, responseID string) string {
	if geminiRepairedSnapshotCase04384(scenario) {
		return geminiRepairedSnapshotSeedWire04384(scenario, responseID)
	}
	wire := geminiLifecycleWire04384(geminiToolSnapshotLifecycle04384(scenario), responseID, "")
	if scenario == "compact-owner-tool-done-only" {
		return strings.Replace(wire, `"metadata":{"gemini":{"finish_reason":"tool_calls"}},`, "", 1)
	}
	if scenario != "compact-owner-tool-finish-eof" && scenario != "compact-owner-tool-unfinished" {
		return wire
	}
	parts := strings.Split(wire, "\r\n\r\n")
	keep := 4
	if scenario == "compact-owner-tool-unfinished" {
		keep = 3
	}
	failed := fmt.Sprintf("event: response.failed\r\ndata: {\"created_at\":1,\"response\":{\"error\":{\"code\":\"provider_stream_error\",\"message\":\"unexpected end of stream\"},\"id\":%q},\"sequence_number\":%d,\"type\":\"response.failed\"}\r\n\r\n", responseID, keep)
	return strings.Join(parts[:keep], "\r\n\r\n") + "\r\n\r\n" + failed
}

func geminiToolSnapshotExpected04384(scenario string) geminiCandidateResult {
	run := geminiCompactExpected04384("compact-empty")
	request := &run.Upstream[0]
	if geminiToolSnapshotCached04384(scenario) {
		name, arguments := "lookup", `{}`
		if scenario == "compact-owner-tool-shell" {
			name, arguments = "shell", `{"cmd":"rtk git status"}`
		}
		history := []any{
			map[string]any{"role": "user", "content": "same request"},
			map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "call_synthetic", "type": "function", "function": map[string]any{"name": name, "arguments": arguments}}}},
			map[string]any{"role": "tool", "tool_call_id": "call_synthetic", "content": "done"},
		}
		messages := request.Body["messages"].([]any)
		request.Body["messages"] = append(append([]any{messages[0]}, history...), messages[1])
		encoded, _ := json.Marshal(request.Body)
		request.RawBody = string(encoded)
		_ = json.Unmarshal(encoded, &request.Body)
	}
	seed := geminiReasoningStreamExpected04384("stream-chat-reasoning-empty")
	seed.Client.Body = geminiToolSnapshotSeedWire04384(scenario, "resp_deepseek_fixture")
	run.Client.Seed = &seed.Client
	run.Upstream = append(seed.Upstream, run.Upstream...)
	return run
}
