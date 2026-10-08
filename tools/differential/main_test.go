package main

import "testing"

func TestNegativeControlDetectsClientStatusMismatch(t *testing.T) {
	if !negativeControl() {
		t.Fatal("comparison did not detect changed client status")
	}
}

func TestRedactRemovesSyntheticKey(t *testing.T) {
	if got := redact("Authorization: Bearer " + apiKey); got != "Authorization: Bearer <synthetic-key>" {
		t.Fatalf("redact = %q", got)
	}
}
