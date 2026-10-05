package quota

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type fakeAccounts struct {
	accounts []accountentity.Account
	current  accountentity.Account
}

func (fake fakeAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), fake.accounts...), nil
}
func (fake fakeAccounts) Current(context.Context) (accountentity.Account, error) {
	if fake.current.ID == "" {
		return accountentity.Account{}, errors.New("no current")
	}
	return fake.current, nil
}
func (fake fakeAccounts) Resolve(_ context.Context, selector string) (accountentity.Account, error) {
	for _, account := range fake.accounts {
		if account.Name == selector {
			return account, nil
		}
	}
	return accountentity.Account{}, errors.New("not found")
}
func (fake fakeAccounts) CodexHome(id string) string { return "/managed/" + id }

type fakeUsage struct{ byHome map[string]quotamodel.Usage }

func (fake fakeUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	usage, ok := fake.byHome[home]
	if !ok {
		return quotamodel.Usage{}, errors.New("probe failed")
	}
	return usage, nil
}

func TestStatusReportsAllAccountsAndQuotaState(t *testing.T) {
	used100 := int64(100)
	future := time.Unix(1000, 0).Unix()
	first := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	second := accountentity.Account{ID: "two", Name: "two", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{first, second}, current: first}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {PlanType: "plus"},
		"/managed/two": {Primary: &quotamodel.Window{UsedPercent: &used100, ResetAt: &future}},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	reports, err := status.Run(context.Background(), Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 || reports[0].State != "ready" || !reports[0].Active || reports[1].State != "exhausted" {
		t.Fatalf("reports = %+v", reports)
	}
}

func TestStatusDefaultsToCurrentAccount(t *testing.T) {
	current := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{current}, current: current}, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/one": {PlanType: "plus"}}})
	reports, err := status.Run(context.Background(), Options{})
	if err != nil || len(reports) != 1 || reports[0].AccountName != "one" {
		t.Fatalf("reports = %+v, err = %v", reports, err)
	}
}

type countingUsage struct{ calls int }

func (usage *countingUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	usage.calls++
	return quotamodel.Usage{}, nil
}

func TestStatusDoesNotProbeDisabledAccounts(t *testing.T) {
	disabled := accountentity.Account{ID: "off", Name: "off", Enabled: false}
	usage := &countingUsage{}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{disabled}, current: disabled}, usage)
	reports, err := status.Run(context.Background(), Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if usage.calls != 0 || len(reports) != 1 || reports[0].State != "disabled" {
		t.Fatalf("disabled quota reports = %+v, calls = %d", reports, usage.calls)
	}
}

func TestStatusReadyUsesQuotaPolicy(t *testing.T) {
	used := int64(100)
	reset := time.Unix(1000, 0).Unix()
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	ready, err := status.Ready(context.Background(), account)
	if err != nil || ready {
		t.Fatalf("ready = %t, err = %v", ready, err)
	}
}

func TestAvailabilityTreatsOnePercentRemainingAsUsable(t *testing.T) {
	used := int64(99)
	reset := time.Unix(1000, 0).Unix()
	account := accountentity.Account{ID: "one", Enabled: true}
	status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	availability, err := status.Availability(context.Background(), account)
	if err != nil || !availability.Ready {
		t.Fatalf("one-percent quota availability = %+v, err = %v", availability, err)
	}
}

func TestAvailabilityUsesObservedResetAndFiniteUnknownRetry(t *testing.T) {
	now := time.Unix(100, 0)
	reset := now.Add(15 * time.Second).Unix()
	used := int64(100)
	account := accountentity.Account{ID: "one", Enabled: true}
	for _, test := range []struct {
		usage    quotamodel.Usage
		deadline time.Time
	}{
		{quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}}, time.Unix(reset, 0)},
		{quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, now.Add(time.Minute)},
	} {
		status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/one": test.usage}})
		status.now = func() time.Time { return now }
		availability, err := status.Availability(context.Background(), account)
		if err != nil || availability.Ready || !availability.RetryAt.Equal(test.deadline) {
			t.Fatalf("availability = %v, %v", availability, err)
		}
	}
}

