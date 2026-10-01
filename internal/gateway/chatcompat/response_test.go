package chatcompat

import (
	"encoding/json"
	"testing"
	"time"
)

func TestChatResponseMapsTextToolsUsageAndRTK(t *testing.T) {
	body := []byte(`{"id":"chatcmpl_test","model":"compat-model","created":1700000000,"choices":[{"message":{"role":"assistant","content":[{"text":"hello"},{"content":"world"},{"text":""}],"tool_calls":[{"id":"call_test","function":{"name":"functions.exec_command","arguments":"{\"cmd\":\"ls\"}"}}]}}],"usage":{"prompt_tokens":3,"completion_tokens":4}}`)
	translated, err := ChatResponse(body, time.Unix(1700000123, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "chatcmpl_test" || got["model"] != "compat-model" || got["created_at"] != float64(1700000000) {
		t.Fatalf("response = %#v", got)
	}
	output := got["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("output = %#v", output)
	}
	tool := output[1].(map[string]any)
	if tool["namespace"] != "functions" || tool["name"] != "exec_command" || tool["arguments"] != `{"cmd":"rtk ls"}` {
		t.Fatalf("tool = %#v", tool)
	}
	usage := got["usage"].(map[string]any)
	if usage["input_tokens"] != float64(3) || usage["output_tokens"] != float64(4) || usage["total_tokens"] != float64(7) {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestChatResponseSparseDefaults(t *testing.T) {
	translated, err := ChatResponse([]byte(`{"choices":[]}`), time.Unix(1700000123, 0))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(translated, &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "resp_prodex" || got["model"] != "unknown" || got["created_at"] != float64(1700000123) || len(got["output"].([]any)) != 0 {
		t.Fatalf("response = %#v", got)
	}
}

func TestRTKCommandWrappingMatchesNoisySegments(t *testing.T) {
	for input, want := range map[string]string{
		"ls":                         "rtk ls",
		"pwd":                        "pwd",
		"cd /repo && cargo check -q": "cd /repo && rtk cargo check -q",
		"rtk ls":                     "rtk ls",
	} {
		got, _ := wrapRTKCommand(input)
		if got != want {
			t.Fatalf("wrap %q = %q, want %q", input, got, want)
		}
	}
}
