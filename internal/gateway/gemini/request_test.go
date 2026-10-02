package gemini

import (
	"encoding/json"
	"testing"
)

func TestGeminiResponsesRequestPreservesMetadataAndMapsTopLevelReasoning(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"model":"gemini-3.5-flash","input":"hello","reasoning_effort":"xhigh",
		"tool_choice":"required","tools":[{"type":"function","name":"lookup","parameters":{"type":"object"}}],
		"metadata":{"request_id":"r1","gemini":{"trace":"g1"}},
		"client_metadata":{"client":"codex"},"prompt_cache_key":" cache ","prompt_cache_retention":"24h"
	}`), "gemini-3.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(translated.body, &chat); err != nil {
		t.Fatal(err)
	}
	if chat["reasoning_effort"] != "high" {
		t.Fatalf("reasoning_effort = %#v", chat["reasoning_effort"])
	}
	if _, exists := chat["tool_choice"]; exists {
		t.Fatalf("tool_choice was not omitted for thinking mode: %#v", chat)
	}
	metadata := translated.metadata
	if metadata["request_id"] != "r1" || metadata["client_metadata"].(map[string]any)["client"] != "codex" {
		t.Fatalf("response metadata = %#v", metadata)
	}
	gemini := metadata["gemini"].(map[string]any)
	if gemini["trace"] != "g1" || gemini["prompt_cache_key"] != " cache " || gemini["prompt_cache_retention"] != "24h" {
		t.Fatalf("Gemini metadata = %#v", gemini)
	}
	if gemini["omitted_tool_choice"].(map[string]any)["from"] != "required" {
		t.Fatalf("omitted choice metadata = %#v", gemini["omitted_tool_choice"])
	}
}

func TestGeminiResponsesRequestTranslatesNestedToolsAndFormats(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"input":"hello","tools":[
			{"type":"namespace","name":"files","tools":[{"type":"function","name":"read","parameters":{"type":"object"}}]},
			{"type":"custom","name":"shell"},
			{"type":"mcp_toolset","server_label":"fs","allowed_tools":["read_file","list_files"]},
			{"type":"web_search_preview","search_context_size":"high"}
		],"tool_choice":{"type":"function","name":"files__read"},
		"response_format":{"type":"structured_output","name":"answer","schema":{"type":"object"}}
	}`), "gemini-3.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(translated.body, &chat); err != nil {
		t.Fatal(err)
	}
	tools := chat["tools"].([]any)
	if len(tools) != 4 || tools[0].(map[string]any)["function"].(map[string]any)["name"] != "files__read" ||
		tools[1].(map[string]any)["function"].(map[string]any)["name"] != "shell" ||
		tools[2].(map[string]any)["function"].(map[string]any)["name"] != "mcp__fs__list_files" ||
		tools[3].(map[string]any)["function"].(map[string]any)["name"] != "mcp__fs__read_file" {
		t.Fatalf("translated tools = %#v", tools)
	}
	if chat["tool_choice"].(map[string]any)["function"].(map[string]any)["name"] != "files__read" ||
		chat["web_search_options"].(map[string]any)["search_context_size"] != "high" ||
		chat["response_format"].(map[string]any)["type"] != "json_object" {
		t.Fatalf("translated controls = %#v", chat)
	}
	if translated.metadata["gemini"].(map[string]any)["degraded_response_format"].(map[string]any)["from"] != "structured_output" {
		t.Fatalf("response metadata = %#v", translated.metadata)
	}
}

func TestGeminiResponsesRequestRejectsDuplicateAndInvalidMetadata(t *testing.T) {
	for _, body := range []string{
		`{"input":"hello","tools":[{"type":"function","name":"x"},{"type":"function","name":"x"}]}`,
		`{"input":"hello","metadata":[]}`,
		`{"input":"hello","client_metadata":[]}`,
		`{"input":"hello","prompt_cache_key":1}`,
	} {
		if _, err := translateResponsesRequest([]byte(body), "gemini-3.5-flash"); err == nil {
			t.Errorf("translateResponsesRequest(%s) succeeded, want validation error", body)
		}
	}
}

func TestGeminiResponsesRequestMapsMCPNamedToolChoice(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"input":"hello","tools":[{"type":"mcp_toolset","server_label":"files","allowed_tools":["read"]}],
		"tool_choice":{"type":"mcp","server_label":"files","name":"read"}
	}`), "gemini-3.5-flash")
	if err != nil {
		t.Fatal(err)
	}
	var chat map[string]any
	if err := json.Unmarshal(translated.body, &chat); err != nil {
		t.Fatal(err)
	}
	if chat["tool_choice"].(map[string]any)["function"].(map[string]any)["name"] != "mcp__files__read" {
		t.Fatalf("tool_choice = %#v", chat["tool_choice"])
	}
}