func TestAvailabilityForRouteUsesLunaReserveOnlyForLuna(t *testing.T) {
	now := time.Unix(10, 0)
	used := int64(100)
	reset := int64(100)
	reserveUsed := int64(20)
	allowed := false
	usage := quotamodel.Usage{
		Allowed: &allowed,
		Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset},
		AdditionalRateLimits: []quotamodel.AdditionalRateLimit{{
			LimitID: "base_model_inference", LimitName: "gpt-luna-reserve",
			MeteredFeature: "base_model_inference", NormalModelSlug: "gpt-5.6-luna",
			Primary: &quotamodel.Window{UsedPercent: &reserveUsed},
		}},
	}
	account := accountentity.Account{ID: "one", Enabled: true}
	status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{"/managed/one": usage}})
	status.now = func() time.Time { return now }
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-5.6-luna"}

	availability, err := status.AvailabilityForRoute(context.Background(), account, selection)
	if err != nil || !availability.Ready {
		t.Fatalf("Luna reserve availability = %+v, err = %v", availability, err)
	}
	selection.RequestedModel = "gpt-5.6-sol"
	availability, err = status.AvailabilityForRoute(context.Background(), account, selection)
	if err != nil || availability.Ready || !availability.RetryAt.Equal(time.Unix(reset, 0)) {
		t.Fatalf("regular model availability = %+v, err = %v", availability, err)
	}
}

func TestAvailabilityForResponsesIgnoresExhaustedWeeklyWindow(t *testing.T) {
	now := time.Unix(10, 0)
	primaryUsed, secondaryUsed := int64(80), int64(100)
	primaryReset, secondaryReset := int64(100), int64(200)
	account := accountentity.Account{ID: "one", Enabled: true}
	status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {
			Primary:   &quotamodel.Window{UsedPercent: &primaryUsed, ResetAt: &primaryReset},
			Secondary: &quotamodel.Window{UsedPercent: &secondaryUsed, ResetAt: &secondaryReset},
		},
	}})
	status.now = func() time.Time { return now }

	availability, err := status.AvailabilityForRoute(context.Background(), account, quotamodel.Selection{
		RouteKind: quotamodel.RouteKindResponses,
	})
	if err != nil || !availability.Ready {
		t.Fatalf("Responses availability = %+v, err = %v", availability, err)
	}
	availability, err = status.Availability(context.Background(), account)
	if err != nil || availability.Ready {
		t.Fatalf("launch availability = %+v, err = %v", availability, err)
	}
}

func TestAvailabilityForRouteTreatsRetiredSparkAsUnavailable(t *testing.T) {
	account := accountentity.Account{ID: "one", Enabled: true}
	status := NewStatus(fakeAccounts{}, fakeUsage{byHome: map[string]quotamodel.Usage{
		"/managed/one": {PlanType: "plus"},
	}})
	status.now = func() time.Time { return time.Unix(10, 0) }
	availability, err := status.AvailabilityForRoute(context.Background(), account, quotamodel.Selection{
		RouteKind: quotamodel.RouteKindResponses, RequestedModel: "GPT_5 3_CODEX_SPARK",
	})
	if err != nil || availability.Ready || availability.RetryAt.IsZero() {
		t.Fatalf("retired Spark availability = %+v, err = %v", availability, err)
	}
}

type fakeRawUsage struct {
	home string
	body []byte
}

func (fake *fakeRawUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	return quotamodel.Usage{}, nil
}

func (fake *fakeRawUsage) FetchRaw(_ context.Context, home string) ([]byte, error) {
	fake.home = home
	return append([]byte(nil), fake.body...), nil
}

func TestStatusRawUsesCurrentAndSelectedAccountHomes(t *testing.T) {
	first := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	second := accountentity.Account{ID: "two", Name: "two", Enabled: false}
	usage := &fakeRawUsage{body: []byte(`{"plan_type":"plus"}`)}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{first, second}, current: first}, usage)

	body, err := status.Raw(context.Background(), "", "")
	if err != nil || string(body) != `{"plan_type":"plus"}` || usage.home != "/managed/one" {
		t.Fatalf("current raw = %q, home = %q, err = %v", body, usage.home, err)
	}
	body, err = status.Raw(context.Background(), "two", "")
	if err != nil || string(body) != `{"plan_type":"plus"}` || usage.home != "/managed/two" {
		t.Fatalf("selected raw = %q, home = %q, err = %v", body, usage.home, err)
	}
}

