package routing

import (
	"context"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type changedQuotaSnapshot043714 struct {
	quota   map[string]quotamodel.Availability
	version map[string]uint64
}

func (f *changedQuotaSnapshot043714) AvailabilityForRoute(_ context.Context, a accountentity.Account, _ quotamodel.Selection) (quotamodel.Availability, error) {
	return f.quota[a.ID], nil
}
func (f *changedQuotaSnapshot043714) CachedAvailabilityForRoute(a accountentity.Account, _ quotamodel.Selection, _ time.Time) (quotamodel.Availability, bool) {
	value, ok := f.quota[a.ID]
	return value, ok
}
func (f *changedQuotaSnapshot043714) QuotaObservationRevision(accountID string) uint64 {
	return f.version[accountID]
}

func TestProdex043714QuotaObservationInvalidatesOnlyRevalidatedProfileCache(t *testing.T) {
	now := time.Unix(300000, 0)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	source := &changedQuotaSnapshot043714{
		quota: map[string]quotamodel.Availability{
			"main":  {RetryAt: now.Add(time.Hour), Source: quotamodel.SourceLive},
			"third": {Ready: true, Source: quotamodel.SourceLive},
		},
		version: map[string]uint64{"main": 1, "third": 1},
	}
	accounts := []proxymodel.Account{
		{ID: "main", Home: "/main", Enabled: true, RouteOrder: 1},
		{ID: "third", Home: "/third", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{Accounts: func(context.Context) ([]proxymodel.Account, error) { return accounts, nil }, QuotaPreflight: source, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	first := router.requestCandidates(accounts, selection, now)
	if len(first) != 1 || first[0].ID != "third" {
		t.Fatalf("initial quota gate: %+v", first)
	}
	// Fresh authenticated usage for main must override its cached 5-minute
	// blocked status without an unrelated third profile losing its cache.
	source.quota["main"] = quotamodel.Availability{Ready: true, Source: quotamodel.SourceLive}
	source.version["main"] = 2
	second := router.requestCandidates(accounts, selection, now)
	if len(second) != 2 {
		t.Fatalf("new confirmed ready quota did not refresh only main: %+v", second)
	}
	other, stillCached := router.cachedQuotaCheck("third", selection, now)
	if !stillCached || !other.ready {
		t.Fatal("independent third profile quota cache was destroyed")
	}
	// A later exhaustion of third must be observed without overwriting main.
	source.quota["third"] = quotamodel.Availability{RetryAt: now.Add(2 * time.Hour), Source: quotamodel.SourceLive}
	source.version["third"] = 2
	third := router.requestCandidates(accounts, selection, now)
	if len(third) != 1 || third[0].ID != "main" {
		t.Fatalf("stale third profile remained selectable: %+v", third)
	}
	mainCached, mainOK := router.cachedQuotaCheck("main", selection, now)
	if !mainOK || !mainCached.ready {
		t.Fatal("refresh of third poisoned main cached ready quota")
	}
}
