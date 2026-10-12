package main

import "testing"

func TestValidateProbeRejectsCachedMaximumPromotion(t *testing.T) {
	item := probe{Name: "cache", Model: "gpt-6.1-sol", ContextWindow: "500000", CompactLimit: "430000"}
	got := childArgs{Model: item.Model, ContextWindow: "872000", CompactLimit: "784800"}
	if err := validateProbe(got, childArgs{Model: item.Model, ContextWindow: item.ContextWindow, CompactLimit: item.CompactLimit}); err == nil {
		t.Fatal("cached maximum promotion escaped the probe")
	}
}

func TestNegativeControlDetectsMutation(t *testing.T) {
	if err := checkNegativeControl(); err != nil {
		t.Fatal(err)
	}
}

func TestParseChildArgsKeepsLastExplicitValues(t *testing.T) {
	got := parseChildArgs([]string{
		"--model", "gpt-6.1-sol", "-c", "model_context_window=500000",
		"-cmodel_auto_compact_token_limit=430000", "-c", "model_context_window=700000",
	})
	want := childArgs{Model: "gpt-6.1-sol", ContextWindow: "700000", CompactLimit: "430000"}
	if got != want {
		t.Fatalf("parsed child args = %#v, want %#v", got, want)
	}
}
