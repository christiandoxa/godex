package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestCopilotQuotaExternalMatchesProdexDisplayPolicy(t *testing.T) {
	login, plan, access, reset := "resolved-login", "business", "enterprise", "2026-11-01"
	info := userInfo{
		Login: &login, CopilotPlan: &plan, AccessTypeSKU: &access,
		LimitedUserQuotas:    map[string]int64{"chat": 8, "completions": 0},
		MonthlyQuotas:        map[string]int64{"chat": 10, "completions": 20},
		LimitedUserResetDate: &reset,
	}
	quota := copilotQuotaExternal(info, "config-login")
	if quota.Provider != "GitHub Copilot" || quota.Account != login || quota.Plan != plan || quota.Status != "Blocked" || quota.Main != "chat 8/10 | comp 0/20" || quota.Reset != "monthly 2026-11-01" || quota.RemainingPercent == nil || *quota.RemainingPercent != 0 || quota.Available == nil || *quota.Available {
		t.Fatalf("quota = %#v", quota)
	}
	want := map[string]string{"Access": "enterprise", "Remaining": "0%"}
	for _, detail := range quota.Details {
		if expected, ok := want[detail.Label]; ok {
			if detail.Value != expected {
				t.Fatalf("detail = %#v, want %q", detail, expected)
			}
			delete(want, detail.Label)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing details = %#v; got %#v", want, quota.Details)
	}
}

func TestCopilotQuotaExternalDefaultsReadyAndUsesAccessPlan(t *testing.T) {
	access := "individual"
	quota := copilotQuotaExternal(userInfo{AccessTypeSKU: &access}, "fallback-login")
	if quota.Account != "fallback-login" || quota.Plan != access || quota.Status != "Ready" || quota.Main != "-" || quota.Reset != "" || quota.Available == nil || !*quota.Available {
		t.Fatalf("quota = %#v", quota)
	}
}

func TestCopilotProfileQuotaResolvesExactAccountAndNeverReturnsCredential(t *testing.T) {
	const credential = "<redacted>"
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		_, _ = writer.Write([]byte(`{"login":"resolved-login","copilot_plan":"pro","limited_user_quotas":{"chat":5,"completions":7},"monthly_quotas":{"chat":10,"completions":10},"limited_user_reset_date":"2026-12-01"}`))
	}))
	defer server.Close()

	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: "config-login"},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: "config-login"}},
		configTokensField:      map[string]any{server.URL + ":config-login": credential},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	host, login := server.URL, "config-login"
	quota, err := source.FetchQuota(context.Background(), profilemodel.QuotaTarget{
		Provider:       "copilot",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login},
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer "+credential {
		t.Fatalf("authorization was not resolved for the requested Copilot account")
	}
	if quota.Account != "resolved-login" || quota.Plan != "pro" || quota.Status != "Ready" || quota.Main != "chat 5/10 | comp 7/10" || quota.Reset != "monthly 2026-12-01" || quota.Available == nil || !*quota.Available {
		t.Fatalf("quota = %#v", quota)
	}
	for _, detail := range quota.Details {
		if detail.Value == credential {
			t.Fatal("Copilot credential leaked into quota details")
		}
	}
}

