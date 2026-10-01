package copilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeAuthPrefersDirectOAuthModels(t *testing.T) {
	var legacyCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models":
			if request.Header.Get("Authorization") != "Bearer oauth-fixture" || request.Header.Get("Copilot-Integration-Id") != runtimeIntegrationID || request.Header.Get("x-github-api-version") != runtimeAPIVersion || request.Header.Get("User-Agent") != runtimeUserAgent {
				t.Fatalf("direct headers = %#v", request.Header)
			}
			_, _ = writer.Write([]byte(`{"data":[{"id":"gpt-5.3-codex"},{"id":"account/custom-model","name":"Account Custom","max_prompt_tokens":345678,"capabilities":{"limits":{"max_context_window_tokens":400000},"vision":true}}]}`))
		case "/copilot_internal/v2/token":
			legacyCalls++
			http.Error(writer, "unexpected", http.StatusInternalServerError)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	auth, err := refreshRuntimeAuth(context.Background(), server.Client(), server.URL+"/copilot_internal/v2/token", server.URL+"/models", "oauth-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if auth.apiKey != "oauth-fixture" || legacyCalls != 0 {
		t.Fatalf("auth/legacy = %#v / %d", auth, legacyCalls)
	}
	ids := auth.modelIDs()
	if len(ids) != 27 || ids[0] != "gpt-5.6-luna" || ids[25] != "gpt-4o" || ids[26] != "account/custom-model" {
		t.Fatalf("model ids = %#v", ids)
	}
	custom := auth.ModelCatalog()[26]
	if custom["display_name"] != "Account Custom" || custom["context_window"] != uint64(345678) || custom["max_prompt_tokens"] != uint64(345678) {
		t.Fatalf("custom catalog entry = %#v", custom)
	}
	capabilities, ok := custom["capabilities"].(map[string]any)
	if !ok || capabilities["vision"] != true {
		t.Fatalf("custom capabilities = %#v", custom["capabilities"])
	}
}

func TestRuntimeAuthFallsBackToLegacyExchange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/models":
			http.Error(writer, "unavailable", http.StatusServiceUnavailable)
		case "/copilot_internal/v2/token":
			if request.Header.Get("Authorization") != "token oauth-fixture" || request.Header.Get("Editor-Version") != "vscode/1.85.1" || request.Header.Get("Editor-Plugin-Version") != "copilot/1.155.0" || request.Header.Get("User-Agent") != "GithubCopilot/1.155.0" {
				t.Fatalf("legacy headers = %#v", request.Header)
			}
			_, _ = writer.Write([]byte(`{"token":"runtime-fixture","models":[{"id":"legacy/custom-model","context_window":222222}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	auth, err := refreshRuntimeAuth(context.Background(), server.Client(), server.URL+"/copilot_internal/v2/token", server.URL+"/models", "oauth-fixture")
	if err != nil || auth.apiKey != "runtime-fixture" {
		t.Fatalf("auth = %#v, err = %v", auth, err)
	}
	ids := auth.modelIDs()
	if len(ids) != 27 || ids[26] != "legacy/custom-model" {
		t.Fatalf("legacy model ids = %#v", ids)
	}
}

func TestRuntimeAuthUsesOAuthWhenBothOptionalPathsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	auth, err := refreshRuntimeAuth(context.Background(), server.Client(), server.URL+"/token", server.URL+"/models", "oauth-fixture")
	if err != nil || auth.apiKey != "oauth-fixture" {
		t.Fatalf("auth = %#v, err = %v", auth, err)
	}
}

func TestRuntimeAuthDoesNotLeakCredentialInErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "server body oauth-secret-sentinel", http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := refreshRuntimeAuth(context.Background(), server.Client(), server.URL+"/token", server.URL+"/models", "oauth-secret-sentinel")
	if err == nil || strings.Contains(err.Error(), "oauth-secret-sentinel") || strings.Contains(err.Error(), "server body") {
		t.Fatalf("unsafe runtime auth error = %v", err)
	}
}
