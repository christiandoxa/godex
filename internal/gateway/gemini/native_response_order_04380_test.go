package gemini

import (
	"reflect"
	"testing"
)

// Source: tagged Prodex 0.438.0
// crates/prodex-provider-core/src/gemini_bridge/hardening/contents/
// tool_pairs/order.rs and Mojo gemini_tool_response_order_plan.
// It groups tool responses before ordinary parts, ranks responses by the
// first matching preceding call ID (NOT name), and preserves stable order.
func TestProdex04380GeminiNativeResponseOrderMatchesTaggedSorter(t *testing.T) {
	model := map[string]any{"role": "model", "parts": []any{
		map[string]any{"functionCall": map[string]any{"id": "call-a", "name": "tool"}},
		map[string]any{"functionCall": map[string]any{"id": "call-b", "name": "tool"}},
		map[string]any{"functionCall": map[string]any{"id": "call-a", "name": "tool"}},
	}}
	textBefore := map[string]any{"text": "before"}
	image := map[string]any{"inlineData": map[string]any{"mimeType": "image/png", "data": "fixture"}}
	textAfter := map[string]any{"text": "after"}
	response := func(id string, value int) any {
		return map[string]any{"functionResponse": map[string]any{"id": id, "response": map[string]any{"item": value}}}
	}
	noID := map[string]any{"functionResponse": map[string]any{"response": map[string]any{"item": 6}}}
	user := map[string]any{"role": "user", "parts": []any{
		textBefore, response("missing-a", 1), response("call-b", 2), image, response("call-a", 3),
		response("call-a", 4), response("missing-b", 5), noID, textAfter,
	}, "metadata": 42}
	original := []any{model, user}
	got := geminiRefineToolResponseOrder04380(original)
	expected := []any{model, map[string]any{"role": "user", "parts": []any{
		response("call-a", 3), response("call-a", 4), response("call-b", 2),
		response("missing-a", 1), response("missing-b", 5), noID, textBefore, image, textAfter,
	}, "metadata": 42}}
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("tagged tool response sort output differs: got=%+v", got)
	}
	if len(user["parts"].([]any)) != 9 || !reflect.DeepEqual(user["parts"].([]any)[0], textBefore) {
		t.Fatal("reordering modified caller-owned original parts")
	}
	if again := geminiRefineToolResponseOrder04380(got); !reflect.DeepEqual(again, got) {
		t.Fatalf("tagged sorting changed again on repeated invocation: %+v", again)
	}
}

func TestProdex04380GeminiNativeResponseOrderNullIdentityAndNoCalls(t *testing.T) {
	source := []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": nil}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "keep"}, map[string]any{"functionResponse": nil}}, "metadata": 42},
		map[string]any{"role": "custom", "parts": []any{map[string]any{"functionResponse": nil}, map[string]any{"text": "custom"}}},
	}
	expected := []any{
		source[0], map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": nil}, map[string]any{"text": "keep"}}, "metadata": 42}, source[2],
	}
	if got := geminiRefineToolResponseOrder04380(source); !reflect.DeepEqual(got, expected) {
		t.Fatalf("null call id and ineligible content ordering differ: %+v", got)
	}
	noCalls := []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "no calls"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "before"}, map[string]any{"functionResponse": map[string]any{"id": "call-a"}}, map[string]any{"text": "after"}}},
	}
	if got := geminiRefineToolResponseOrder04380(noCalls); !reflect.DeepEqual(got, noCalls) {
		t.Fatalf("rows without preceding tool-call IDs must be unchanged: %+v", got)
	}
}

// The tagged stable ordering ranks only by call ID, not by a visible tool
// name. Duplicates keep arrival order even if the names disagree.
func TestProdex04380GeminiNativeResponseOrderDuplicateIDsStayStable(t *testing.T) {
	contents := []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": "id", "name": "first"}}}},
		map[string]any{"role": "user", "parts": []any{
			map[string]any{"functionResponse": map[string]any{"id": "id", "name": "other", "response": 1}},
			map[string]any{"functionResponse": map[string]any{"id": "id", "name": "first", "response": 2}},
			map[string]any{"text": "last"},
		}},
	}
	if got := geminiRefineToolResponseOrder04380(contents); !reflect.DeepEqual(got, contents) {
		t.Fatalf("equal ID/rank responses were reordered by name or metadata: %+v", got)
	}
}