type fakeOverrideUsage struct {
	baseURL string
	home    string
	usage   quotamodel.Usage
}

func (fake *fakeOverrideUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	return fake.usage, nil
}

func (fake *fakeOverrideUsage) FetchAt(_ context.Context, home, baseURL string) (quotamodel.Usage, error) {
	fake.home, fake.baseURL = home, baseURL
	return fake.usage, nil
}

func (fake *fakeOverrideUsage) FetchRaw(context.Context, string) ([]byte, error) {
	return []byte(`{"plan_type":"plus"}`), nil
}

func (fake *fakeOverrideUsage) FetchRawAt(_ context.Context, home, baseURL string) ([]byte, error) {
	fake.home, fake.baseURL = home, baseURL
	return []byte(`{"plan_type":"plus"}`), nil
}

func TestStatusUsesQuotaBaseURLOverride(t *testing.T) {
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	usage := &fakeOverrideUsage{usage: quotamodel.Usage{PlanType: "plus"}}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, usage)
	const baseURL = "https://quota.test/backend-api"
	reports, err := status.Run(context.Background(), Options{Selector: "one", BaseURL: baseURL})
	if err != nil || len(reports) != 1 || usage.baseURL != baseURL || usage.home != "/managed/one" {
		t.Fatalf("reports = %+v, baseURL/home = %q/%q, err = %v", reports, usage.baseURL, usage.home, err)
	}
	if _, err := status.Raw(context.Background(), "one", baseURL); err != nil || usage.baseURL != baseURL {
		t.Fatalf("raw override baseURL = %q, err = %v", usage.baseURL, err)
	}
}

type fakeProfileSource struct {
	targets []profilemodel.QuotaTarget
}

func (source fakeProfileSource) QuotaTargets(context.Context) ([]profilemodel.QuotaTarget, error) {
	return append([]profilemodel.QuotaTarget(nil), source.targets...), nil
}

type trackingUsage struct {
	homes   []string
	rawHome string
}

type fakeModelProviderInspector struct {
	setting *profilemodel.ModelProviderSetting
	err     error
	home    string
	calls   int
}

func (fake *fakeModelProviderInspector) InspectModelProvider(_ context.Context, home string) (*profilemodel.ModelProviderSetting, error) {
	fake.calls++
	fake.home = home
	return fake.setting, fake.err
}

func (usage *trackingUsage) Fetch(_ context.Context, home string) (quotamodel.Usage, error) {
	usage.homes = append(usage.homes, home)
	return quotamodel.Usage{PlanType: "plus"}, nil
}

func (usage *trackingUsage) FetchRaw(_ context.Context, home string) ([]byte, error) {
	usage.rawHome = home
	return []byte(`{"plan_type":"plus"}`), nil
}

func TestStatusProfileViewFiltersAuthAndProvider(t *testing.T) {
	usage := &trackingUsage{}
	status := NewStatus(fakeAccounts{}, usage)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{
		{Name: "main", CodexHome: "/profiles/main", Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true, Compatible: true},
		{Name: "logged-out", CodexHome: "/profiles/logged-out", Provider: "openai", Auth: "no-auth", Enabled: true},
		{Name: "claude", CodexHome: "/profiles/claude", Provider: "anthropic", Auth: "claude-oauth", Enabled: true},
	}})

	reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "openai", AuthFilter: "quota-compatible"})
	if err != nil || len(reports) != 1 || reports[0].ProfileName != "main" || reports[0].State != "ready" {
		t.Fatalf("filtered reports = %+v, err = %v", reports, err)
	}
	if len(usage.homes) != 1 || usage.homes[0] != "/profiles/main" {
		t.Fatalf("OpenAI quota homes = %#v", usage.homes)
	}

	usage.homes = nil
	reports, err = status.Run(context.Background(), Options{All: true, ProviderFilter: "claude"})
	if err != nil || len(reports) != 1 || reports[0].ProfileName != "claude" || reports[0].State != "unsupported" {
		t.Fatalf("claude reports = %+v, err = %v", reports, err)
	}
	if len(usage.homes) != 0 {
		t.Fatalf("unsupported provider was probed: %#v", usage.homes)
	}

	reports, err = status.Run(context.Background(), Options{All: true, AuthFilter: "no-auth"})
	if err != nil || len(reports) != 1 || reports[0].ProfileName != "logged-out" || reports[0].State != "no-auth" {
		t.Fatalf("no-auth reports = %+v, err = %v", reports, err)
	}
}

