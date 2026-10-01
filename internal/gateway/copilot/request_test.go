package copilot

import (
	"encoding/json"
	"testing"
)

func TestCopilotCanonicalModelAliases(t *testing.T) {
	for input, want := range map[string]string{
		"": "gpt-5.3-codex", "auto": "gpt-5.3-codex", "DEFAULT": "gpt-5.3-codex",
		"codex": "gpt-5.3-codex", "gpt-5.5": "gpt-5.5", "sonnet": "claude-sonnet-4-6",
		"gemini": "gemini-3.1-pro-preview",
	} {
		if got := canonicalCopilotModel(input); got != want {
			t.Fatalf("canonical model %q = %q, want %q", input, got, want)
		}
	}
}

func TestCopilotRequestCanonicalizesAndPreservesCompaction(t *testing.T) {
	body := []byte(`{"model":"codex","messages":[{"role":"assistant","content":[{"type":"text","text":"keep","encrypted_content":"drop-a"}]}],"reasoning":{"encrypted_content":"drop-b","summary":"keep"},"input":[{"type":"compaction","encrypted_content":"keep-compact"}]}`)
	rewritten := canonicalizeCopilotRequest(body)
	var value map[string]any
	if err := json.Unmarshal(rewritten, &value); err != nil {
		t.Fatal(err)
	}
	if value["model"] != defaultRuntimeModel {
		t.Fatalf("model = %#v", value["model"])
	}
	messages := value["messages"].([]any)
	content := messages[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	if _, ok := content["encrypted_content"]; ok || content["text"] != "keep" {
		t.Fatalf("content = %#v", content)
	}
	reasoning := value["reasoning"].(map[string]any)
	if _, ok := reasoning["encrypted_content"]; ok || reasoning["summary"] != "keep" {
		t.Fatalf("reasoning = %#v", reasoning)
	}
	compaction := value["input"].([]any)[0].(map[string]any)
	if compaction["encrypted_content"] != "keep-compact" {
		t.Fatalf("compaction = %#v", compaction)
	}
}

func TestCopilotRequestInvalidJSONIsPreserved(t *testing.T) {
	body := []byte("not-json")
	if got := string(canonicalizeCopilotRequest(body)); got != string(body) {
		t.Fatalf("invalid JSON changed to %q", got)
	}
}

func TestCopilotAgentDetectionMatchesReference(t *testing.T) {
	for _, test := range []struct {
		body string
		want bool
	}{
		{`{"messages":[{"role":"ASSISTANT"}]}`, true},
		{`{"messages":[{"role":"tool"}]}`, true},
		{`{"messages":[{"role":" ASSISTANT "}]}`, false},
		{`{"messages":[{"role":"user"}]}`, false},
		{`{"input":[{"role":" \u2003ASSISTANT\u2003"}]}`, true},
		{`{"input":[{"role":""}]}`, true},
		{`{"input":[{"role":17}]}`, true},
	} {
		if got := copilotHasAgentInput([]byte(test.body)); got != test.want {
			t.Fatalf("agent(%s) = %t, want %t", test.body, got, test.want)
		}
	}
}

func TestCopilotVisionDetectionMatchesReference(t *testing.T) {
	for _, test := range []struct {
		body string
		want bool
	}{
		{`{"input":[{"type":"input_image","file_id":" file-1 "}]}`, true},
		{`{"input":[{"type":"input_image","image_url":"https://example.test/x.png"}]}`, true},
		{`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","file_id":"file-2"}]}]}`, true},
		{`{"messages":[{"role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/x.png"}}]}]}`, true},
		{`{"messages":[{"role":"assistant","content":[{"type":"image_url","image_url":{"url":"https://example.test/x.png"}}]}]}`, false},
		{`{"input":[{"type":"input_text","text":"input_image image_url"}]}`, false},
		{`{"metadata":{"type":"input_image","image_url":"https://example.test/x.png"}}`, false},
	} {
		if got := copilotHasVisionInput([]byte(test.body)); got != test.want {
			t.Fatalf("vision(%s) = %t, want %t", test.body, got, test.want)
		}
	}
}
