package codex

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

func TestProdex04356RefreshUnauthorizedAuthUsesExactOAuthContract(t *testing.T) {
	home := t.TempDir()
	authPath := filepath.Join(home, "auth.json")
	if err := os.WriteFile(authPath, []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"old-access","refresh_token":"old-refresh","account_id":"account-a"},"future":{"keep":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var requestBody map[string]string
	var originator string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		originator = request.Header.Get("originator")
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(body, &requestBody); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"new-access","refresh_token":"new-refresh"}`)
	}))
	defer server.Close()
	t.Setenv(refreshTokenOverrideEnv, server.URL)

	process := NewCodexProcess("", Terminal{})
	got, err := process.RefreshUnauthorizedAuth(t.Context(), home, proxymodel.Auth{
		AccessToken: "old-access", AccountID: "account-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "new-access" || got.AccountID != "account-a" {
		t.Fatalf("refreshed auth = %#v", got)
	}
	if chatGPTAuthRefreshURL != "https://auth.openai.com/oauth/token" ||
		refreshTokenOverrideEnv != "CODEX_REFRESH_TOKEN_URL_OVERRIDE" ||
		requestBody["client_id"] != "app_EMoamEEZ73f0CkXaXp7hrann" ||
		requestBody["grant_type"] != "refresh_token" ||
		requestBody["refresh_token"] != "old-refresh" ||
		originator != "codex_cli_rs" {
		t.Fatalf("refresh request = body %#v originator %q", requestBody, originator)
	}

	content, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatal(err)
	}
	tokens, _ := stored["tokens"].(map[string]any)
	future, _ := stored["future"].(map[string]any)
	if tokens["access_token"] != "new-access" ||
		tokens["refresh_token"] != "new-refresh" ||
		future["keep"] != true ||
		strings.TrimSpace(stored["last_refresh"].(string)) == "" {
		t.Fatalf("stored refreshed auth = %#v", stored)
	}
	info, err := os.Stat(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("refreshed auth mode = %o, want private", info.Mode().Perm())
	}
}

func TestProdex04356RefreshUnauthorizedAuthCASPreservesNewerCredential(t *testing.T) {
	home := t.TempDir()
	authPath := filepath.Join(home, "auth.json")
	initial := `{"auth_mode":"chatgpt","tokens":{"access_token":"old-access","refresh_token":"old-refresh","account_id":"account-a"}}`
	if err := os.WriteFile(authPath, []byte(initial), 0o600); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		newer := `{"auth_mode":"chatgpt","tokens":{"access_token":"concurrent-access","refresh_token":"concurrent-refresh","account_id":"account-a"}}`
		if err := os.WriteFile(authPath, []byte(newer), 0o600); err != nil {
			t.Error(err)
			writer.WriteHeader(http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"access_token":"stale-server-access","refresh_token":"stale-server-refresh"}`)
	}))
	defer server.Close()
	t.Setenv(refreshTokenOverrideEnv, server.URL)

	process := NewCodexProcess("", Terminal{})
	got, err := process.RefreshUnauthorizedAuth(t.Context(), home, proxymodel.Auth{
		AccessToken: "old-access", AccountID: "account-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != "concurrent-access" {
		t.Fatalf("CAS returned auth = %#v", got)
	}
	content, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "stale-server") ||
		!strings.Contains(string(content), "concurrent-access") ||
		!strings.Contains(string(content), "concurrent-refresh") {
		t.Fatalf("CAS auth content = %s", content)
	}
}
