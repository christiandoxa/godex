package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384GeminiIgnoredOracleRejectsMatchingControlLeak(t *testing.T) {
	for _, scenario := range geminiIgnoredControlCases04384() {
		if !json.Valid(geminiIgnoredInput04384(scenario)) {
			t.Fatal("ignored-control fixture is not complete JSON")
		}
	}
	for _, scenario := range []string{"request-ignored-unknown-null", "request-parallel-missing", "request-ignored-seed-"} {
		if _, _, known := geminiIgnoredControl04384(scenario); known {
			t.Fatal("unknown ignored-control fixture was accepted")
		}
	}
	if !geminiIgnoredNegativeControl04384() {
		t.Fatal("matching leaked ignored controls escaped exact upstream invariants")
	}
}
