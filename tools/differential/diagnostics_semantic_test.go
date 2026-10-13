package main

import (
	"strings"
	"testing"
)

func TestProdexLaunchBannerUsesCapturedSeparatorWidth(t *testing.T) {
	lines := strings.Split(prodexDeepSeekLaunchBanner, "\n")
	if len(lines) < 4 {
		t.Fatalf("launch banner has %d lines", len(lines))
	}
	want := "[ Runtime Provider ] " + strings.Repeat("=", 89)
	if lines[3] != want {
		t.Fatalf("runtime provider separator = %q, want %q", lines[3], want)
	}
}

func TestSourcePinnedRuntimeBannerMatchesOnlyExpectedOutput(t *testing.T) {
	prodex := productRun{Stderr: prodexDeepSeekLaunchBanner}
	godex := productRun{Stderr: ""}
	if !equivalentRuntimeDiagnostics(prodex, godex) {
		t.Fatal("exact tagged startup banner was not accepted")
	}
	for _, unexpected := range []string{
		"warning: quota exhausted\n",
		"authentication failed\n",
		"model provider disagrees\n",
	} {
		changed := prodex
		changed.Stderr += unexpected
		if equivalentRuntimeDiagnostics(changed, godex) {
			t.Fatalf("unexpected diagnostic ignored: %q", unexpected)
		}
	}
	changed := prodex
	changed.Stderr = strings.Replace(changed.Stderr, "Using one provider API key", "Using two provider API keys", 1)
	if equivalentRuntimeDiagnostics(changed, godex) {
		t.Fatal("changed launch semantics were ignored")
	}
}

func TestCancellationDiagnosticsRequireMatchingReasonAndLocalProxy(t *testing.T) {
	reference := productRun{
		ExitStatus: 1, Cancelled: true,
		Stderr: prodexDeepSeekLaunchBanner +
			"synthetic Codex shim request failed: Post \"http://127.0.0.1:43111/v1/responses\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)\n",
	}
	candidate := productRun{
		ExitStatus: 1, Cancelled: true,
		Stderr: "synthetic Codex shim request failed: Post \"http://127.0.0.1:43001/backend-api/godex/responses\": context deadline exceeded (Client.Timeout exceeded while awaiting headers)\n",
	}
	if !equivalentRuntimeDiagnostics(reference, candidate) {
		t.Fatal("same synthetic cancellation reason with distinct local proxy URLs was rejected")
	}
	fixtures := []struct {
		name   string
		change func(*productRun)
	}{
		{"remote_origin", func(r *productRun) { r.Stderr = strings.ReplaceAll(r.Stderr, "127.0.0.1", "example.test") }},
		{"wrong_path", func(r *productRun) {
			r.Stderr = strings.ReplaceAll(r.Stderr, "/backend-api/godex/responses", "/v1/chat/completions")
		}},
		{"changed_reason", func(r *productRun) {
			r.Stderr = strings.ReplaceAll(r.Stderr, "context deadline exceeded", "connection refused")
		}},
		{"extra_warning", func(r *productRun) { r.Stderr += "provider unavailable\n" }},
		{"missing_cancel", func(r *productRun) { r.Cancelled = false }},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			mutated := candidate
			fixture.change(&mutated)
			if equivalentRuntimeDiagnostics(reference, mutated) {
				t.Fatalf("invalid cancellation evidence accepted: %s", fixture.name)
			}
		})
	}
}

// Credential count is part of the tagged launch contract: a second key
// enables rotation; the one-key banner must not be accepted for that case.
func TestProdexTwoKeyLaunchBannerIsExact(t *testing.T) {
	reference := productRun{
		Stderr:  prodexDeepSeekTwoKeyLaunchBanner,
		Command: []string{"<synthetic-multiple-credentials-in-environment>"},
	}
	candidate := productRun{Stderr: ""}
	if !equivalentRuntimeDiagnostics(reference, candidate) {
		t.Fatal("tagged two-key launch banner rejected")
	}
	reference.Stderr = prodexDeepSeekLaunchBanner
	if equivalentRuntimeDiagnostics(reference, candidate) {
		t.Fatal("misleading one-key banner accepted for two-key provider pool")
	}
	reference.Stderr = prodexDeepSeekTwoKeyLaunchBanner + "quota preflight actually attempted\n"
	if equivalentRuntimeDiagnostics(reference, candidate) {
		t.Fatal("unexpected diagnostic ignored for two-key mode")
	}
}
