package codex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInspectQuotaAuthClassifiesCodexProfiles(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		directory  bool
		wantLabel  string
		compatible bool
	}{
		{name: "missing", wantLabel: "no-auth"},
		{name: "invalid", content: "not-json", wantLabel: "invalid-auth"},
		{name: "chatgpt", content: `{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic"}}`, wantLabel: "chatgpt", compatible: true},
		{name: "api key", content: `{"auth_mode":"apikey","OPENAI_API_KEY":"synthetic"}`, wantLabel: "api-key"},
		{name: "unreadable shape", directory: true, wantLabel: "unreadable-auth"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertQuotaAuthCase(t, test.content, test.directory, test.wantLabel, test.compatible)
		})
	}
}

func assertQuotaAuthCase(t *testing.T, content string, directory bool, wantLabel string, compatible bool) {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, "auth.json")
	if err := writeQuotaAuthCase(path, content, directory); err != nil {
		t.Fatal(err)
	}
	got, err := NewCodexProcess("", Terminal{}).InspectQuotaAuth(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != wantLabel || got.Compatible != compatible {
		t.Fatalf("summary = %+v", got)
	}
}

func writeQuotaAuthCase(path, content string, directory bool) error {
	if directory {
		return os.Mkdir(path, 0o700)
	}
	if content == "" {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o600)
}
