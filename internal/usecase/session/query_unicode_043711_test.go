package session

import (
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

// Prodex 0.437.11's Rust adapter applies full Unicode str::to_lowercase
// to each session report field before its bounded Mojo byte comparison.
// Go's simple strings.ToLower does not expand dotted I or use final sigma.
func TestProdex043711SessionQueryUnicodeLowercaseMatchesTaggedAdapter(t *testing.T) {
	for _, tc := range []struct {
		name, field, query string
		want               bool
	}{
		{"dotted I expands", "İstanbul", "i\u0307stanbul", true},
		{"Greek contextual final sigma", "ΣΟΣ", "σος", true},
		{"dotted I does not equal plain i", "İstanbul", "istanbul", false},
		{"plain I does not equal dotted lowercase", "Istanbul", "i\u0307stanbul", false},
		{"ordinary ASCII search", "HELLO world", "hello", true},
		{"CLI filter whitespace remains trimmed", "anything", "  ", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesSessionQuery(sessionmodel.Report{
				ID:         "01911111-1111-7111-8111-111111111111",
				ThreadName: tc.field,
				Path:       "/synthetic/nonmatching.jsonl",
				Profile:    "local",
			}, sessionmodel.Query{Text: tc.query})
			if got != tc.want {
				t.Fatalf("session report Unicode query %q in %q = %t, want %t",
					tc.query, tc.field, got, tc.want)
			}
		})
	}
}

func TestProdex043711SessionListUnicodeQuerySelectsCorrectReport(t *testing.T) {
	catalog := NewCatalog(accountsFake{}, readerFake{reports: map[string][]sessionentity.Session{
		"one": {{
			ID:         "01911111-1111-7111-8111-111111111111",
			ThreadName: "İstanbul",
			Path:       "/synthetic/first.jsonl",
		}},
		"two": {{
			ID:         "01922222-2222-7222-8222-222222222222",
			ThreadName: "Istanbul",
			Path:       "/synthetic/second.jsonl",
		}},
	}}, &launcherFake{})
	for _, tc := range []struct {
		query string
		want  string
	}{
		{"i\u0307stanbul", "İstanbul"},
		{"istanbul", "Istanbul"},
	} {
		results, err := catalog.List(t.Context(), sessionmodel.Query{Text: tc.query})
		if err != nil || len(results) != 1 || results[0].ThreadName != tc.want {
			t.Fatalf("list query %q returned %+v, err=%v; want %q",
				tc.query, results, err, tc.want)
		}
	}
}
