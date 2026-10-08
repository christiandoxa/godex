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
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	generation := native["generationConfig"].(map[string]any)
	thinking := generation["thinkingConfig"].(map[string]any)
	if thinking["thinkingLevel"] != "HIGH" || thinking["includeThoughts"] != true {
		t.Fatalf("thinking config = %#v", thinking)
	}
	if _, exists := native["toolConfig"]; exists {
		t.Fatalf("toolConfig was not omitted for thinking mode: %#v", native)
	}
	metadata := translated.metadata
	if metadata["request_id"] != "r1" || metadata["client_metadata"].(map[string]any)["client"] != "codex" {
		t.Fatalf("response metadata = %#v", metadata)
	}
	provider := metadata["gemini"].(map[string]any)
	if provider["trace"] != "g1" || provider["prompt_cache_key"] != " cache " || provider["prompt_cache_retention"] != "24h" {
		t.Fatalf("Gemini metadata = %#v", provider)
	}
	if provider["omitted_tool_choice"].(map[string]any)["from"] != "required" {
		t.Fatalf("omitted choice metadata = %#v", provider["omitted_tool_choice"])
	}
}

func TestGeminiResponsesRequestHardensFirstUnsignedGemini3ToolCall(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"model":"gemini-3.1-pro-preview","input":[
			{"type":"function_call","call_id":"unsigned","name":"shell","arguments":"{}"},
			{"type":"function_call","call_id":"signed","name":"shell","arguments":"{}","gemini_thought_signature":"sig"},
			{"type":"function_call_output","call_id":"unsigned","output":"ok"}
		]
	}`), "gemini-3.1-pro-preview")
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	contents := native["contents"].([]any)
	first := contents[0].(map[string]any)["parts"].([]any)[0].(map[string]any)
	second := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if first["thoughtSignature"] != "skip_thought_signature_validator" {
		t.Fatalf("unsigned Gemini 3 tool call = %#v", first)
	}
	call := second["functionCall"].(map[string]any)
	if call["thoughtSignature"] != "sig" {
		t.Fatalf("signed Gemini 3 tool call = %#v", second)
	}
	if _, exists := second["thoughtSignature"]; exists {
		t.Fatalf("nested signature was duplicated on Gemini part = %#v", second)
	}
}

func TestGeminiResponsesRequestDoesNotHardenGemini25ToolCalls(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"model":"gemini-2.5-pro","input":[
			{"type":"function_call","call_id":"unsigned","name":"shell","arguments":"{}"}
		]
	}`), "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	part := native["contents"].([]any)[0].(map[string]any)["parts"].([]any)[0].(map[string]any)
	if _, exists := part["thoughtSignature"]; exists {
		t.Fatalf("Gemini 2.5 tool call was hardened = %#v", part)
	}
}

func TestGeminiToolCallThoughtSignatureHardeningStopsAfterFirstCall(t *testing.T) {
	body := map[string]any{"contents": []any{map[string]any{
		"role": "model",
		"parts": []any{
			map[string]any{"functionCall": map[string]any{"name": "first"}},
			map[string]any{"functionCall": map[string]any{"name": "second"}},
		},
	}}}
	if injected := hardenGeminiToolCallThoughtSignatures(body, "gemini-3.1-pro-preview"); injected != 1 {
		t.Fatalf("injected thought signatures = %d, want 1", injected)
	}
	parts := body["contents"].([]any)[0].(map[string]any)["parts"].([]any)
	if parts[0].(map[string]any)["thoughtSignature"] != "skip_thought_signature_validator" {
		t.Fatalf("first tool call = %#v", parts[0])
	}
	if _, exists := parts[1].(map[string]any)["thoughtSignature"]; exists {
		t.Fatalf("second tool call was hardened = %#v", parts[1])
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
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	tools := native["tools"].([]any)
	declarations := tools[0].(map[string]any)["functionDeclarations"].([]any)
	if len(declarations) != 4 ||
		declarations[0].(map[string]any)["name"] != "files__read" ||
		declarations[1].(map[string]any)["name"] != "shell" ||
		declarations[2].(map[string]any)["name"] != "mcp__fs__list_files" ||
		declarations[3].(map[string]any)["name"] != "mcp__fs__read_file" {
		t.Fatalf("translated declarations = %#v", declarations)
	}
	if _, ok := tools[1].(map[string]any)["googleSearch"]; !ok {
		t.Fatalf("Gemini web-search tool = %#v", tools)
	}
	choice := native["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
	if choice["mode"] != "ANY" || choice["allowedFunctionNames"].([]any)[0] != "files__read" {
		t.Fatalf("translated toolConfig = %#v", native["toolConfig"])
	}
	generation := native["generationConfig"].(map[string]any)
	if generation["responseMimeType"] != "application/json" ||
		generation["responseJsonSchema"].(map[string]any)["type"] != "object" {
		t.Fatalf("translated generation config = %#v", generation)
	}
	if provider, _ := translated.metadata["gemini"].(map[string]any); provider["degraded_response_format"] != nil {
		t.Fatalf("native structured output was incorrectly marked degraded: %#v", translated.metadata)
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
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	choice := native["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
	if choice["allowedFunctionNames"].([]any)[0] != "mcp__files__read" {
		t.Fatalf("toolConfig = %#v", native["toolConfig"])
	}
}