func TestStatusProfileViewDefaultsToActiveAndRawUsesProfileHome(t *testing.T) {
	usage := &trackingUsage{}
	status := NewStatus(fakeAccounts{}, usage)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{
		{Name: "main", CodexHome: "/profiles/main", Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true, Compatible: true},
		{Name: "other", CodexHome: "/profiles/other", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true},
	}})

	reports, err := status.Run(context.Background(), Options{})
	if err != nil || len(reports) != 1 || reports[0].ProfileName != "main" {
		t.Fatalf("active reports = %+v, err = %v", reports, err)
	}
	body, err := status.Raw(context.Background(), "other", "")
	if err != nil || string(body) != `{"plan_type":"plus"}` || usage.rawHome != "/profiles/other" {
		t.Fatalf("raw = %q, home = %q, err = %v", body, usage.rawHome, err)
	}
}

func TestStatusRawReturnsConfiguredCodexProviderSnapshot(t *testing.T) {
	usage := &trackingUsage{}
	inspector := &fakeModelProviderInspector{setting: &profilemodel.ModelProviderSetting{
		ProviderID: "prodex-deepseek", Source: "config.toml",
	}}
	status := NewStatus(fakeAccounts{}, usage)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "custom", CodexHome: "/profiles/custom", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true,
	}}})
	status.SetModelProviderInspector(inspector)

	body, err := status.Raw(context.Background(), "custom", "")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Provider string `json:"provider"`
		Account  string `json:"account"`
		Plan     string `json:"plan"`
		Status   string `json:"status"`
		Main     string `json:"main"`
		Details  []struct {
			Label string `json:"label"`
			Value string `json:"value"`
		} `json:"details"`
	}
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("raw snapshot = %q: %v", body, err)
	}
	if snapshot.Provider != "DeepSeek" || snapshot.Account != "prodex-deepseek" || snapshot.Plan != "config.toml" ||
		snapshot.Status != "Configured" || snapshot.Main != "quota handled by provider/Codex" ||
		len(snapshot.Details) != 2 || snapshot.Details[0].Label != "Model provider" || snapshot.Details[1].Value != "config.toml" {
		t.Fatalf("raw snapshot = %#v", snapshot)
	}
	if inspector.calls != 1 || inspector.home != "/profiles/custom" || usage.rawHome != "" {
		t.Fatalf("inspection/raw usage = %d/%q / %q", inspector.calls, inspector.home, usage.rawHome)
	}
}

func TestStatusShowsConfiguredCodexProviderWithoutQuotaProbe(t *testing.T) {
	usage := &trackingUsage{}
	inspector := &fakeModelProviderInspector{setting: &profilemodel.ModelProviderSetting{
		ProviderID: "prodex-deepseek", Source: "config.toml",
	}}
	status := NewStatus(fakeAccounts{}, usage)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "custom", CodexHome: "/profiles/custom", Provider: "openai", Auth: "api-key", Enabled: true,
	}}})
	status.SetModelProviderInspector(inspector)
	reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "openai"})
	if err != nil {
		t.Fatal(err)
	}
	if inspector.calls != 1 || inspector.home != "/profiles/custom" || len(usage.homes) != 0 {
		t.Fatalf("provider inspection/quota calls = %d/%q / %#v", inspector.calls, inspector.home, usage.homes)
	}
	if len(reports) != 1 || reports[0].State != "configured" || reports[0].Auth != "api-key" || reports[0].External == nil ||
		reports[0].External.Provider != "DeepSeek" || reports[0].External.Account != "prodex-deepseek" ||
		reports[0].External.Plan != "config.toml" || reports[0].External.Status != "Configured" ||
		reports[0].External.Main != "quota handled by provider/Codex" || reports[0].External.Available != nil {
		t.Fatalf("custom provider quota report = %#v", reports)
	}
}

