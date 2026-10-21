package kiro

import "testing"

// Tagged Kiro tool ledger uses the *presence* of title/kind string fields
// (Option<String>), not whether their trimmed values are nonblank. A
// present empty string suppresses prior-label inheritance; only an ABSENT
// field is allowed to use the previous title/kind.
func TestProdex04384KiroExplicitEmptyToolFieldsDoNotInheritOldLabels(t *testing.T) {
	for _, tc := range []struct {
		name, extra, wantName string
		wantKind              any
	}{
		{"absent-both", ``, "Initial title", "read"},
		{"explicit-empty-title", `,"title":""`, "read", "read"},
		{"explicit-empty-kind", `,"kind":""`, "Initial title", nil},
		{"explicit-both-empty", `,"title":"","kind":""`, "Kiro internal activity", nil},
		{"explicit-title-whitespace", `,"title":"  "`, "read", "read"},
		{"explicit-kind-whitespace", `,"kind":"  "`, "Initial title", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"Initial title","kind":"read"}}`)
			second := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call_update","toolCallId":"call-1"` + tc.extra + `}}`)
			turn := collectKiroTurnState([]acpEnvelope{first, second})
			if len(turn.toolActivities) != 2 {
				t.Fatalf("activity count %d, want 2", len(turn.toolActivities))
			}
			item, _ := turn.toolActivities[1].(map[string]any)
			if item["name"] != tc.wantName || item["kind"] != tc.wantKind {
				t.Fatalf("present empty Kiro tool update %s unexpectedly inherited label: name=%#v kind=%#v want %q/%#v", tc.name, item["name"], item["kind"], tc.wantName, tc.wantKind)
			}
		})
	}
}

// The compiled KIRO_ACP_MAX_TOOL_ID_BYTES policy admits an empty string
// identifier (length 0) for label storage; it only rejects IDs >256 bytes.
// Kiro ACP Rust's discriminator validation checks stringness, not nonblank.
func TestProdex04384KiroEmptyStringToolIDCanInheritLabel(t *testing.T) {
	first := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"","title":"Reusable label","kind":"read"}}`)
	second := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call_update","toolCallId":""}}`)
	turn := collectKiroTurnState([]acpEnvelope{first, second})
	if len(turn.toolActivities) != 2 {
		t.Fatalf("empty ID tool activity count=%d", len(turn.toolActivities))
	}
	item, _ := turn.toolActivities[1].(map[string]any)
	if item["name"] != "Reusable label" || item["kind"] != "read" {
		t.Fatalf("Mojo empty-key permitted label inheritance not preserved: %#v", item)
	}
}
