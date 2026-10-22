package deepseek

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/christiandoxa/godex/internal/gateway/chatcompat"
)

func TestProdex04384DeepSeekHistoryUsesOriginalToolOutputID(t *testing.T) {
	for _, fixture := range []struct {
		name, storedID, outputID, field string
		cached                          bool
	}{
		{"exact", "call_synthetic", "call_synthetic", "call_id", true},
		{"padded", " call_synthetic ", " call_synthetic ", "call_id", true},
		{"Unicode alias", "\tcall_synthetic\u2003", "\tcall_synthetic\u2003", "tool_call_id", true},
		{"ID alias", " call_synthetic ", " call_synthetic ", "id", true},
		{"padded output mismatch", "call_synthetic", " call_synthetic ", "call_id", false},
		{"padded stored mismatch", " call_synthetic ", "call_synthetic", "call_id", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			store := chatcompat.NewConversationStore()
			history := []any{map[string]any{"role": "user", "content": "original context"}, assistantToolCall(fixture.storedID, "lookup", "{}")}
			store.Insert("resp_synthetic", history)
			body, err := json.Marshal(map[string]any{"input": []any{map[string]any{"type": "function_call_output", fixture.field: fixture.outputID, "output": "done"}}})
			if err != nil {
				t.Fatal(err)
			}
			got := deepSeekConversationHistoryForRequest(body, store)
			var want []any
			if fixture.cached {
				want = history
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("tool-output lookup changed opaque ID or selected a different history owner")
			}
		})
	}
}
