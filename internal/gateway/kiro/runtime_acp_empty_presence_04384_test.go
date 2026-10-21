package kiro

import "testing"

// Tagged Rust keeps Option<Vec> and Option<String> presence even when an
// update explicitly replaces earlier fields with [] or "". The compiled
// Kiro AcpMetadata kernel serializes those present empty values; it does
// not discard them on length/blank-string checks.
func TestProdex04384KiroEmptyACPUpdatesRetainPresence(t *testing.T) {
	for _, tc := range []struct {
		name        string
		params      string
		key         string
		emptyString bool
	}{
		{"empty-plan", `{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":[]}}`, "plan", false},
		{"empty-commands", `{"sessionId":"session-1","update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}`, "available_commands", false},
		{"empty-mode", `{"sessionId":"session-1","update":{"sessionUpdate":"current_mode_update","currentModeId":""}}`, "current_mode_id", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			turn := collectKiroTurnState([]acpEnvelope{kiroUpdateEnvelope04384(tc.params)})
			metadata := turn.metadata("")
			kiro, _ := metadata["kiro"].(map[string]any)
			value, found := kiro[tc.key]
			if !found {
				t.Fatalf("empty Kiro field %s absent from metadata=%#v", tc.key, metadata)
			}
			if tc.emptyString {
				if value != "" {
					t.Fatalf("empty Kiro field %s=%#v want empty string", tc.key, value)
				}
			} else if values, ok := value.([]any); !ok || len(values) != 0 {
				t.Fatalf("empty Kiro field %s=%#v want []", tc.key, value)
			}
		})
	}
}

func TestProdex04384KiroEmptyUpdatesClearPreviousValues(t *testing.T) {
	updates := []acpEnvelope{
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":[{"content":"initial","priority":"high","status":"open"}]}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"available_commands_update","availableCommands":[{"name":"initial"}]}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"current_mode_update","currentModeId":"initial"}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","title":"initial","updatedAt":"previous"}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"plan","entries":[]}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"available_commands_update","availableCommands":[]}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"current_mode_update","currentModeId":""}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","title":"","updatedAt":""}}`),
	}
	turn := collectKiroTurnState(updates)
	meta := turn.metadata("")
	kiro, _ := meta["kiro"].(map[string]any)
	for _, key := range []string{"plan", "available_commands"} {
		values, ok := kiro[key].([]any)
		if !ok || len(values) != 0 {
			t.Fatalf("empty %s did not override earlier value: %#v", key, kiro[key])
		}
	}
	if got := kiro["current_mode_id"]; got != "" {
		t.Fatalf("empty current mode lost: %#v", got)
	}
	info, ok := kiro["session_info"].(map[string]any)
	if !ok || info["title"] != "" || info["updated_at"] != "" {
		t.Fatalf("empty Kiro session info update did not overwrite earlier title/timestamp: %#v", kiro["session_info"])
	}
}

func TestProdex04384KiroSessionInfoPreservesUnicodeWhitespaceAndAbsentFields(t *testing.T) {
	inputs := []acpEnvelope{
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","title":"  🔍  ","updatedAt":"first"}}`),
		kiroUpdateEnvelope04384(`{"sessionId":"session-1","update":{"sessionUpdate":"session_info_update","updatedAt":""}}`),
	}
	turn := collectKiroTurnState(inputs)
	meta := turn.metadata("")
	kiro, _ := meta["kiro"].(map[string]any)
	info, ok := kiro["session_info"].(map[string]any)
	if !ok || info["title"] != "  🔍  " || info["updated_at"] != "" {
		t.Fatalf("Kiro optional session info lost source string/presence: %#v", kiro["session_info"])
	}
}
