package kiro

import "testing"

// Tagged 0.438.4 Mojo kiro_activity_ascii_lower folds ASCII uppercase
// bytes only; Go Unicode Lower would wrongly replace U+212A KELVIN SIGN
// with ASCII 'k' or U+0130 dotted I with ASCII 'i'. Non-ASCII variants
// cannot impersonate sensitive ASCII substrings or status literals.
func TestProdex04384KiroActivitySensitiveLabelsFoldASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, title, want string
	}{
		{"exact-lower-token", "token report", "analysis"},
		{"exact-upper-token", "TOKEN report", "analysis"},
		{"mixed-case-authorization", "AuThOrIzAtIoN", "analysis"},
		{"unicode-kelvin-token", "toKen report", "toKen report"},
		{"unicode-dotted-i-credential", "credentİal report", "credentİal report"},
		{"ordinary-unicode", "Überprüfung 界", "Überprüfung 界"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := kiroActivityItem(tc.title, "running", "analysis", true, false)
			if item["name"] != tc.want {
				t.Fatalf("Kiro source ASCII-only redaction of %q produced %#v, want %q", tc.title, item["name"], tc.want)
			}
		})
	}
}

func TestProdex04384KiroActivityStatusASCIIOnly(t *testing.T) {
	for _, tc := range []struct {
		name, status, wantStatus, wantPhase string
	}{
		{"canonical-uppercase", "RUNNING", "running", "started"},
		{"canonical-failed", "FAILED", "failed", "failed"},
		{"padded-uppercase", "  COMPLETED ", "completed", "completed"},
		{"unicode-dotted-i-failed", "FAİLED", "unknown", "started"},
		{"unicode-dotted-i-running", "RUNNİNG", "unknown", "started"},
		{"unknown-status", "future-status", "unknown", "started"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := kiroActivityItem("Task", tc.status, "read", true, false)
			if item["status"] != tc.wantStatus || item["phase"] != tc.wantPhase {
				t.Fatalf("Kiro status %q => status=%v phase=%v, expected=%q/%q", tc.status, item["status"], item["phase"], tc.wantStatus, tc.wantPhase)
			}
		})
	}
}
