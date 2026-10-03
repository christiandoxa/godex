package runtime

import (
	"reflect"
	"strings"
	"testing"
)

func TestNativeAntigravityConsumesDisabledProdexFeatures(t *testing.T) {
	arguments := []string{
		"--no-presidio", "--no-sub-agent", "--no-auto-rotate", "--full-access",
		"--provider", "gemini", "--cli", "agy",
		"--model", "gemini-3.1-pro", "exec", "review",
	}
	if !UsesNativeAntigravity(arguments) {
		t.Fatal("disabled Prodex feature flags hid native Antigravity selection")
	}
	selection, got, err := parseRunArguments(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if selection.CLI != "agy" || selection.Model != "gemini-3.1-pro" ||
		!reflect.DeepEqual(got, []string{"exec", "review"}) {
		t.Fatalf("native Antigravity selection/arguments = %#v / %#v", selection, got)
	}
}

func TestNativeAntigravityOptionsAfterPositionalArguments(t *testing.T) {
	arguments := []string{"session-id", "--provider", "gemini", "--cli", "agy", "--no-presidio"}
	if !UsesNativeAntigravity(arguments) {
		t.Fatal("native Antigravity selection after a positional argument was not detected")
	}
	selection, got, err := parseRunArguments(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "gemini" || selection.CLI != "agy" || !reflect.DeepEqual(got, []string{"session-id"}) {
		t.Fatalf("native Antigravity selection/arguments = %#v / %#v", selection, got)
	}
}

func TestNativeAntigravityRejectsEnabledProdexFeatures(t *testing.T) {
	for _, fixture := range [][]string{
		{"--presidio"}, {"--sub-agent"}, {"--auto-rotate"}, {"--skip-quota-check"}, {"--no-proxy"},
		{"--tool", "rtk"}, {"--require-tool", "rtk"}, {"--sub-agent-provider", "openai"},
		{"--sub-agent-model", "gpt-6-luna"}, {"--sub-agent-model-reasoning-effort", "max"},
		{"--sub-agent-url", "https://example.test"}, {"--sub-agent-max-concurrency", "2"},
	} {
		arguments := append(append([]string(nil), fixture...), "--provider", "gemini", "--cli", "agy", "exec")
		if !UsesNativeAntigravity(arguments) {
			t.Fatalf("%#v hid native Antigravity selection", fixture)
		}
		_, _, err := parseRunArguments(arguments)
		if err == nil || !strings.Contains(err.Error(), "unsupported for native Antigravity") {
			t.Fatalf("native Antigravity accepted %#v: %v", fixture, err)
		}
	}
	_, _, err := parseRunArguments([]string{"--provider", "gemini", "--cli", "agy", "--presidio"})
	if err == nil || !strings.Contains(err.Error(), "--presidio is unsupported") {
		t.Fatalf("native Antigravity Presidio error = %v", err)
	}
}
