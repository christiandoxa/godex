package codex

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectModelProviderMatchesCodexConfigQuotaBehavior(t *testing.T) {
	process := NewCodexProcess("", Terminal{})
	for _, test := range []struct {
		name   string
		config string
		wantID string
	}{
		{name: "missing config"},
		{name: "openai", config: "model_provider = \"OPENAI\"\n"},
		{name: "openai with surrounding spaces is a distinct provider", config: "model_provider = ' openai '\n", wantID: " openai "},
		{name: "blank provider", config: "model_provider = '  '\n"},
		{name: "custom", config: "model_provider = \"prodex-deepseek\"\n[model_providers.prodex-deepseek]\nname = \"DeepSeek\"\n", wantID: "prodex-deepseek"},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			if test.config != "" {
				if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(test.config), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			setting, err := process.InspectModelProvider(context.Background(), home)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantID == "" {
				if setting != nil {
					t.Fatalf("setting = %#v, want none", setting)
				}
				return
			}
			if setting == nil || setting.ProviderID != test.wantID || setting.Source != "config.toml" {
				t.Fatalf("setting = %#v, want %q from config.toml", setting, test.wantID)
			}
		})
	}
}

func TestInspectModelProviderRejectsUnsafeAndInvalidConfig(t *testing.T) {
	process := NewCodexProcess("", Terminal{})
	for _, test := range []struct {
		name   string
		config string
	}{
		{name: "malformed TOML", config: "model_provider = ["},
		{name: "oversized", config: "#" + strings.Repeat("x", maxCodexConfigBytes)},
	} {
		t.Run(test.name, func(t *testing.T) {
			home := t.TempDir()
			if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(test.config), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := process.InspectModelProvider(context.Background(), home); err == nil {
				t.Fatal("invalid config unexpectedly accepted")
			}
		})
	}
	if _, err := process.InspectModelProvider(context.Background(), "."); err == nil {
		t.Fatal("relative Codex home unexpectedly accepted")
	}
}

func TestInspectModelProviderTreatsNonStringValueAsUnset(t *testing.T) {
	process := NewCodexProcess("", Terminal{})
	for _, config := range []string{
		"model_provider = 42\n",
		"model_provider = true\n",
		"model_provider = []\n",
	} {
		home := t.TempDir()
		if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
		if setting, err := process.InspectModelProvider(context.Background(), home); err != nil || setting != nil {
			t.Fatalf("config %q setting = %#v, error = %v; want unset provider", config, setting, err)
		}
	}
}

func TestInspectModelProviderFollowsConfigSymlink(t *testing.T) {
	process := NewCodexProcess("", Terminal{})
	home := t.TempDir()
	config := filepath.Join(t.TempDir(), "shared-config.toml")
	if err := os.WriteFile(config, []byte("model_provider = 'prodex-deepseek'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(config, filepath.Join(home, "config.toml")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	setting, err := process.InspectModelProvider(context.Background(), home)
	if err != nil || setting == nil || setting.ProviderID != "prodex-deepseek" {
		t.Fatalf("symlink provider = %#v, err = %v", setting, err)
	}
}
