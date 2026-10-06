package quota

import (
	"context"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestAvailabilitySharesFiveMinuteUsageSnapshotWithRouteChecks(t *testing.T) {
	account := accountentity.Account{ID: "one", Enabled: true}
	usage := &countingUsage{}
	status := NewStatus(fakeAccounts{}, usage)
	now := time.Unix(100, 0)
	status.now = func() time.Time { return now }

	if _, err := status.Availability(context.Background(), account); err != nil {
		t.Fatal(err)
	}
	cached, ok := status.CachedAvailabilityForRoute(account, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if !ok || !cached.Ready {
		t.Fatalf("cached route availability = %+v, cached = %t", cached, ok)
	}
	if _, err := status.AvailabilityForRoute(context.Background(), account, quotamodel.Selection{}); err != nil {
		t.Fatal(err)
	}
	if usage.calls != 1 {
		t.Fatalf("quota fetches before expiry = %d, want 1", usage.calls)
	}

	now = now.Add(usageCacheFreshness + time.Second)
	if _, ok := status.CachedAvailabilityForRoute(account, quotamodel.Selection{}, now); ok {
		t.Fatal("expired quota snapshot remained available to routing")
	}
	if _, err := status.AvailabilityForRoute(context.Background(), account, quotamodel.Selection{}); err != nil {
		t.Fatal(err)
	}
	if usage.calls != 2 {
		t.Fatalf("quota fetches after expiry = %d, want 2", usage.calls)
	}
}

type overridePolicyUsage struct {
	fetchCalls  int
	policyCalls int
	baseURL     string
	noProxy     bool
}

func (usage *overridePolicyUsage) Fetch(context.Context, string) (quotamodel.Usage, error) {
	usage.fetchCalls++
	return quotamodel.Usage{}, nil
}

func (usage *overridePolicyUsage) FetchAtPolicy(
	_ context.Context,
	_ string,
	baseURL string,
	noProxy bool,
) (quotamodel.Usage, error) {
	usage.policyCalls++
	usage.baseURL = baseURL
	usage.noProxy = noProxy
	used := int64(100)
	reset := time.Unix(10_000, 0).Unix()
	return quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used, ResetAt: &reset}}, nil
}

func TestProdex04356AvailabilityBaseURLBypassesDefaultLiveCache(t *testing.T) {
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	usage := &overridePolicyUsage{}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, usage)
	status.now = func() time.Time { return time.Unix(100, 0) }

	defaultAvailability, err := status.Availability(context.Background(), account)
	if err != nil || !defaultAvailability.Ready {
		t.Fatalf("default availability = %+v, err=%v", defaultAvailability, err)
	}
	const baseURL = "https://override.example.test/backend-api"
	overrideAvailability, err := status.AvailabilityAtPolicy(context.Background(), account, baseURL, true)
	if err != nil {
		t.Fatal(err)
	}
	if overrideAvailability.Ready {
		t.Fatalf("override reused default live cache: %+v", overrideAvailability)
	}
	if usage.fetchCalls != 1 || usage.policyCalls != 1 || usage.baseURL != baseURL || !usage.noProxy {
		t.Fatalf("usage calls/default/policy/base/no-proxy = %d/%d/%q/%t",
			usage.fetchCalls, usage.policyCalls, usage.baseURL, usage.noProxy)
	}
}

func TestQuotaPressureMatchesRouteBandsPlanScaleAndResetCost(t *testing.T) {
	now := time.Unix(100, 0)
	weeklyUsed, fiveHourUsed := int64(50), int64(80)
	weeklyReset, fiveHourReset := int64(200), int64(300)
	usage := quotamodel.Usage{
		PlanType:  "pro",
		Primary:   &quotamodel.Window{UsedPercent: &fiveHourUsed, ResetAt: &fiveHourReset},
		Secondary: &quotamodel.Window{UsedPercent: &weeklyUsed, ResetAt: &weeklyReset},
	}
	pressure := quotaPressure(usage, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if !pressure.Known || pressure.Band != 0 || pressure.Weekly != 1_000 || pressure.FiveHour != 5_000 || pressure.Total != 15_000 || pressure.ReserveFloor != 20 {
		t.Fatalf("pressure = %+v", pressure)
	}

	weeklyUsed, fiveHourUsed = 95, 100
	pressure = quotaPressure(usage, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if pressure.Band != 3 {
		t.Fatalf("exhausted route band = %d, want 3", pressure.Band)
	}
}

func TestLunaReserveIdentifierMatchesOneReferenceField(t *testing.T) {
	for _, test := range []struct {
		name  string
		limit quotamodel.AdditionalRateLimit
		want  bool
	}{
		{name: "exact reserve ID", limit: quotamodel.AdditionalRateLimit{NormalModelSlug: "gpt-5.6-luna", LimitID: "gpt_reserve"}, want: true},
		{name: "luna and reserve in one field", limit: quotamodel.AdditionalRateLimit{NormalModelSlug: "luna", LimitName: "gpt-luna-reserve"}, want: true},
		{name: "tokens split across fields", limit: quotamodel.AdditionalRateLimit{NormalModelSlug: "luna", LimitID: "luna", LimitName: "reserve"}},
		{name: "wrong model", limit: quotamodel.AdditionalRateLimit{NormalModelSlug: "sol", LimitName: "gpt-luna-reserve"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := lunaReserve(test.limit); got != test.want {
				t.Fatalf("lunaReserve() = %t, want %t", got, test.want)
			}
		})
	}
}
