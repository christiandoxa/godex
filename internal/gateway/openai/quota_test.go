package openai

import (
	"context"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestQuotaClientFetchesManagedChatGPTUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("quota path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer synthetic-token" {
			t.Fatal("missing bearer token")
		}
		if request.Header.Get("ChatGPT-Account-Id") != "account-123" {
			t.Fatalf("account header = %q", request.Header.Get("ChatGPT-Account-Id"))
		}
		if request.Header.Get("originator") != "codex_cli_rs" || request.Header.Get("x-openai-codex-luna-reserve") != "1" {
			t.Fatalf("quota compatibility headers = %#v", request.Header)
		}
		_, _ = writer.Write([]byte(`{
			"plan_type":"plus",
			"rate_limit":{
				"primary_window":{"used_percent":20,"reset_at":200,"limit_window_seconds":18000},
				"secondary_window":{"used_percent":40,"reset_at":300,"limit_window_seconds":604800}
			}
		}`))
	}))
	defer server.Close()

	home := writeQuotaAuth(t, "synthetic-token", "account-123")
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		t.Fatal(err)
	}
	usage, err := client.Fetch(context.Background(), home)
	if err != nil {
		t.Fatal(err)
	}
	if usage.PlanType != "plus" || usage.Primary == nil || usage.Primary.UsedPercent == nil || *usage.Primary.UsedPercent != 20 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestDecodeQuotaUsageAcceptsIndexedCamelCase(t *testing.T) {
	usage, err := decodeQuotaUsage([]byte(`{
		"planType":"team",
		"ordinaryUsageAllowed":false,
		"rateLimitsByLimitId":{
			"codex":{
				"primary":{"usedPercent":100,"windowDurationMins":300},
				"secondary":{"usedPercent":75,"windowDurationMins":10080}
			}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if usage.PlanType != "team" || usage.Allowed == nil || *usage.Allowed || usage.Primary == nil || usage.Primary.LimitWindowSeconds == nil || *usage.Primary.LimitWindowSeconds != 18000 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestQuotaClientDoesNotEchoErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{"secret":"must-not-escape"}`))
	}))
	defer server.Close()
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Fetch(context.Background(), writeQuotaAuth(t, "synthetic-token", "account-123"))
	if err == nil || err.Error() != "quota endpoint returned HTTP 401" {
		t.Fatalf("quota error = %v", err)
	}
}

func writeQuotaAuth(t *testing.T, token, accountID string) string {
	t.Helper()
	home := t.TempDir()
	content := `{"auth_mode":"chatgpt","tokens":{"access_token":"` + token + `","account_id":"` + accountID + `"}}`
	if err := os.WriteFile(filepath.Join(home, "auth.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestQuotaClientFetchRawPreservesValidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"plan_type":"plus","extra":{"future":true}}`))
	}))
	defer server.Close()
	client, err := NewQuotaClient(server.URL+"/backend-api", nil, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.FetchRaw(context.Background(), writeQuotaAuth(t, "synthetic-token", "account-123"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"plan_type":"plus","extra":{"future":true}}` {
		t.Fatalf("raw body = %q", body)
	}
}

func TestQuotaClientFetchAtUsesOverrideBaseURL(t *testing.T) {
	defaultServer := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("default quota endpoint should not be called")
	}))
	defer defaultServer.Close()
	overrideServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/backend-api/wham/usage" {
			t.Fatalf("override quota path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"plan_type":"plus"}`))
	}))
	defer overrideServer.Close()
	client, err := NewQuotaClient(defaultServer.URL+"/backend-api", nil, codex.NewCodexProcess("", codex.Terminal{}))
	if err != nil {
		t.Fatal(err)
	}
	home := writeQuotaAuth(t, "synthetic-token", "account-123")
	usage, err := client.FetchAt(context.Background(), home, overrideServer.URL+"/backend-api")
	if err != nil || usage.PlanType != "plus" {
		t.Fatalf("override usage = %+v, err = %v", usage, err)
	}
	if _, err := client.FetchAt(context.Background(), home, "file:///tmp/quota"); err == nil {
		t.Fatal("non-http quota override unexpectedly accepted")
	}
}
