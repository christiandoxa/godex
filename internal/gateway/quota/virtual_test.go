package quota

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestVirtualDeepSeekMatchesEnvAndBalanceContract(t *testing.T) {
	var authHeaders []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authHeaders = append(authHeaders, request.Header.Get("Authorization"))
		if request.URL.Path != "/user/balance" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"is_available":true,"balance_infos":[{"currency":"USD","total_balance":"12.34","granted_balance":"10","topped_up_balance":"2.34"},{"currency":"CNY","total_balance":"88","granted_balance":"-","topped_up_balance":"5"}]}`))
	}))
	defer server.Close()
	virtual := NewVirtual(server.Client())
	env := map[string]string{
		"DEEPSEEK_API_KEYS":        " first ,second; third\nfourth ",
		"DEEPSEEK_API_KEY":         "single-ignored",
		"PRODEX_DEEPSEEK_BASE_URL": server.URL + "/v1/",
	}
	virtual.getenv = func(name string) string { return env[name] }
	results := virtual.Collect(context.Background(), "deepseek", "")
	if len(results) != 4 {
		t.Fatalf("results = %#v", results)
	}
	for index, result := range results {
		wantName := "deepseek"
		if index > 0 {
			wantName = "deepseek-" + string(rune('1'+index))
		}
		if result.Name != wantName || result.Provider != "deepseek" || result.Auth != "deepseek-key" || result.Err != nil || result.External == nil {
			t.Fatalf("result %d = %#v", index, result)
		}
		info := result.External
		if info.Provider != "DeepSeek" || info.Plan != "api-key" || info.Status != "Ready" || info.Main != "USD 12.34 | CNY 88" || info.Available == nil || !*info.Available || len(info.Details) != 2 {
			t.Fatalf("info %d = %#v", index, info)
		}
	}
	wantHeaders := []string{"Bearer first", "Bearer second", "Bearer third", "Bearer fourth"}
	if strings.Join(authHeaders, ",") != strings.Join(wantHeaders, ",") {
		t.Fatalf("authorization headers = %#v", authHeaders)
	}
}

func TestVirtualDeepSeekEmptyPluralFallsBackToSingle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer single" {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		_, _ = writer.Write([]byte(`{"is_available":false,"balance_infos":[]}`))
	}))
	defer server.Close()
	virtual := NewVirtual(server.Client())
	env := map[string]string{"DEEPSEEK_API_KEYS": " , ; \n ", "DEEPSEEK_API_KEY": "single"}
	virtual.getenv = func(name string) string {
		if name == "PRODEX_DEEPSEEK_BASE_URL" {
			return server.URL
		}
		return env[name]
	}
	results := virtual.Collect(context.Background(), "deepseek", "")
	if len(results) != 1 || results[0].Err != nil || results[0].External == nil || results[0].External.Status != "Blocked" || results[0].External.Main != "balance unavailable" {
		t.Fatalf("results = %#v", results)
	}
}

func TestVirtualDeepSeekMissingKeyAndExplicitBaseURL(t *testing.T) {
	virtual := NewVirtual(nil)
	virtual.getenv = func(string) string { return "" }
	results := virtual.Collect(context.Background(), "deepseek", "https://quota.invalid/v1")
	if len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "DEEPSEEK_API_KEY") {
		t.Fatalf("missing-key results = %#v", results)
	}
}

func TestVirtualLocalModelsURLAndAPIKeyPrecedence(t *testing.T) {
	var paths []string
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		paths = append(paths, request.URL.Path)
		auth = append(auth, request.Header.Get("Authorization"))
		_, _ = writer.Write([]byte(`{"data":[{"id":"one"},{"id":"two"}]}`))
	}))
	defer server.Close()
	virtual := NewVirtual(server.Client())
	virtual.getenv = func(name string) string {
		switch name {
		case "PRODEX_LOCAL_API_KEY":
			return "prodex-local"
		case "OPENAI_API_KEY":
			return "openai-fallback"
		default:
			return ""
		}
	}
	for _, base := range []string{server.URL, server.URL + "/v1", server.URL + "/v1/models"} {
		result := virtual.Collect(context.Background(), "local", base)
		if len(result) != 1 || result[0].Err != nil || result[0].External == nil {
			t.Fatalf("base %q result = %#v", base, result)
		}
		info := result[0].External
		if info.Provider != "Local OpenAI-compatible" || info.Account != base || info.Plan != "local" || info.Status != "Reachable" || info.Main != "2 model(s)" || len(info.Details) != 1 || !strings.HasSuffix(info.Details[0].Value, "/models") {
			t.Fatalf("base %q info = %#v", base, info)
		}
	}
	if strings.Join(paths, ",") != "/v1/models,/v1/models,/v1/models" {
		t.Fatalf("paths = %#v", paths)
	}
	for _, header := range auth {
		if header != "Bearer prodex-local" {
			t.Fatalf("authorization = %#v", auth)
		}
	}
}

func TestVirtualLocalRequiresBaseURLAndDoesNotRunForAll(t *testing.T) {
	virtual := NewVirtual(nil)
	if got := virtual.Collect(context.Background(), "all", ""); got != nil {
		t.Fatalf("all filter virtual reports = %#v", got)
	}
	results := virtual.Collect(context.Background(), "local", "")
	if len(results) != 1 || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "requires --base-url") {
		t.Fatalf("local missing base results = %#v", results)
	}
}

func TestLocalModelsURLRejectsCredentialsAndQuery(t *testing.T) {
	for _, value := range []string{"file:///tmp/models", "https://user:pass@example.test", "https://example.test?token=secret"} {
		if _, err := localModelsURL(value); err == nil {
			t.Fatalf("unsafe URL %q accepted", value)
		}
	}
}

func TestVirtualAgyMatchesProdexCommandAndQuotaShape(t *testing.T) {
	virtual := NewVirtual(nil)
	var binary string
	var arguments []string
	virtual.run = func(_ context.Context, gotBinary string, gotArguments []string) (commandResult, error) {
		binary = gotBinary
		arguments = append([]string(nil), gotArguments...)
		return commandResult{stdout: []byte(`[
  {
    "account":"person@example.test",
    "plan":"pro",
    "status":"Ready",
    "credits":12.5,
    "details":{"daily":"80%","weekly":"60%"},
    "models":["model-a","model-b"],
    "region":"us-east-1"
  }
]`)}, nil
	}
	results := virtual.Collect(context.Background(), "agy", "")
	if binary != "agy" || strings.Join(arguments, " ") != "auth quota --format=json --detail --all-accounts" {
		t.Fatalf("command = %q %#v", binary, arguments)
	}
	if len(results) != 1 || results[0].Name != "agy:person@example.test" || results[0].Provider != "agy" || results[0].Auth != "agy" || results[0].Err != nil || results[0].External == nil {
		t.Fatalf("results = %#v", results)
	}
	info := results[0].External
	if info.Provider != "Anti-Gravity" || info.Account != "person@example.test" || info.Plan != "pro" || info.Status != "Ready" || info.Main != "12.50 credits available" || info.Available == nil || !*info.Available {
		t.Fatalf("info = %#v", info)
	}
	wantDetails := map[string]string{
		"details:daily":  "80%",
		"details:weekly": "60%",
		"models[0]":      "model-a",
		"models[1]":      "model-b",
		"region":         "us-east-1",
	}
	if len(info.Details) != len(wantDetails) {
		t.Fatalf("details = %#v", info.Details)
	}
	for _, detail := range info.Details {
		if wantDetails[detail.Label] != detail.Value {
			t.Fatalf("detail = %#v; all = %#v", detail, info.Details)
		}
	}
}

func TestVirtualAgySupportsObjectAndUsageFallback(t *testing.T) {
	virtual := NewVirtual(nil)
	virtual.run = func(context.Context, string, []string) (commandResult, error) {
		return commandResult{stdout: []byte(`{"email":"person@example.test","usage":7.25}`)}, nil
	}
	results := virtual.Collect(context.Background(), "agy", "")
	if len(results) != 1 || results[0].External == nil || results[0].External.Main != "7.25 usage" || results[0].Name != "agy:person@example.test" {
		t.Fatalf("results = %#v", results)
	}
}

func TestVirtualAgyFailureAndAllFilterRemainBounded(t *testing.T) {
	virtual := NewVirtual(nil)
	calls := 0
	virtual.run = func(context.Context, string, []string) (commandResult, error) {
		calls++
		return commandResult{stderr: []byte("secret-output-must-not-be-surfaced"), exitCode: 2}, nil
	}
	if results := virtual.Collect(context.Background(), "all", ""); results != nil || calls != 0 {
		t.Fatalf("all results/calls = %#v / %d", results, calls)
	}
	results := virtual.Collect(context.Background(), "agy", "")
	if len(results) != 1 || results[0].Err == nil || strings.Contains(results[0].Err.Error(), "secret-output") || calls != 1 {
		t.Fatalf("agy failure = %#v, calls=%d", results, calls)
	}
}

func TestProfileAgyQuotaUsesPreferredAccountWithoutAllAccounts(t *testing.T) {
	virtual := NewVirtual(nil)
	var arguments []string
	virtual.run = func(_ context.Context, _ string, got []string) (commandResult, error) {
		arguments = append([]string(nil), got...)
		return commandResult{stdout: []byte(`[{"account":"first@example.test","plan":"free","credits":1},{"account":"preferred@example.test","plan":"pro","credits":9.5}]`)}, nil
	}
	preferred := "preferred@example.test"
	info, err := virtual.FetchQuota(context.Background(), profilemodel.QuotaTarget{
		Provider:       "agy",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "agy", Account: &preferred},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(arguments, " "), "--all-accounts") {
		t.Fatalf("profile AGY command unexpectedly used all-accounts: %#v", arguments)
	}
	if strings.Join(arguments, " ") != "auth quota --format=json --detail" {
		t.Fatalf("profile AGY arguments = %#v", arguments)
	}
	if info.Account != preferred || info.Plan != "pro" || info.Main != "9.50 credits available" {
		t.Fatalf("preferred AGY info = %#v", info)
	}
}

func TestProfileAgyQuotaFallsBackToFirstAccountWhenPreferredMissing(t *testing.T) {
	virtual := NewVirtual(nil)
	virtual.run = func(context.Context, string, []string) (commandResult, error) {
		return commandResult{stdout: []byte(`[{"account":"first@example.test","credits":2},{"account":"second@example.test","credits":3}]`)}, nil
	}
	missing := "missing@example.test"
	info, err := virtual.FetchQuota(context.Background(), profilemodel.QuotaTarget{
		Provider:       "agy",
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "agy", Account: &missing},
	})
	if err != nil {
		t.Fatal(err)
	}
	if info.Account != "first@example.test" || info.Main != "2.00 credits available" {
		t.Fatalf("fallback AGY info = %#v", info)
	}
}

func TestProfileAgyQuotaRejectsWrongProvider(t *testing.T) {
	virtual := NewVirtual(nil)
	if _, err := virtual.FetchQuota(context.Background(), profilemodel.QuotaTarget{Provider: "kiro"}); err == nil {
		t.Fatal("non-AGY target unexpectedly accepted by AGY quota adapter")
	}
}
