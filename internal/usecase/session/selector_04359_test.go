package session

import (
	"context"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
)

// Exact Prodex 0.435.9 session-selector Mojo compares bytes, folding ASCII
// A-Z only. Unicode casefold is not a valid session-ID match or prefix.
func TestProdex04359SessionSelectorUsesASCIICasefoldOnly(t *testing.T) {
	catalog := NewCatalog(accountsFake{}, readerFake{reports: map[string][]sessionentity.Session{
		"one": {{ID: "ÄBC", Path: "/profiles/one/sessions/unicode.jsonl", UpdatedUnix: 1},
			{ID: "AbCd123", Path: "/profiles/one/sessions/ascii.jsonl", UpdatedUnix: 2}},
	}}, &launcherFake{})
	for _, selector := range []string{"ä", "äbc"} {
		if report, err := catalog.Resolve(context.Background(), selector); err == nil {
			t.Fatalf("Unicode-cased session selector %q resolved %q; Prodex requires ASCII-only casefold", selector, report.ID)
		}
	}
	for _, selector := range []string{"aBcD", "ABcd1"} {
		report, err := catalog.Resolve(context.Background(), selector)
		if err != nil || report.ID != "AbCd123" {
			t.Fatalf("ASCII-cased session selector %q = %+v, %v", selector, report, err)
		}
	}
}