func TestCopilotProfileQuotaRawUsesProdexFourMiBBound(t *testing.T) {
	const credential = "fixture-token"
	padding := strings.Repeat("x", (1<<20)+1024)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"login":"raw-login","padding":"` + padding + `"}`))
	}))
	defer server.Close()

	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: "config-login"},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: "config-login"}},
		configTokensField:      map[string]any{server.URL + ":config-login": credential},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	host, login := server.URL, "config-login"
	body, err := source.FetchQuotaRaw(context.Background(), profilemodel.QuotaTarget{
		Provider: "copilot", ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(body) <= 1<<20 || !bytes.Contains(body, []byte(`"login":"raw-login"`)) {
		t.Fatalf("raw Copilot body size/content = %d", len(body))
	}
}

func TestCopilotProfileQuotaRequiresHostAndLogin(t *testing.T) {
	source := NewSource(nil)
	for _, target := range []profilemodel.QuotaTarget{
		{Provider: "copilot"},
		{Provider: "copilot", ProviderConfig: profilemodel.ProviderSnapshot{Host: stringPointer("github.com")}},
	} {
		if _, err := source.FetchQuota(context.Background(), target); err == nil {
			t.Fatalf("incomplete target %#v unexpectedly accepted", target)
		}
	}
}

func TestCopilotQuotaResetMatchesProdexSummaryAndLocalEpoch(t *testing.T) {
	reset := " 2026-11-01 "
	summary, epoch := copilotQuotaReset(&reset)
	if summary != "monthly 2026-11-01" || epoch == nil {
		t.Fatalf("reset = %q / %#v", summary, epoch)
	}
	want := time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local).Unix()
	if *epoch != want {
		t.Fatalf("reset epoch = %d, want %d", *epoch, want)
	}
	empty := "   "
	summary, epoch = copilotQuotaReset(&empty)
	if summary != "monthly " || epoch != nil {
		t.Fatalf("empty reset = %q / %#v", summary, epoch)
	}
	if summary, epoch := copilotQuotaReset(nil); summary != "" || epoch != nil {
		t.Fatalf("nil reset = %q / %#v", summary, epoch)
	}
}

func TestCopilotProfileQuotaRawPreservesUserInfoJSONValue(t *testing.T) {
	const credential = "fixture-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+credential {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = writer.Write([]byte(`{"login":"raw-login","copilot_plan":"pro","unknown":{"big":9007199254740993}}`))
	}))
	defer server.Close()

	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: "config-login"},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: "config-login"}},
		configTokensField:      map[string]any{server.URL + ":config-login": credential},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	host, login := server.URL, "config-login"
	body, err := source.FetchQuotaRaw(context.Background(), profilemodel.QuotaTarget{
		Provider: "copilot", ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login},
	})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value["login"] != "raw-login" || value["unknown"].(map[string]any)["big"].(json.Number).String() != "9007199254740993" {
		t.Fatalf("raw Copilot user info = %#v", value)
	}
	if strings.Contains(string(body), credential) {
		t.Fatal("raw Copilot quota leaked credential")
	}
}

func TestCopilotProfileQuotaRawDoesNotHTMLEscapeJSONValues(t *testing.T) {
	const credential = "fixture-token"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"login":"<raw>&user"}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: "config-login"},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: "config-login"}},
		configTokensField:      map[string]any{server.URL + ":config-login": credential},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	host, login := server.URL, "config-login"
	body, err := source.FetchQuotaRaw(context.Background(), profilemodel.QuotaTarget{
		Provider: "copilot", ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login},
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `{"login":"<raw>&user"}` {
		t.Fatalf("raw Copilot HTML escaping = %s", body)
	}
}

func TestCopilotProfileQuotaRawMatchesProdexUserAgentAndHTTPErrorDetail(t *testing.T) {
	const credential = "fixture-token"
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userAgent = request.Header.Get("User-Agent")
		writer.WriteHeader(http.StatusForbidden)
		_, _ = writer.Write([]byte(`{"message":"denied","code":403}`))
	}))
	defer server.Close()
	configDir := t.TempDir()
	writeCopilotConfig(t, configDir, map[string]any{
		configLastUserField:    map[string]any{"host": server.URL, configLoginField: "config-login"},
		configLoggedUsersField: []any{map[string]any{"host": server.URL, configLoginField: "config-login"}},
		configTokensField:      map[string]any{server.URL + ":config-login": credential},
	})
	source := NewSource(server.Client())
	source.getenv = func(key string) string {
		if key == testCopilotHomeEnv {
			return configDir
		}
		return ""
	}
	host, login := server.URL, "config-login"
	_, err := source.FetchQuotaRaw(context.Background(), profilemodel.QuotaTarget{
		Provider: "copilot", ProviderConfig: profilemodel.ProviderSnapshot{Kind: "copilot", Host: &host, Login: &login},
	})
	if userAgent != "godex/0.435.1" {
		t.Fatalf("User-Agent = %q", userAgent)
	}
	if err == nil || !strings.Contains(err.Error(), "Copilot account query failed (HTTP 403) at "+server.URL+"/copilot_internal/user") ||
		!strings.Contains(err.Error(), "\n  \"code\": 403") || !strings.Contains(err.Error(), "\n  \"message\": \"denied\"") {
		t.Fatalf("Copilot raw HTTP error = %v", err)
	}
}
