package kiro

import (
	"encoding/json"
	"testing"
)

// Source Mojo KIRO_ACP_STOP_REASON tests the first PRESENT key, not the
// first string value. A null, object or numeric stopReason shadows the
// later aliases, returning null rather than falling back to stop_reason.
func TestProdex04384KiroStopReasonHonorsFirstPresentField(t *testing.T) {
	for _, tc := range []struct {
		name, result, want string
	}{
		{"canonical", `{"stopReason":"max_tokens","stop_reason":"end_turn"}`, "max_tokens"},
		{"snake-case", `{"stop_reason":"refusal","status":"end_turn"}`, "refusal"},
		{"status-only", `{"status":"cancelled"}`, "cancelled"},
		{"null-top-prevents-fallback", `{"stopReason":null,"stop_reason":"max_tokens"}`, ""},
		{"numeric-top-prevents-fallback", `{"stopReason":7,"status":"refusal"}`, ""},
		{"object-top-prevents-fallback", `{"stopReason":{},"status":"cancelled"}`, ""},
		{"false-snake-prevents-fallback", `{"stop_reason":false,"status":"max_tokens"}`, ""},
		{"empty-string-top", `{"stopReason":"","stop_reason":"refusal"}`, ""},
		{"absent-all", `{}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope := acpEnvelope{Result: json.RawMessage(tc.result)}
			if got := kiroStopReason(envelope); got != tc.want {
				t.Fatalf("first-present Kiro stop field %s yielded %q want %q", tc.result, got, tc.want)
			}
		})
	}
}

func TestProdex04384KiroInvalidPrimaryStopDoesNotForgeIncompleteResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		incomplete bool
	}{
		{"canonical", `{"stopReason":"max_tokens"}`, true},
		{"null-then-max-tokens", `{"stopReason":null,"stop_reason":"max_tokens"}`, false},
		{"object-then-refusal", `{"stopReason":{},"status":"refusal"}`, false},
		{"snake-case-alone", `{"stop_reason":"max_tokens"}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			envelope := acpEnvelope{Result: json.RawMessage(tc.body)}
			reason, _ := kiroIncompleteDetails(envelope)
			if (reason != "") != tc.incomplete {
				t.Fatalf("invalid earlier Kiro stop fabricated an incomplete response: raw=%s reason=%q", tc.body, reason)
			}
		})
	}
}