func TestCodexModelProviderDisplayUsesExactProviderID(t *testing.T) {
	info := codexModelProviderQuota(profilemodel.ModelProviderSetting{
		ProviderID: " prodex-deepseek ", Source: "config.toml",
	})
	if info.Provider != "Custom provider ( prodex-deepseek )" || info.Account != " prodex-deepseek " {
		t.Fatalf("custom provider info = %#v", info)
	}
}

func TestStatusReportsCodexProviderInspectionFailure(t *testing.T) {
	usage := &trackingUsage{}
	inspector := &fakeModelProviderInspector{err: errors.New("synthetic config error")}
	status := NewStatus(fakeAccounts{}, usage)
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "custom", CodexHome: "/profiles/custom", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true,
	}}})
	status.SetModelProviderInspector(inspector)
	reports, err := status.Run(context.Background(), Options{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].State != "error" || reports[0].Auth != "chatgpt" || reports[0].Err == nil || len(usage.homes) != 0 {
		t.Fatalf("inspection failure report/quota calls = %#v / %#v", reports, usage.homes)
	}
}

func TestStatusFiltersConfiguredCodexProvidersLikeProdex(t *testing.T) {
	for _, test := range []struct {
		name       string
		filter     string
		providerID string
		wantCount  int
	}{
		{name: "deepseek", filter: "deepseek", providerID: "prodex-deepseek", wantCount: 1},
		{name: "local", filter: "local", providerID: "prodex-local", wantCount: 1},
		{name: "anthropic custom provider is not an anthropic profile", filter: "anthropic", providerID: "prodex-anthropic"},
	} {
		t.Run(test.name, func(t *testing.T) {
			usage := &trackingUsage{}
			inspector := &fakeModelProviderInspector{setting: &profilemodel.ModelProviderSetting{
				ProviderID: test.providerID, Source: "config.toml",
			}}
			status := NewStatus(fakeAccounts{}, usage)
			status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
				Name: "custom", CodexHome: "/profiles/custom", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true,
			}}})
			status.SetModelProviderInspector(inspector)
			reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: test.filter})
			if err != nil {
				t.Fatal(err)
			}
			if len(reports) != test.wantCount || len(usage.homes) != 0 {
				t.Fatalf("reports/quota probes = %#v / %#v", reports, usage.homes)
			}
		})
	}
}

func TestStatusAuthFiltersUseConfiguredCodexProvider(t *testing.T) {
	for _, test := range []struct {
		filter    string
		wantCount int
	}{
		{filter: "quota-compatible", wantCount: 1},
		{filter: "non-quota-compatible"},
		{filter: "model-provider:prodex-deepseek"},
		{filter: "chatgpt", wantCount: 1},
	} {
		t.Run(test.filter, func(t *testing.T) {
			usage := &trackingUsage{}
			status := NewStatus(fakeAccounts{}, usage)
			status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
				Name: "custom", CodexHome: "/profiles/custom", Provider: "openai", Auth: "chatgpt", Enabled: true, Compatible: true,
			}}})
			status.SetModelProviderInspector(&fakeModelProviderInspector{setting: &profilemodel.ModelProviderSetting{
				ProviderID: "prodex-deepseek", Source: "config.toml",
			}})
			reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "openai", AuthFilter: test.filter})
			if err != nil {
				t.Fatal(err)
			}
			if len(reports) != test.wantCount || len(usage.homes) != 0 {
				t.Fatalf("reports/quota probes = %#v / %#v", reports, usage.homes)
			}
		})
	}
}

type fakeVirtualQuota struct {
	provider string
	baseURL  string
	results  []quotamodel.VirtualResult
}

func (fake *fakeVirtualQuota) Collect(_ context.Context, provider, baseURL string) []quotamodel.VirtualResult {
	fake.provider, fake.baseURL = provider, baseURL
	return append([]quotamodel.VirtualResult(nil), fake.results...)
}

func TestStatusVirtualCollectorRunsByProviderFilterNotAllFlag(t *testing.T) {
	virtual := &fakeVirtualQuota{}
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{})
	status.SetVirtual(virtual)
	if _, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "all"}); err != nil {
		t.Fatal(err)
	}
	if virtual.provider != "all" {
		t.Fatalf("virtual provider = %q", virtual.provider)
	}
	virtual.provider = ""
	if _, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "agy"}); err != nil {
		t.Fatal(err)
	}
	if virtual.provider != "agy" {
		t.Fatalf("agy virtual provider = %q", virtual.provider)
	}
}

