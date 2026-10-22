package compact

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

func TestLocalFallbackMatchesProdexCompactContract(t *testing.T) {
	body := []byte(`{"model":"test-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"retain this context"}]},{"type":"function_call","call_id":"call_1","name":"read_file","arguments":{"path":"/repo/a"}},{"type":"function_call_output","call_id":"call_1","output":"file contents"}]}`)
	response, err := LocalFallback(body, "anthropic", "local-policy")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	var value map[string]any
	if err := json.Unmarshal(content, &value); err != nil {
		t.Fatal(err)
	}
	text := value["output"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	for _, expected := range []string{SummaryPrefix, "retain this context", "tool call read_file (call_1)", "tool output call_1: file contents"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("compact text missing %q: %s", expected, text)
		}
	}
	if response.StatusCode != 200 || response.Header.Get("X-Prodex-Compact-Mode") != "local-fallback" || response.Header.Get("X-Prodex-Compact-Provider") != "anthropic" || response.Header.Get("X-Prodex-Compact-Degraded") != "true" || response.Header.Get("X-Prodex-Compact-Reason") != "local-policy" {
		t.Fatalf("compact response = status:%d headers:%v", response.StatusCode, response.Header)
	}
}

func TestLocalSummaryRetainsOnlyRecentBoundedSnippets(t *testing.T) {
	input := make([]any, 0, 30)
	for index := 0; index < 30; index++ {
		input = append(input, map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": strings.Repeat("月", 1000) + string(rune('A'+index%26))}}})
	}
	body, _ := json.Marshal(map[string]any{"model": "fixture", "input": input})
	summary := LocalSummary(body)
	if len(summary) > maxSummaryBytes || !strings.Contains(summary, "Original input items: 30") || !strings.Contains(summary, "Retained recent items: 24") {
		t.Fatalf("summary bounds/metadata = len:%d %q", len(summary), summary[:min(len(summary), 240)])
	}
}

func TestLocalSummaryMalformedAndEmptyInput(t *testing.T) {
	if got := LocalSummary([]byte("{bad")); got != "Local Prodex compact fallback could not parse the compact request body." {
		t.Fatalf("malformed summary = %q", got)
	}
	got := LocalSummary([]byte(`{"input":[]}`))
	if !strings.Contains(got, "Model: unknown") || !strings.Contains(got, "No parseable recent message or tool content was found") {
		t.Fatalf("empty summary = %q", got)
	}
}
