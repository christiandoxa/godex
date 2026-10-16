package gemini

import (
	"encoding/json"
	"testing"
)

// Tagged Prodex 0.438.0's request-content Mojo classifier treats U+001C as
// whitespace. A present blank signature is a barrier, so a lower-precedence
// signature must not be copied into native functionCall content.
func TestProdex04380GeminiNativeRejectsRecordSeparatorThoughtSignature(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"model":"gemini-2.5-pro","input":[
			{"type":"function_call","call_id":"call-c0","name":"lookup","arguments":"{}","gemini_thought_signature":"\u001c","thought_signature":"fallback"}
		]
	}`), "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if err := json.Unmarshal(translated.body, &native); err != nil {
		t.Fatal(err)
	}
	contents := native["contents"].([]any)
	parts := contents[1].(map[string]any)["parts"].([]any)
	call := parts[0].(map[string]any)["functionCall"].(map[string]any)
	if _, present := call["thoughtSignature"]; present {
		t.Fatalf("blank U+001C thought signature crossed native boundary: %+v", call)
	}
}
