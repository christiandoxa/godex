package kiro

import (
	"encoding/json"
	"testing"
)

func kiroUpdateEnvelope04384(params string) acpEnvelope {
	return acpEnvelope{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(params)}
}

// Tagged Prodex 0.438.4's runtime_kiro_acp_session_update calls the
// compiled Mojo required-field gate and ignores parse failures. A phantom
// tool_call without an ID/title must not become a client-visible synthetic
// Kiro activity or alter later tool title inheritance.
func TestProdex04384InvalidKiroACPUpdateCannotForgeToolActivity(t *testing.T) {
	for _, tc := range []struct {
		name, params string
		want         int
	}{
		{"canonical-tool", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"List files"}}`, 1},
		{"missing-id", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","title":"List files"}}`, 0},
		{"missing-title", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1"}}`, 0},
		{"wrong-id-type", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":1,"title":"List files"}}`, 0},
		{"wrong-title-type", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":42}}`, 0},
		{"missing-update-id", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call_update","title":"List files"}}`, 0},
		{"missing-session-id", `{"update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"List files"}}`, 0},
		{"wrong-session-id", `{"sessionId":123,"update":{"sessionUpdate":"tool_call","toolCallId":"call-1","title":"List files"}}`, 0},
		{"padded-update-kind", `{"sessionId":"session-1","update":{"sessionUpdate":"tool_call ","toolCallId":"call-1","title":"List files"}}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(tc.params)})
			if len(turn.toolActivities) != tc.want {
				t.Fatalf("malformed Kiro tool notification minted %d activities want %d: %#v", len(turn.toolActivities), tc.want, turn.toolActivities)
			}
		})
	}
}

func TestProdex04384InvalidKiroACPUpdateCannotReplaceValidUsage(t *testing.T) {
	valid := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":12,"size":40}}`)
	for _, tc := range []struct{ name, params string }{
		{"missing-used", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","size":0}}`},
		{"missing-size", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0}}`},
		{"negative-used", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":-1,"size":40}}`},
		{"float-size", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":3.5}}`},
		{"padded-kind", `{"sessionId":"session-1","update":{"sessionUpdate":" usage_update","used":0,"size":0}}`},
		{"missing-session", `{"update":{"sessionUpdate":"usage_update","used":0,"size":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := collectKiroTurnState([]acpEnvelope{valid, kiroUpdateEnvelope04384(tc.params)})
			if turn.usage == nil || turn.usage["used"] != uint64(12) || turn.usage["size"] != uint64(40) {
				t.Fatalf("invalid Kiro ACP usage update replaced valid usage: %#v", turn.usage)
			}
		})
	}
}

func TestProdex04384InvalidKiroACPUpdateCannotAppearInLiveStream(t *testing.T) {
	state := newKiroLiveState(5, "kiro-model", "synthetic")
	route := runtimeRoute{kind: routeChat}
	for _, params := range []string{
		`{"update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"unowned"}}}`,
		`{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk ","content":{"type":"text","text":"padded"}}}`,
		`{"sessionId":"session-1","update":{"sessionUpdate":"tool_call","title":"no id"}}`,
	} {
		frames := state.observe(route, kiroUpdateEnvelope04384(params))
		if len(frames) != 0 || state.turn.assistantText != "" || len(state.turn.toolActivities) != 0 {
			t.Fatalf("invalid Kiro live notification leaked output frames=%q state=%+v", frames, state.turn)
		}
	}
	canonical := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"valid"}}}`)
	if frames := state.observe(route, canonical); len(frames) != 1 || state.turn.assistantText != "valid" {
		t.Fatalf("valid Kiro chat notification was lost: frames=%q text=%q", frames, state.turn.assistantText)
	}
}

// Serde Value::as_u64 accepts the complete unsigned 64-bit range; Go's
// legacy runtimeUint mistakenly called json.Number.Int64 and returned zero
// for valid usage >=2^63, corrupting Kiro cost/capacity reporting.
func TestProdex04384KiroACPUsageUsesEntireUnsigned64BitRange(t *testing.T) {
	for _, tc := range []struct {
		name, used, size                  string
		wantUsed, wantSize, wantRemaining uint64
	}{
		{"max-u64", "18446744073709551615", "18446744073709551615", ^uint64(0), ^uint64(0), 0},
		{"greater-than-i64-max", "9223372036854775808", "18446744073709551615", uint64(1) << 63, ^uint64(0), uint64(1)<<63 - 1},
		{"ordinary-values", "12", "40", 12, 40, 28},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":` + tc.used + `,"size":` + tc.size + `}}`
			turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(params)})
			usage := turn.usage
			if usage == nil || usage["used"] != tc.wantUsed || usage["size"] != tc.wantSize || usage["remaining"] != tc.wantRemaining {
				t.Fatalf("Kiro parsed full u64 usage %s as %#v", tc.name, usage)
			}
		})
	}
}

func TestProdex04384MalformedKiroPlanAndCostLeaveEarlierState(t *testing.T) {
	validPlan := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":[{"content":"first","priority":"high","status":"open"}]}}`)
	validUsage := kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":1,"size":9}}`)
	for _, tc := range []struct{ name, params string }{
		{"plan-wrong-type", `{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":{}}}`},
		{"plan-entries-missing-priority", `{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":[{"content":"bad","status":"open"}]}}`},
		{"usage-invalid-cost-currency", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":0,"cost":{"amount":0.5}}}`},
		{"usage-invalid-cost-amount", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":0,"size":0,"cost":{"amount":"0.5","currency":"USD"}}}`},
		{"usage-overflow", `{"sessionId":"session-1","update":{"sessionUpdate":"usage_update","used":18446744073709551616,"size":0}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := collectKiroTurnState([]acpEnvelope{validPlan, validUsage, kiroUpdateEnvelope04384(tc.params)})
			if len(turn.plan) != 1 || turn.usage == nil || turn.usage["used"] != uint64(1) || turn.usage["size"] != uint64(9) {
				t.Fatalf("malformed Kiro plan/cost changed previously valid state=%+v", turn)
			}
		})
	}
}