func TestStatusAppendsVirtualProviderQuotaAfterProfileFiltering(t *testing.T) {
	available := true
	virtual := &fakeVirtualQuota{results: []quotamodel.VirtualResult{{
		Name: "deepseek", Provider: "deepseek", Auth: "deepseek-key",
		External: &quotamodel.ExternalInfo{
			Provider: "DeepSeek", Plan: "api-key", Status: "Ready", Main: "USD 12.34", Available: &available,
		},
	}}}
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{})
	status.SetVirtual(virtual)
	reports, err := status.Run(context.Background(), Options{
		All: true, ProviderFilter: "deepseek", AuthFilter: "chatgpt", BaseURL: "https://quota.example.test/v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if virtual.provider != "deepseek" || virtual.baseURL != "https://quota.example.test/v1" {
		t.Fatalf("virtual arguments = %q / %q", virtual.provider, virtual.baseURL)
	}
	if len(reports) != 1 || reports[0].ProfileName != "deepseek" || reports[0].State != "ready" || reports[0].External == nil || reports[0].External.Main != "USD 12.34" {
		t.Fatalf("virtual reports = %#v", reports)
	}
}

func TestStatusVirtualProviderErrorBecomesReportError(t *testing.T) {
	virtual := &fakeVirtualQuota{results: []quotamodel.VirtualResult{{
		Name: "local", Provider: "local", Auth: "local", Err: errors.New("synthetic virtual quota failure"),
	}}}
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{})
	status.SetVirtual(virtual)
	reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].State != "error" || reports[0].Err == nil {
		t.Fatalf("virtual error reports = %#v", reports)
	}
}

type fakeExternalProfileQuota struct {
	target profilemodel.QuotaTarget
	info   quotamodel.ExternalInfo
	err    error
	calls  int
}

func (fake *fakeExternalProfileQuota) FetchQuota(_ context.Context, target profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	fake.calls++
	fake.target = target
	return fake.info, fake.err
}

func TestStatusUsesProfileBackedExternalQuotaAdapter(t *testing.T) {
	available := true
	external := &fakeExternalProfileQuota{info: quotamodel.ExternalInfo{
		Provider: "Kiro CLI", Account: "person@example.test", Plan: "builder-id",
		Status: "Ready (imported)", Main: "2 imported models", Available: &available,
	}}
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "kiro-main", CodexHome: "/profiles/kiro-main", Provider: "kiro", Auth: "kiro",
		Enabled: true, Active: true,
		ProviderConfig: profilemodel.ProviderSnapshot{Kind: "kiro", ProfileName: stringPtr("main")},
	}}})
	status.SetExternalProvider("kiro", external)
	reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "kiro"})
	if err != nil {
		t.Fatal(err)
	}
	if external.calls != 1 || external.target.CodexHome != "/profiles/kiro-main" || external.target.ProviderConfig.ProfileName == nil || *external.target.ProviderConfig.ProfileName != "main" {
		t.Fatalf("external call/target = %d / %#v", external.calls, external.target)
	}
	if len(reports) != 1 || reports[0].State != "ready (imported)" || reports[0].External == nil || reports[0].External.Main != "2 imported models" || reports[0].Err != nil {
		t.Fatalf("external reports = %#v", reports)
	}
}

func TestStatusExternalQuotaAdapterFailureBecomesReportError(t *testing.T) {
	external := &fakeExternalProfileQuota{err: errors.New("synthetic Kiro quota failure")}
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "kiro-main", CodexHome: "/profiles/kiro-main", Provider: "kiro", Auth: "kiro", Enabled: true,
	}}})
	status.SetExternalProvider("kiro", external)
	reports, err := status.Run(context.Background(), Options{All: true, ProviderFilter: "kiro"})
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].State != "error" || reports[0].Err == nil || reports[0].External != nil {
		t.Fatalf("external failure report = %#v", reports)
	}
}

func stringPtr(value string) *string { return &value }
