package codex

import (
	"encoding/json"
	"testing"
)

// Exact tagged Prodex 0.437.13 session_selector_matches() compares bytes,
// folding ASCII A-Z only. Unicode simple-fold / lowercase can improperly
// bind a malformed or foreign session_meta resume ID to the requested owner.
func TestProdex043713SessionMetadataSelectorMatchesOnlyASCIIBytes(t *testing.T) {
	for _, tc := range []struct {
		name, id, selector string
		want               bool
	}{
		{"ASCII exact", "01900000-0000-7000-8000-000000000713", "01900000-0000-7000-8000-000000000713", true},
		{"ASCII case-insensitive prefix", "ABCDEF00-0000-7000-8000-000000000713", "abcdef00", true},
		{"ASCII full lowercase selector", "ABCDEF00-0000-7000-8000-000000000713", "abcdef00-0000-7000-8000-000000000713", true},
		{"different ASCII prefix", "ABCDEF00-0000-7000-8000-000000000713", "abce", false},
		{"shorter candidate", "ABC", "abcd", false},
		{"Unicode Kelvin versus ASCII K", "K-owner", "k-owner", false},
		{"Unicode capital I dotted versus lowercase", "İ-start", "i-start", false},
		{"Unicode long S versus ASCII s", "ſession", "session", false},
		{"Unicode sigma simple-fold", "Σ-id", "σ-id", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionIDMatchesSelector(tc.id, tc.selector); got != tc.want {
				t.Fatalf("sessionIDMatchesSelector(%q,%q)=%v, want %v", tc.id, tc.selector, got, tc.want)
			}
			line := `{"type":"session_meta","payload":{"id":` + quotedJSONString043713(tc.id) + `}}`
			if got := sessionLineResumeIDMatches(line, tc.selector); got != tc.want {
				t.Fatalf("sessionLineResumeIDMatches(%q,%q)=%v, want %v", line, tc.selector, got, tc.want)
			}
		})
	}
}

func quotedJSONString043713(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
