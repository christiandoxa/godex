package kiro

import "testing"

// Tagged 0.438.4 Rust uses Option<Value>::is_some for rawInput/rawOutput,
// even for explicit null, but Option<Vec<Value>>::is_some_and(!is_empty)
// for content and locations. Presence alone must not make an empty list
// look like hidden user tool content.
func TestProdex04384KiroToolDetailsOmittedMatchesRustOptionSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, extra string
		want        bool
	}{
		{"no-extra", ``, false},
		{"raw-input-null", `,"rawInput":null`, true},
		{"raw-output-null", `,"rawOutput":null`, true},
		{"raw-input-empty", `,"rawInput":{}`, true},
		{"raw-output-empty-string", `,"rawOutput":""`, true},
		{"content-empty-array", `,"content":[]`, false},
		{"locations-empty-array", `,"locations":[]`, false},
		{"content-invalid-object", `,"content":{"text":"not-an-array"}`, false},
		{"locations-invalid-object", `,"locations":{"path":"not-an-array"}`, false},
		{"content-one-item", `,"content":[{"type":"text","text":"hidden"}]`, true},
		{"locations-one-item", `,"locations":[{"path":"secret"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"List files"` + tc.extra + `}}`
			state := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(params)})
			if len(state.toolActivities) != 1 {
				t.Fatalf("valid Kiro activity absent: %#v", state.toolActivities)
			}
			item, _ := state.toolActivities[0].(map[string]any)
			got, _ := item[kiroFieldDetailsOmitted].(bool)
			if got != tc.want {
				t.Fatalf("Kiro tool detail disclosure policy %q got=%t want=%t activity=%#v", tc.name, got, tc.want, item)
			}
		})
	}
}

func TestProdex04384KiroToolUpdateDetailsOmittedMatchesRustOptionSemantics(t *testing.T) {
	initial := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"List files"}}`)
	for _, tc := range []struct {
		name, extra string
		want        bool
	}{
		{"null-raw-output", `,"rawOutput":null`, true},
		{"empty-content", `,"content":[]`, false},
		{"empty-locations", `,"locations":[]`, false},
		{"nonempty-locations", `,"locations":[{"path":"hidden"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call_update","toolCallId":"call-1"` + tc.extra + `}}`)
			state := collectKiroTurnState([]acpEnvelope{initial, next})
			if len(state.toolActivities) != 2 {
				t.Fatalf("tool update missing: %#v", state.toolActivities)
			}
			item, _ := state.toolActivities[1].(map[string]any)
			got, _ := item[kiroFieldDetailsOmitted].(bool)
			if got != tc.want {
				t.Fatalf("Kiro tool update %q detail flag got=%t want=%t", tc.name, got, tc.want)
			}
		})
	}
}
