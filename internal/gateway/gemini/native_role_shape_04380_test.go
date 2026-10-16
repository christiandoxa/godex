package gemini

import (
	"reflect"
	"testing"
)

// Tagged Prodex 0.438.0 gemini_provider_core_harden_contents first coalesces
// adjacent contents of identical role without discarding part order, then
// enforces an initial and final user boundary on a model-only transcript.
func TestProdex04380GeminiNativeContentCoalescesAdjacentRoles(t *testing.T) {
	chat := map[string]any{"messages": []any{
		map[string]any{"role": "user", "content": "first"},
		map[string]any{"role": "user", "content": "second"},
		map[string]any{"role": "assistant", "content": "reply-1"},
		map[string]any{"role": "assistant", "content": "reply-2"},
		map[string]any{"role": "user", "content": "last"},
	}}
	request, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	expected := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "first"}, map[string]any{"text": "second"}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "reply-1"}, map[string]any{"text": "reply-2"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "last"}}},
	}
	if !reflect.DeepEqual(request["contents"], expected) {
		t.Fatalf("Gemini native adjacent-role coalescing differs from tagged kernel: %+v", request["contents"])
	}
}

func TestProdex04380GeminiNativeContentRepairsModelOnlyBoundaries(t *testing.T) {
	chat := map[string]any{"messages": []any{map[string]any{"role": "assistant", "content": "existing answer"}}}
	request, err := geminiGenerateContentRequest(chat, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	expected := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "[Continuing from previous Codex context.]"}}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "existing answer"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Please continue."}}},
	}
	if !reflect.DeepEqual(request["contents"], expected) {
		t.Fatalf("Gemini native model-only role boundary diverged from tagged hardener: %+v", request["contents"])
	}
}

func TestProdex04380GeminiNativePlainRoleHardenerPreservesFirstMetadataAndParts(t *testing.T) {
	original := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "first"}}, "first_metadata": map[string]any{"keep": true}},
		map[string]any{"role": "user", "parts": []any{false, map[string]any{"text": "second"}}, "later_metadata": "must not replace first"},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "answer"}}},
	}
	expected := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "first"}, false, map[string]any{"text": "second"}}, "first_metadata": map[string]any{"keep": true}},
		map[string]any{"role": "model", "parts": []any{map[string]any{"text": "answer"}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "Please continue."}}},
	}
	shaped := geminiEnforceContentRoleShape04380(original)
	if !reflect.DeepEqual(shaped, expected) {
		t.Fatalf("native plain role hardening lost first metadata or part order: %+v", shaped)
	}
	// The original request cannot be mutated by the coalescing operation.
	if len(original[0].(map[string]any)["parts"].([]any)) != 1 {
		t.Fatal("native role normalization mutated original caller-owned parts")
	}
}

func TestProdex04380GeminiNativeToolPairingCoalescesFollowingUserText(t *testing.T) {
	original := []any{
		map[string]any{"role": "model", "parts": []any{map[string]any{"functionCall": map[string]any{"name": "lookup", "id": "call-1"}}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"functionResponse": map[string]any{"name": "lookup", "id": "call-1"}}}},
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "following user text"}}},
	}
	expected := []any{
		map[string]any{"role": "user", "parts": []any{map[string]any{"text": "[Continuing from previous Codex context.]"}}},
		original[0],
		map[string]any{"role": "user", "parts": []any{
			map[string]any{"functionResponse": map[string]any{"name": "lookup", "id": "call-1"}},
			map[string]any{"text": "following user text"},
		}},
	}
	if got := geminiHardenContents04380(original); !reflect.DeepEqual(got, expected) {
		t.Fatalf("native hardening changed paired identity or user text: %+v", got)
	}
}

func TestProdex04380GeminiNativeEmptyPlainContentsStayEmpty(t *testing.T) {
	if actual := geminiHardenContents04380(nil); len(actual) != 0 {
		t.Fatalf("empty native Gemini transcript invented a turn: %+v", actual)
	}
}
