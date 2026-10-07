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

func TestInspectQuotaAuthPrefersNonOpenAIModelProviderOverAuthJSON(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = 'amazon-bedrock'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"<redacted>"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewCodexProcess("", Terminal{}).InspectQuotaAuth(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "model-provider:amazon-bedrock" || got.Compatible {
		t.Fatalf("summary = %+v", got)
	}
}

func TestInspectQuotaAuthReportsConfigErrorBeforeAuthJSON(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"<redacted>"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewCodexProcess("", Terminal{}).InspectQuotaAuth(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "config-error" || got.Compatible {
		t.Fatalf("summary = %+v", got)
	}
}

func TestInspectQuotaAuthKeepsOpenAIProviderAuthSummary(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte("model_provider = 'OPENAI'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"<redacted>"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NewCodexProcess("", Terminal{}).InspectQuotaAuth(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if got.Label != "chatgpt" || !got.Compatible {
		t.Fatalf("summary = %+v", got)
	}
}

func TestProdex04358QuotaAuthSummaryLabelPolicy(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		wantLabel  string
		compatible bool
	}{
		{name: "mode-only chatgpt", content: `{"auth_mode":" chat-gpt "}`, wantLabel: "chatgpt", compatible: true},
		{name: "bedrock mode", content: `{"auth_mode":" Bedrock_API-Key "}`, wantLabel: "bedrock-api-key"},
		{name: "bedrock credential", content: `{"auth_mode":"custom","bedrock_api_key":{"api_key":" key "}}`, wantLabel: "bedrock-api-key"},
		{name: "chatgpt token wins", content: `{"auth_mode":"api-key","OPENAI_API_KEY":"key","tokens":{"access_token":" token "}}`, wantLabel: "chatgpt", compatible: true},
		{name: "bedrock wins api key", content: `{"auth_mode":"api-key","OPENAI_API_KEY":"key","bedrock_api_key":{"api_key":"bedrock"}}`, wantLabel: "bedrock-api-key"},
		{name: "missing mode with auth object", content: `{}`, wantLabel: "auth-present"},
		{name: "custom preserves spelling", content: `{"auth_mode":" Custom_Mode "}`, wantLabel: " Custom_Mode "},
		{name: "empty mode is present", content: `{"auth_mode":""}`, wantLabel: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := summarizeQuotaAuth([]byte(test.content))
			if got.Label != test.wantLabel || got.Compatible != test.compatible {
				t.Fatalf("summary = %+v, want label=%q compatible=%t", got, test.wantLabel, test.compatible)
			}
		})
	}
}
