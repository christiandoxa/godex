package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

const (
	quotaCredentialFixture = "<redacted>"
	quotaEmailFixture      = "person@example.test"
	quotaExpiryFixtureMS   = int64(4102444800000)
)

func TestAnthropicProfileQuotaWithoutAdminKeyUsesOAuthSnapshot(t *testing.T) {
	home := writeClaudeQuotaFixture(t, claudeQuotaFixture(t, true, "pro"))
	source := NewSource()
	source.getenv = func(string) string { return "" }
	info, err := source.FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "Anthropic Claude" || info.Account != quotaEmailFixture || info.Plan != "claude-ai-oauth:pro" || info.Status != "Ready (OAuth)" || info.Main != "rate limits require admin key" || info.Available == nil || !*info.Available {
		t.Fatalf("info = %#v", info)
	}
	if len(info.Details) != 2 || info.Details[0].Label != "OAuth expires" || info.Details[0].Value != time.Unix(quotaExpiryFixtureMS/1000, 0).Local().Format("2006-01-02 15:04:05") || info.Details[1].Label != "Admin API" || !strings.Contains(info.Details[1].Value, "ANTHROPIC_ADMIN_KEY") {
		t.Fatalf("details = %#v", info.Details)
	}
}

func TestAnthropicProfileQuotaUsesAdminRateLimits(t *testing.T) {
	var observedKey string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		observedKey = request.Header.Get("x-api-key")
		if request.URL.Path != "/v1/organizations/rate_limits" || request.Header.Get("anthropic-version") != anthropicAPIVersion || request.Header.Get("accept") != "application/json" {
			t.Fatalf("request = %s headers=%v", request.URL.Path, request.Header)
		}
		_, _ = writer.Write([]byte(`{"data":[{"group_type":"model_group","models":["claude-sonnet-4-6"],"limits":[{"type":"requests","value":100},{"type":"tokens","value":"2000"}]},{"group_type":"organization","limits":[{"type":"requests","value":500}]}]}`))
	}))
	defer server.Close()
	home := writeClaudeQuotaFixture(t, claudeQuotaFixture(t, false, "max"))
	source := NewSource()
	source.getenv = func(name string) string {
		switch name {
		case "ANTHROPIC_ADMIN_KEY":
			return quotaCredentialFixture
		case "PRODEX_ANTHROPIC_ADMIN_BASE_URL":
			return server.URL
		default:
			return ""
		}
	}
	info, err := source.FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if observedKey != quotaCredentialFixture || info.Status != "Ready" || info.Main != "1 model rate group(s)" || info.Plan != "claude-ai-oauth:max" {
		t.Fatalf("key/info = <redacted> / %#v", info)
	}
	want := map[string]string{
		"Rate groups": "2", "Model groups": "1",
		"claude-sonnet-4-6": "requests=100, tokens=2000",
		"organization":      "requests=500",
	}
	for _, detail := range info.Details[1:] {
		if expected, ok := want[detail.Label]; ok {
			if detail.Value != expected {
				t.Fatalf("detail = %#v, want %q", detail, expected)
			}
			delete(want, detail.Label)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing rate-limit details = %#v; got %#v", want, info.Details)
	}
}

func TestAnthropicProfileQuotaAdminFailureDegradesWithoutResponseLeak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte("internal-error-body"))
	}))
	defer server.Close()
	home := writeClaudeQuotaFixture(t, claudeQuotaFixture(t, false, ""))
	source := NewSource()
	source.getenv = func(name string) string {
		if name == "ANTHROPIC_ADMIN_API_KEY" {
			return quotaCredentialFixture
		}
		if name == "ANTHROPIC_BASE_URL" {
			return server.URL
		}
		return ""
	}
	info, err := source.FetchQuota(context.Background(), profilemodel.QuotaTarget{CodexHome: home})
	if err != nil {
		t.Fatal(err)
	}
	if info.Status != "Ready (OAuth)" || info.Main != "rate limits require admin key" {
		t.Fatalf("info = %#v", info)
	}
	last := info.Details[len(info.Details)-1]
	if last.Label != "Admin API" || !strings.Contains(last.Value, "HTTP 403") || strings.Contains(last.Value, "internal-error-body") {
		t.Fatalf("admin failure detail = %#v", last)
	}
}

func TestAnthropicAdminBaseURLRejectsUnsafeValues(t *testing.T) {
	source := NewSource()
	for _, value := range []string{"file:///tmp/api", "https://user:pass@example.test", "https://example.test?value=redacted"} {
		source.getenv = func(name string) string {
			if name == "PRODEX_ANTHROPIC_ADMIN_BASE_URL" {
				return value
			}
			return ""
		}
		if _, err := source.anthropicAdminBaseURL(); err == nil {
			t.Fatalf("unsafe admin URL %q accepted", value)
		}
	}
}

func claudeQuotaFixture(t *testing.T, nested bool, subscription string) string {
	t.Helper()
	token := map[string]any{
		"accessToken": quotaCredentialFixture,
		"expiresAt":   quotaExpiryFixtureMS,
		"email":       quotaEmailFixture,
	}
	if subscription != "" {
		token["subscriptionType"] = subscription
	}
	var value any = token
	if nested {
		value = map[string]any{"claudeAiOauth": token}
	}
	content, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeClaudeQuotaFixture(t *testing.T, content string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, CredentialsFile), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}
