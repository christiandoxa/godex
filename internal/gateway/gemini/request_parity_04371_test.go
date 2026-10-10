package gemini

import (
	"encoding/json"
	"reflect"
	"testing"
)

func decodeGeminiRequest(t *testing.T, body string, model string) map[string]any {
	t.Helper()
	translated, err := translateResponsesRequest([]byte(body), model)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(translated.body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGemini04371RequestThinkingDefaults(t *testing.T) {
	tests := []struct {
		name   string
		model  string
		effort string
		want   map[string]any
	}{
		{"gemini default", "gemini-2.5-pro", "", map[string]any{"includeThoughts": true, "thinkingBudget": float64(8192)}},
		{"gemini low", "gemini-2.5-pro", "low", map[string]any{"includeThoughts": true, "thinkingBudget": float64(1024)}},
		{"gemini xhigh", "gemini-2.5-pro", "xhigh", map[string]any{"includeThoughts": true, "thinkingBudget": float64(24576)}},
		{"gemini disabled", "gemini-2.5-pro", "none", map[string]any{"includeThoughts": false, "thinkingBudget": float64(0)}},
		{"gemini 3", "gemini-3.5-flash", "medium", map[string]any{"includeThoughts": true, "thinkingLevel": "MEDIUM"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := `{"input":"hello"}`
			if test.effort != "" {
				body = `{"input":"hello","reasoning":{"effort":"` + test.effort + `"}}`
			}
			value := decodeGeminiRequest(t, body, test.model)
			generation := value["generationConfig"].(map[string]any)
			if !reflect.DeepEqual(generation["thinkingConfig"], test.want) {
				t.Fatalf("thinkingConfig = %#v, want %#v", generation["thinkingConfig"], test.want)
			}
		})
	}
}

func TestGemini04371RequestPreservesHistoryAndMedia(t *testing.T) {
	value := decodeGeminiRequest(t, `{
		"input":[
			{"role":"user","content":[{"type":"input_text","text":"Describe"},{"type":"input_image","image_url":"data:image/png;base64,AAAA"}]},
			{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"lookup","arguments":"{\"q\":\"x\"}"}}]},
			{"role":"tool","tool_call_id":"call_1","content":"{\"ok\":true}"},
			{"role":"user","content":"continue"}
		]
	}`, "gemini-2.5-pro")
	contents := value["contents"].([]any)
	if len(contents) != 4 {
		t.Fatalf("contents = %#v", contents)
	}
	parts := contents[0].(map[string]any)["parts"].([]any)
	if parts[1].(map[string]any)["inlineData"].(map[string]any)["mimeType"] != "image/png" {
		t.Fatalf("media part = %#v", parts[1])
	}
	call := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
	if call["name"] != "lookup" || call["id"] != "call_1" {
		t.Fatalf("function call = %#v", call)
	}
	response := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if response["name"] != "lookup" || response["id"] != "call_1" {
		t.Fatalf("function response = %#v", response)
	}
}

func TestGemini04371RequestSanitizesSchemasAndMapsBuiltins(t *testing.T) {
	value := decodeGeminiRequest(t, `{
		"input":"use tools",
		"tools":[
			{"type":"function","function":{"name":"search","parameters":{"type":"object","strict":true,"$schema":"x","additionalProperties":false,"properties":{"q":{"type":"string","additionalProperties":false}}}}},
			{"type":"web_search_preview"},{"type":"code_execution"},{"type":"url_context"},
			{"type":"computer_use","computer_use":{"environment":"ENVIRONMENT_DESKTOP","excluded_predefined_functions":["move"]}}
		]
	}`, "gemini-2.5-pro")
	tools := value["tools"].([]any)
	if len(tools) != 5 {
		t.Fatalf("tools = %#v", tools)
	}
	declaration := tools[4].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
	schema := declaration["parameters"].(map[string]any)
	if _, ok := schema["strict"]; ok {
		t.Fatalf("strict schema field survived: %#v", schema)
	}
	if _, ok := schema["additionalProperties"]; ok {
		t.Fatalf("additionalProperties survived: %#v", schema)
	}
	builtin := tools[0].(map[string]any)["computerUse"].(map[string]any)
	if builtin["environment"] != "ENVIRONMENT_DESKTOP" {
		t.Fatalf("computer tool = %#v", builtin)
	}
}

func TestGemini04371RequestIgnoresPreviousResponseID(t *testing.T) {
	value := decodeGeminiRequest(t, `{"input":"continue","previous_response_id":"resp_1"}`, "gemini-2.5-pro")
	if _, ok := value["previous_response_id"]; ok {
		t.Fatalf("continuation field leaked into native request: %#v", value)
	}
}
