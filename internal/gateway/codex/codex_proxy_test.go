package codex

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCheckProxySupportAcceptsAndRejectsCapability(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	for _, test := range []struct {
		name    string
		exit    string
		wantErr bool
	}{
		{name: "supported", exit: "0"},
		{name: "unsupported", exit: "1", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := writeProxyHelper(t, `if [ "$#" -eq 1 ] && [ "$1" = "--version" ]; then
  printf 'codex-cli 0.159.2\n'
  exit 0
fi
exit `+test.exit)
			err := NewCodexProcess(script, Terminal{}).CheckProxySupport(context.Background())
			if (err != nil) != test.wantErr {
				t.Fatalf("capability error = %v", err)
			}
			if test.wantErr && !strings.Contains(err.Error(), "upgrade Codex") {
				t.Fatalf("capability error = %v", err)
			}
		})
	}
}

func TestRunThroughProxyPreservesArgumentsAndHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("helper uses a POSIX executable")
	}
	record := filepath.Join(t.TempDir(), "record")
	t.Setenv("GODEX_PROXY_RECORD", record)
	script := writeProxyHelper(t, `printf '%s' "$CODEX_HOME" > "$GODEX_PROXY_RECORD.home"
printf '%s\n' "$@" > "$GODEX_PROXY_RECORD.args"`)
	home := t.TempDir()
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), home, "http://127.0.0.1:1234", []string{"--model", "synthetic"}); err != nil {
		t.Fatal(err)
	}
	childHome, err := os.ReadFile(record + ".home")
	if err != nil || string(childHome) != home {
		t.Fatalf("child home = %q, err = %v", childHome, err)
	}
	arguments, err := os.ReadFile(record + ".args")
	if err != nil || !strings.Contains(string(arguments), `chatgpt_base_url="http://127.0.0.1:1234/backend-api"`) || !strings.Contains(string(arguments), "--model\nsynthetic") {
		t.Fatalf("child arguments = %q, err = %v", arguments, err)
	}
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), home, "http://127.0.0.1:1234", []string{"-c", "openai_base_url=https://outside"}); err == nil {
		t.Fatal("proxy override unexpectedly accepted")
	}
	if err := NewCodexProcess(script, Terminal{}).RunThroughProxy(context.Background(), "/", "http://127.0.0.1:1234", nil); err == nil {
		t.Fatal("proxy with filesystem root unexpectedly accepted")
	}
}

func writeProxyHelper(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
