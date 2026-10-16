package gemini

import (
	"reflect"
	"testing"
)

const missingResultText04380 = "The tool execution result was missing from the Codex conversation history. Treat this tool result as unavailable and recover by inspecting current workspace state before continuing."

func nativePairCall04380(id, name string) any {
	return map[string]any{"functionCall": map[string]any{"id": id, "name": name, "args": map[string]any{}}, "thoughtSignature": "skip_thought_signature_validator"}
}
func nativePairMissing04380(id, name string) any {
	return map[string]any{"functionResponse": map[string]any{"id": id, "name": name, "response": map[string]any{"error": missingResultText04380}}}
}

// Direct examples from tagged Prodex 0.438.0
// gemini_bridge/hardening/contents/tool_pairs/tests.rs. The source policy
// compares id AND name and intentionally does not deduplicate identical
// missing calls. Synthetic results never execute a tool.
func TestProdex04380GeminiNativePairMissingResultsMatchesIDAndName(t *testing.T) {
	model := map[string]any{"role": "model", "parts": []any{
		map[string]any{"functionCall": map[string]any{"id": "a", "name": "lookup"}},
		map[string]any{"functionCall": map[string]any{"id": "b", "name": "検索"}},
		map[string]any{"functionCall": map[string]any{"id": "b", "name": "検索"}},
	}}
	real := map[string]any{"functionResponse": map[string]any{"id": "a", "name": "lookup", "response": map[string]any{"real": true}}}
	wrong := map[string]any{"functionResponse": map[string]any{"id": "b", "name": "other", "response": map[string]any{"real": true}}}
	user := map[string]any{"role": "user", "parts": []any{real, wrong}, "metadata": 7}
	original := []any{model, user}
	want := []any{model, map[string]any{"role": "user", "parts": []any{real, wrong, nativePairMissing04380("b", "検索"), nativePairMissing04380("b", "検索")}, "metadata": 7}}
	got := geminiRepairMissingToolResults04380(original)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("id/name pairing or duplicate missing results diverged from tagged kernel: got=%+v want=%+v", got, want)
	}
	if len(user["parts"].([]any)) != 2 {
		t.Fatal("mutated caller's original user parts")
	}
}

func TestProdex04380GeminiNativePairMissingResultsMalformedUserAndNullCall(t *testing.T) {
	model := map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": nil}}}
	malformed := map[string]any{"role": "user", "parts": "invalid", "keep": true}
	next := map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"id": "next", "name": "tool"}}}}
	original := []any{model, malformed, next}
	want := []any{model,
		map[string]any{"role": "user", "parts": []any{nativePairMissing04380("", "unknown")}},
		malformed, next,
		map[string]any{"role": "user", "parts": []any{nativePairMissing04380("next", "tool")}},
	}
	if got := geminiRepairMissingToolResults04380(original); !reflect.DeepEqual(got, want) {
		t.Fatalf("malformed following user or null call pairing mismatch: got=%+v", got)
	}
}

func TestProdex04380GeminiNativeOrphanResponseAddsExactSyntheticCalls(t *testing.T) {
	existing := map[string]any{"functionCall": map[string]any{"id": "a", "name": "known", "args": map[string]any{"keep": 1}}}
	user := map[string]any{"role": "user", "parts": []any{
		map[string]any{"functionResponse": map[string]any{"id": "a", "name": "known", "response": map[string]any{"ok": true}}},
		map[string]any{"functionResponse": map[string]any{"id": "a", "name": "other", "response": map[string]any{"ok": false}}},
		map[string]any{"functionResponse": nil},
	}}
	original := []any{map[string]any{"role": "model", "parts": []any{existing}, "keep": "first"}, user}
	want := []any{map[string]any{"role": "model", "parts": []any{existing, nativePairCall04380("a", "other"), nativePairCall04380("", "unknown")}, "keep": "first"}, user}
	if got := geminiRepairOrphanToolResponses04380(original); !reflect.DeepEqual(got, want) {
		t.Fatalf("orphan response call synthesis mismatched tagged source: got=%+v", got)
	}
}

func TestProdex04380GeminiNativeOrphanResponsePreviousInvalidAndRepeated(t *testing.T) {
	user := map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"id": "x", "name": "tool"}}}}
	invalid := []any{map[string]any{"role": "model", "parts": false}, user}
	if got := geminiRepairOrphanToolResponses04380(invalid); !reflect.DeepEqual(got, invalid) {
		t.Fatalf("invalid prior model parts changed: %+v", got)
	}
	unpaired := []any{user, user}
	synthetic := map[string]any{"role": "model", "parts": []any{nativePairCall04380("x", "tool")}}
	want := []any{synthetic, user, synthetic, user}
	if got := geminiRepairOrphanToolResponses04380(unpaired); !reflect.DeepEqual(got, want) {
		t.Fatalf("repeated unmatched responses not repaired independently: %+v", got)
	}
}
