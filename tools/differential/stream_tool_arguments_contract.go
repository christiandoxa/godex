package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Prodex 0.438.0: streamed tool arguments are introduced as a delta, not
// as empty arguments in item.added. Both invalid Unicode/numeric/depth and
// valid paired-surrogate cases require exact frames, not a loose code match.
const toolArgumentsResponseID043713 = "chatcmpl-toolargs"
const toolArgumentsCallID043713 = "call-toolargs"
const toolArgumentsNativeError043713 = "DeepSeek streamed malformed JSON arguments for tool call `shell` at index 0: invalid JSON"

func toolArgumentScenario043713(name string) (arguments string, valid bool, known bool) {
	switch name {
	case "sse-tool-surrogate":
		return `{"cmd":"\uD800"}`, false, true
	case "sse-tool-depth":
		return strings.Repeat("[", 128) + "0" + strings.Repeat("]", 128), false, true
	case "sse-tool-overflow":
		return `{"value":1e309}`, false, true
	case "sse-tool-paired":
		return `{"cmd":"\uD83D\uDE00"}`, true, true
	default:
		return "", false, false
	}
}

func canonicalFixtureToolArgumentsSSE043713(body string) (string, string, error) {
	if !strings.HasSuffix(body, "\r\n\r\n") {
		return "", "", errors.New("stream did not terminate with CRLF SSE boundaries")
	}
	blocks := strings.Split(strings.TrimSuffix(body, "\r\n\r\n"), "\r\n\r\n")
	if len(blocks) != 4 && len(blocks) != 5 {
		return "", "", fmt.Errorf("stream event count=%d, want terminal 4/5", len(blocks))
	}
	parsed := make([]any, 0, len(blocks))
	for index, block := range blocks {
		lines := strings.Split(block, "\r\n")
		if len(lines) != 2 || !strings.HasPrefix(lines[0], "event: ") || !strings.HasPrefix(lines[1], "data: ") {
			return "", "", fmt.Errorf("frame %d is not an exact two-line SSE event", index)
		}
		event := make(map[string]any)
		if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "data: ")), &event); err != nil {
			return "", "", fmt.Errorf("frame %d JSON invalid: %w", index, err)
		}
		if event["type"] != strings.TrimPrefix(lines[0], "event: ") || event["sequence_number"] != float64(index) {
			return "", "", fmt.Errorf("frame %d event type or sequence differs", index)
		}
		parsed = append(parsed, event)
	}
	delta, ok := parsed[2].(map[string]any)["delta"].(string)
	if !ok {
		return "", "", errors.New("missing raw tool argument delta")
	}
	scenario := ""
	var valid bool
	for _, name := range []string{"sse-tool-surrogate", "sse-tool-depth", "sse-tool-overflow", "sse-tool-paired"} {
		args, success, _ := toolArgumentScenario043713(name)
		if delta == args {
			scenario, valid = name, success
			break
		}
	}
	if scenario == "" || (valid && len(parsed) != 5) || (!valid && len(parsed) != 4) {
		return "", "", errors.New("unknown tool argument scenario or terminal event count")
	}
	item := map[string]any{"type": "function_call", "call_id": toolArgumentsCallID043713, "name": "shell"}
	want := []any{
		map[string]any{"type": "response.created", "sequence_number": float64(0), "created_at": float64(1), "response": map[string]any{"id": toolArgumentsResponseID043713}},
		map[string]any{"type": "response.output_item.added", "sequence_number": float64(1), "output_index": float64(0), "item": item},
		map[string]any{"type": "response.function_call_arguments.delta", "sequence_number": float64(2), "output_index": float64(0), "call_id": toolArgumentsCallID043713, "delta": delta},
	}
	if valid {
		completeItem := map[string]any{"type": "function_call", "call_id": toolArgumentsCallID043713, "name": "shell", "arguments": delta}
		want = append(want,
			map[string]any{"type": "response.output_item.done", "sequence_number": float64(3), "output_index": float64(0), "item": completeItem},
			map[string]any{"type": "response.completed", "sequence_number": float64(4), "created_at": float64(1), "response": map[string]any{
				"id": toolArgumentsResponseID043713, "model": "deepseek-v4-pro", "output": []any{completeItem},
				"metadata": map[string]any{"deepseek": map[string]any{"finish_reason": "tool_calls"}},
			}},
		)
	} else {
		want = append(want, map[string]any{
			"type": "response.failed", "sequence_number": float64(3), "created_at": float64(1),
			"response": map[string]any{"id": toolArgumentsResponseID043713, "error": map[string]any{
				"code": "invalid_tool_call_arguments", "message": toolArgumentsNativeError043713,
			}},
		})
	}
	if !reflect.DeepEqual(parsed, want) {
		return "", "", errors.New("streamed tool events differ from the exact 0.438.0 fixture")
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return "", "", err
	}
	return string(encoded), scenario, nil
}

func validFixtureToolArgumentsSSE043713(body, scenario string) bool {
	_, name, err := canonicalFixtureToolArgumentsSSE043713(body)
	return err == nil && name == scenario
}

func equivalentFixtureToolArgumentsSSE043713(left, right string) bool {
	a, scenarioA, errA := canonicalFixtureToolArgumentsSSE043713(left)
	b, scenarioB, errB := canonicalFixtureToolArgumentsSSE043713(right)
	return errA == nil && errB == nil && scenarioA == scenarioB && a == b
}
