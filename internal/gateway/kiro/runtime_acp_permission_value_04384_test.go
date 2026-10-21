package kiro

import (
	"encoding/json"
	"testing"
)

// Tagged Rust ACP process reads params as serde_json::Value and uses
// `.find(kind==allow_once)`, ignoring malformed *unrelated* array elements.
// Go's typed []struct unmarshal previously rejected the entire list when
// any irrelevant entry's kind/optionId had the wrong type, cancelling an
// otherwise valid one-time subagent permission choice.
func TestProdex04384KiroACPSubAgentPermissionIgnoresMalformedUnrelatedOptions(t *testing.T) {
	for _, tc := range []struct {
		name, params, want string
	}{
		{"canonical", `{"options":[{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"unrelated-kind-number", `{"options":[{"optionId":"unrelated","kind":42},{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"unrelated-id-number", `{"options":[{"optionId":42,"kind":"reject_once"},{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"unrelated-null-member", `{"options":[null,{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"unrelated-array-element", `{"options":[[1,2,3],{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"allow-once-before-always", `{"options":[{"optionId":"always","kind":"allow_always"},{"optionId":"once","kind":"allow_once"}]}`, "once"},
		{"allow-always-control", `{"options":[{"optionId":"always","kind":"allow_always"}]}`, "always"},
		{"first-allow-once-nonstr-id", `{"options":[{"optionId":42,"kind":"allow_once"},{"optionId":"once","kind":"allow_once"}]}`, ""},
		{"first-allow-once-empty-id", `{"options":[{"optionId":"","kind":"allow_once"},{"optionId":"once","kind":"allow_once"}]}`, ""},
		{"not-an-options-array", `{"options":{"kind":"allow_once","optionId":"once"}}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := acpPermissionOption(json.RawMessage(tc.params))
			if got != tc.want {
				t.Fatalf("serde Value permission options=%s got=%q want=%q", tc.params, got, tc.want)
			}
		})
	}
}

func TestProdex04384KiroACPSubAgentPermissionReplySelectsValidOption(t *testing.T) {
	params := json.RawMessage(`{"options":[{"optionId":24,"kind":"reject_once"},{"optionId":"once","kind":"allow_once"}]}`)
	reply := permissionACPServerResponse(json.RawMessage(`"permission-1"`), params)
	result, ok := reply["result"].(map[string]any)
	if !ok {
		t.Fatal("permission response result is not an object")
	}
	outcome, ok := result["outcome"].(map[string]any)
	if !ok || outcome["outcome"] != "selected" || outcome["optionId"] != "once" {
		t.Fatalf("Kiro subagent permission failed to select valid once option: %#v", reply)
	}
}
