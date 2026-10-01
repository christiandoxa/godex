package copilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	fixtureGitHubHost      = "https://github.com"
	fixtureConfigLogin     = "config-login"
	fixtureResolvedLogin   = "resolved-login"
	fixtureSecondLogin     = "second"
	fixtureBusinessPlan    = "business"
	fixtureEnterpriseSKU   = "enterprise"
	configLastUserField    = "lastLoggedInUser"
	configLoggedUsersField = "loggedInUsers"
	configTokensField      = "copilotTokens"
	configLoginField       = "login"
	testCopilotHomeEnv     = "COPILOT_HOME"
)

func TestCopilotSourceLoadsConfigTokenAndUserInfo(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		if request.URL.Path != "/copilot_internal/user" {
			t.Errorf("path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(map[string]any{
			configLoginField:  fixtureResolvedLogin,
			"access_type_sku": fixtureEnterpriseSKU,
			"copilot_plan":    fixtureBusinessPlan,
			"endpoints":       map[string]any{"api": "https://copilot-api.example.test"},
		})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = writer.Write(body)
	}))
	defer server.Close()

	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: fixtureConfigLogin},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: fixtureConfigLogin}},
		configTokensField:      map[string]any{server.URL + ":" + fixtureConfigLogin: "fixture-token"},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}

	credential, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer fixture-token" {
		t.Fatalf("authorization header = %q", authorization)
	}
	if credential.Email != fixtureConfigLogin || len(credential.SecretFiles) != 0 || credential.Warning != "" {
		t.Fatalf("credential = %#v", credential)
	}
	assertCopilotPointer(t, credential.Provider.Host, server.URL)
	assertCopilotPointer(t, credential.Provider.Login, fixtureResolvedLogin)
	assertCopilotPointer(t, credential.Provider.APIURL, "https://copilot-api.example.test")
	assertCopilotPointer(t, credential.Provider.AccessTypeSKU, fixtureEnterpriseSKU)
	assertCopilotPointer(t, credential.Provider.CopilotPlan, fixtureBusinessPlan)
}

func TestCopilotSourceFallsBackAcrossLoggedInUsers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField: map[string]any{"host": "https://first.example.test", configLoginField: "first"},
		configLoggedUsersField: []any{
			map[string]any{"host": "https://first.example.test", configLoginField: "first"},
			map[string]any{"host": server.URL, configLoginField: fixtureSecondLogin},
		},
		configTokensField: map[string]any{server.URL + ":" + fixtureSecondLogin: "second-token"},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	source.run = func(context.Context, string, []string) (commandResult, error) {
		return commandResult{exitCode: 1}, errors.New("backend unavailable")
	}
	credential, err := source.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if credential.Email != fixtureSecondLogin {
		t.Fatalf("credential = %#v", credential)
	}
	assertCopilotPointer(t, credential.Provider.Login, fixtureSecondLogin)
	assertCopilotPointer(t, credential.Provider.APIURL, "https://api."+strings.TrimPrefix(server.URL, "http://"))
}

func TestCopilotConfigSupportsLineCommentsWithoutDamagingURLs(t *testing.T) {
	raw := []byte(fmt.Sprintf("{\n"+
		"  // selected user\n"+
		"  %q: {\"host\": %q, %q: \"octocat\"},\n"+
		"  %q: [{\"host\": %q, %q: \"octocat\"}],\n"+
		"  %q: {%q: \"fixture\"} // token mapping\n"+
		"}",
		configLastUserField, fixtureGitHubHost, configLoginField,
		configLoggedUsersField, fixtureGitHubHost, configLoginField,
		configTokensField, fixtureGitHubHost+":octocat",
	))
	config, err := parseConfig(raw)
	if err != nil {
		t.Fatal(err)
	}
	if config.LastLoggedInUser == nil || config.LastLoggedInUser.Host != fixtureGitHubHost || configToken(config, fixtureGitHubHost, "octocat") != "fixture" {
		t.Fatalf("config = %#v", config)
	}
}

func TestCopilotAPIURLPoliciesMatchProdex(t *testing.T) {
	for host, want := range map[string]string{
		"github.com":               "https://api.githubcopilot.com",
		"https://github.com/":      "https://api.githubcopilot.com",
		"acme.ghe.com":             "https://copilot-api.acme.ghe.com",
		"https://github.acme.test": "https://api.github.acme.test",
	} {
		if got := defaultCopilotAPIURL(host); got != want {
			t.Fatalf("default API for %q = %q, want %q", host, got, want)
		}
	}
	for host, want := range map[string]string{
		"github.com":                    "https://api.github.com",
		"https://github.acme.test":      "https://api.github.acme.test",
		"http://localhost:1234/base":    "http://localhost:1234",
		"https://api.github.example.io": "https://api.github.example.io",
	} {
		got, err := copilotUserAPIOrigin(host)
		if err != nil || got != want {
			t.Fatalf("user API origin for %q = %q, %v; want %q", host, got, err, want)
		}
	}
}

func TestCopilotConfigRejectsSymlink(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink creation may require privileges on Windows")
	}
	realDir := t.TempDir()
	writeCopilotConfig(t, realDir, map[string]any{})
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(realDir, copilotConfigFileName), filepath.Join(root, copilotConfigFileName)); err != nil {
		t.Fatal(err)
	}
	source := NewSource(nil)
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return root
		}
		return ""
	}
	if _, err := source.readConfig(); err == nil {
		t.Fatal("symlinked Copilot config unexpectedly accepted")
	}
}

func writeCopilotConfig(t *testing.T, directory string, value map[string]any) {
	t.Helper()
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, copilotConfigFileName), content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertCopilotPointer(t *testing.T, value *string, want string) {
	t.Helper()
	if value == nil || *value != want {
		t.Fatalf("value = %#v, want %q", value, want)
	}
}
