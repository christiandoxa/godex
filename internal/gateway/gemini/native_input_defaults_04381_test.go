package gemini

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestProdex04381GeminiNativeDefaultsNullRoleToUser(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{
		"input":[{"role":null,"content":[{"type":"input_text","text":"keep this turn"}]}]
	}`), "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(translated.body, &body); err != nil {
		t.Fatal(err)
	}
	want := []any{map[string]any{
		"role":  "user",
		"parts": []any{map[string]any{"text": "keep this turn"}},
	}}
	if !reflect.DeepEqual(body["contents"], want) {
		t.Fatalf("null-role native content was dropped: got=%#v want=%#v", body["contents"], want)
	}
}

func TestProdex04381GeminiNativeEmptyInputKeepsUserTurn(t *testing.T) {
	translated, err := translateResponsesRequest([]byte(`{"input":[]}`), "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(translated.body, &body); err != nil {
		t.Fatal(err)
	}
	want := []any{map[string]any{
		"role":  "user",
		"parts": []any{map[string]any{"text": ""}},
	}}
	if !reflect.DeepEqual(body["contents"], want) {
		t.Fatalf("empty native input lost the required user turn: got=%#v want=%#v", body["contents"], want)
	}
}
