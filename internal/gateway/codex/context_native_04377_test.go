package codex

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

func TestProdex04377PingPreservesNativeContextConfiguration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell")
	}
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model = \"gpt-6.1-sol\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "models_cache.json"), []byte(`{"models":[{"slug":"gpt-6.1-sol","context_window":272000,"max_context_window":872000}]}`), 0o600); err != nil {
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
		Model: "gpt-6.1-sol",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ping result = %#v", result)
	}
	got, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "model_context_window=") || strings.Contains(string(got), "model_auto_compact_token_limit=") {
		t.Fatalf("ping changed native context configuration: %q", got)
	}
	want := strings.Join(pingArguments(pingmodel.Options{Model: "gpt-6.1-sol"}), "\n") + "\n"
	if string(got) != want {
		t.Fatalf("ping args = %q, want %q", got, want)
	}
}

func TestProdex04377DirectRuntimePreservesContextAndModelSwitching(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper uses a POSIX shell")
	}
	root := t.TempDir()
	home := filepath.Join(root, "codex-home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	profileConfig := []byte("model = \"gpt-6.1-sol\"\nmodel_context_window = 640000\nmodel_auto_compact_token_limit = 520000\n")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), profileConfig, 0o600); err != nil {
		t.Fatal(err)
	}
	recordArgs := filepath.Join(root, "args")
	recordHome := filepath.Join(root, "home")
	script := filepath.Join(root, "codex")
	body := strings.Join([]string{
		"#!/bin/sh",
		"printf '%s\\n' \"$@\" > \"$GODEX_RUNTIME_ARGS\"",
		"printf '%s' \"$CODEX_HOME\" > \"$GODEX_RUNTIME_HOME\"",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	process := NewCodexProcess(script, Terminal{Stdout: io.Discard, Stderr: io.Discard})
	for _, test := range []struct {
		name  string
		model string
		args  []string
	}{
		{name: "sol", model: "gpt-6.1-sol", args: []string{"--model", "gpt-6.1-sol", "-c", "model_context_window=500000", "-c", "model_auto_compact_token_limit=430000", "exec", "hello"}},
		{name: "luna", model: "gpt-6-luna", args: []string{"--model", "gpt-6-luna", "-c", "model_context_window=272000", "-c", "model_auto_compact_token_limit=258400", "exec", "hello"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("GODEX_RUNTIME_ARGS", recordArgs)
			t.Setenv("GODEX_RUNTIME_HOME", recordHome)
			if err := process.RunRuntimeDirect(context.Background(), home, test.args, false); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(recordArgs)
			if err != nil {
				t.Fatal(err)
			}
			want := strings.Join([]string{"--model", test.model, "exec", "-c", test.args[3], "-c", test.args[5], "hello"}, "\n") + "\n"
			if string(got) != want {
				t.Fatalf("child args = %q, want %q", got, want)
			}
			childHome, err := os.ReadFile(recordHome)
			if err != nil {
				t.Fatal(err)
			}
			if string(childHome) != home {
				t.Fatalf("child CODEX_HOME = %q, want %q", childHome, home)
			}
			config, err := os.ReadFile(filepath.Join(home, "config.toml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(config) != string(profileConfig) {
				t.Fatalf("profile config changed while switching to %s", test.model)
			}
		})
	}
}
