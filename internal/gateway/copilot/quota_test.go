package copilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

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
	if quota.Provider != "GitHub Copilot" || quota.Account != login || quota.Plan != plan || quota.Status != "Blocked" || quota.Main != "chat 8/10 | comp 0/20" || quota.Reset != "monthly 2026-11-01" || quota.Available == nil || *quota.Available {
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
