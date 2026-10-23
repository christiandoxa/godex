package session

import (
	"context"
	"reflect"
	"testing"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

func TestExplicitProfileResolvesLegacySessionWithoutSharedCatalogMutation(t *testing.T) {
	catalog := NewCatalog(accountsFake{}, readerFake{reports: map[string][]sessionentity.Session{
		"one": {{ID: "a111", Path: "legacy-one", Source: "cli"}},
		"two": {{ID: "a111", Path: "foreign-duplicate", Source: "cli"}},
	}}, nil)
	catalog.SetSharedCodexHome("shared")
	catalog.SetOwnerLookup(func(context.Context, string) (string, error) { return "two", nil })
	input := sessionmodel.Launch{
		ProfileSelector: "personal", SessionSelector: "a1", IDIndex: 2,
		Arguments: []string{"exec", "resume", "a1", "continue"},
	}
	report, args, err := catalog.ResolveArguments(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if report.Profile != "personal" || report.AccountID != "one" || report.UpstreamAccountID != "two" || report.CodexHome != "one" || report.Path != "legacy-one" || !reflect.DeepEqual(args, []string{"exec", "resume", "a111", "continue"}) {
		t.Fatalf("legacy profile target/arguments = %+v / %q", report, args)
	}
	listed, err := catalog.List(t.Context(), sessionmodel.Query{})
	if err != nil || len(listed) != 0 {
		t.Fatalf("legacy resolution changed shared listing: %+v err=%v", listed, err)
	}
	for _, profile := range []string{"", "missing"} {
		input.ProfileSelector = profile
		if _, _, err := catalog.ResolveArguments(t.Context(), input); err == nil {
			t.Fatalf("legacy session accepted without a valid explicit profile %q", profile)
		}
	}
	input.ProfileSelector, input.AccountSelector = "personal", "work"
	if _, _, err := catalog.ResolveArguments(t.Context(), input); err == nil {
		t.Fatal("legacy resolution bypassed account ownership")
	}
}

func TestLegacyProfileFallbackPreservesSharedTargetsAndAmbiguity(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		shared    []sessionentity.Session
		legacy    []sessionentity.Session
		wantPath  string
		wantError bool
	}{
		{"shared target", []sessionentity.Session{{ID: "a111", Path: "shared-path"}}, []sessionentity.Session{{ID: "a111", Path: "legacy-path"}}, "shared-path", false},
		{"shared ambiguity", []sessionentity.Session{{ID: "a111"}, {ID: "a111"}}, []sessionentity.Session{{ID: "a111"}}, "", true},
		{"legacy ambiguity", nil, []sessionentity.Session{{ID: "a111"}, {ID: "a111"}}, "", true},
		{"missing target", nil, nil, "", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			catalog := NewCatalog(accountsFake{}, readerFake{reports: map[string][]sessionentity.Session{"shared": fixture.shared, "one": fixture.legacy}}, nil)
			catalog.SetSharedCodexHome("shared")
			input := sessionmodel.Launch{ProfileSelector: "personal", SessionSelector: "a111", IDIndex: 1, Arguments: []string{"resume", "a111"}}
			report, _, err := catalog.ResolveArguments(t.Context(), input)
			if (err != nil) != fixture.wantError || (err == nil && report.Path != fixture.wantPath) {
				t.Fatalf("target=%+v err=%v", report, err)
			}
		})
	}
}
