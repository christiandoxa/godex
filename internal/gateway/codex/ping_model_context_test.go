package codex

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

func TestPingModelContextUsesTaggedStaticCatalog(t *testing.T) {
	home := t.TempDir()
	base := pingArguments(pingmodel.Options{Model: "gpt-5.3-codex"})
	got, err := pingModelContextArguments(home, base)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{"-c", "model_context_window=400000", "-c", "model_auto_compact_token_limit=360000"}
	if len(got) != len(base)+len(wantPrefix) || !reflect.DeepEqual(got[:len(wantPrefix)], wantPrefix) ||
		!reflect.DeepEqual(got[len(wantPrefix):], base) {
		t.Fatalf("enriched args = %#v", got)
	}
}

func TestPingModelContextLeavesUnknownLargeModelUnchangedWithoutMetadata(t *testing.T) {
	home := t.TempDir()
	base := pingArguments(pingmodel.Options{Model: "gpt-5.9-custom"})
	got, err := pingModelContextArguments(home, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("unknown model args = %#v, want %#v", got, base)
	}
}

func TestPingModelContextUsesTaggedCacheMaxContextPolicy(t *testing.T) {
	home := t.TempDir()
	cache := "{\"models\":[{\"slug\":\" GPT-6-LUNA \",\"context_window\":272000,\"max_context_window\":872000}]}"
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	base := pingArguments(pingmodel.Options{Model: " gpt-6-luna "})
	got, err := pingModelContextArguments(home, base)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"model_context_window=872000", "model_auto_compact_token_limit=784800"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("cache-enriched args missing %q: %#v", want, got)
		}
	}
}

func TestPingModelContextTreatsConfiguredModelCacheContextAsExplicit(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"gpt-5.5\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cache := "{\"models\":[{\"slug\":\"gpt-5.5\",\"context_window\":272000}]}"
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
	base := pingArguments(pingmodel.Options{})
	got, err := pingModelContextArguments(home, base)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(got, "\n")
	if strings.Contains(joined, "model_context_window=272000") {
		t.Fatalf("configured cached context was re-injected: %#v", got)
	}
	if !strings.Contains(joined, "model_auto_compact_token_limit=244800") {
		t.Fatalf("configured cached context did not drive compact limit: %#v", got)
	}
}

func TestPingModelContextPreservesExplicitConfigContextAndCompactLimit(t *testing.T) {
	home := t.TempDir()
	content := "model = \"gpt-5.4\"\nmodel_context_window = \" 333000 \"\nmodel_auto_compact_token_limit = 300000\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	base := pingArguments(pingmodel.Options{})
	got, err := pingModelContextArguments(home, base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, base) {
		t.Fatalf("explicit config values were redundantly injected: %#v", got)
	}
}

func TestPingModelContextRejectsMalformedConfig(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{Model: "gpt-5.4"})); err == nil {
		t.Fatal("malformed config unexpectedly accepted")
	}
}

func TestPingOpenAIProductionPathInjectsLargeModelContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("integration helper uses a POSIX shell")
	}
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "args")
	script := filepath.Join(root, "codex")
	body := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > \"$GODEX_PING_ARGS\"",
		"printf '%s\\n' '{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"hello\"}}'",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_PING_ARGS", record)
	process := NewCodexProcess(script, Terminal{})
	result, err := process.PingOpenAI(context.Background(), pingmodel.Target{Name: "work", CodexHome: home}, pingmodel.Options{
		Model: "gpt-5.3-codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ping result = %#v", result)
	}
	content, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	joined := string(content)
	for _, want := range []string{"model_context_window=400000", "model_auto_compact_token_limit=360000", "--model\ngpt-5.3-codex\n"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("child args missing %q: %q", want, joined)
		}
	}
}

func TestPingModelContextMatchesTaggedCacheEdgeSemantics(t *testing.T) {
	t.Run("trailing JSON invalidates cache", func(t *testing.T) {
		home := t.TempDir()
		content := "{\"models\":[{\"slug\":\"gpt-5.3-codex\",\"context_window\":777000}]} {}"
		if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{Model: "gpt-5.3-codex"}))
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "model_context_window=400000") || strings.Contains(joined, "777000") {
			t.Fatalf("trailing-JSON cache was used: %#v", got)
		}
	})

	t.Run("empty slug does not fall back to id", func(t *testing.T) {
		home := t.TempDir()
		content := "{\"models\":[{\"slug\":\"\",\"id\":\"gpt-6-luna\",\"context_window\":222000}]}"
		if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{Model: "gpt-6-luna"}))
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "model_context_window=1050000") || strings.Contains(joined, "222000") {
			t.Fatalf("empty slug incorrectly fell back to id: %#v", got)
		}
	})

	t.Run("present null max does not fall through to alternate max key", func(t *testing.T) {
		home := t.TempDir()
		content := "{\"models\":[{\"slug\":\"gpt-6-luna\",\"context_window\":272000,\"max_context_window\":null,\"max_context_window_tokens\":999000}]}"
		if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{Model: "gpt-6-luna"}))
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "model_context_window=272000") || strings.Contains(joined, "999000") {
			t.Fatalf("null primary max incorrectly used alternate max: %#v", got)
		}
	})

	t.Run("non-openai configured provider makes cache context non-explicit", func(t *testing.T) {
		home := t.TempDir()
		config := "model = \"gpt-5.5\"\nmodel_provider = \"prodex-copilot\"\n"
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		cache := "{\"models\":[{\"slug\":\"gpt-5.5\",\"context_window\":272000}]}"
		if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(cache), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{}))
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "model_context_window=272000") ||
			!strings.Contains(joined, "model_auto_compact_token_limit=244800") {
			t.Fatalf("non-OpenAI config provider explicitness = %#v", got)
		}
	})
}

func TestPingOpenAIContextPolicyMatchesProdex04351(t *testing.T) {
	for _, model := range []string{"GPT-5.6-SOL", " gpt-5-mini ", "gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna", "codex-auto-review"} {
		if !pingOpenAILargeContextModel(model) {
			t.Errorf("large-context model rejected: %q", model)
		}
	}
	for _, model := range []string{"gpt-4o", "gpt-6.2-sol", "codex-auto"} {
		if pingOpenAILargeContextModel(model) {
			t.Errorf("non-large-context model accepted: %q", model)
		}
	}
	for _, model := range []string{"gpt-5.6-sol", " GPT-5.6-TERRA ", "gpt-5.6-luna", "gpt-6-astra", "gpt-6.1-sol", "gpt-6-sol", "gpt-6-luna"} {
		if !pingOpenAIPreferMaxContextModel(model) {
			t.Errorf("max-context model rejected: %q", model)
		}
	}
	for _, model := range []string{"gpt-5.5", "gpt-5.4", "gpt-5.3-codex"} {
		if pingOpenAIPreferMaxContextModel(model) {
			t.Errorf("non-max-context model accepted: %q", model)
		}
	}
}

func TestPingModelContextEnforcesProdexConfigBound(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "config.toml")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(pingCodexConfigMaxBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := pingModelContextArguments(home, pingArguments(pingmodel.Options{Model: "gpt-5.4"})); err == nil ||
		!strings.Contains(err.Error(), "1048576") {
		t.Fatalf("oversized config error = %v", err)
	}
}
