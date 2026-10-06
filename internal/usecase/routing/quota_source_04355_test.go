package routing

import (
	"context"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestProdex04355ResponsesRanksLiveQuotaBeforePersistedSnapshot(t *testing.T) {
	now := time.Unix(240_000, 0)
	pressure := pressureScore(0, 100)
	quota := &cachedPressureQuota{byID: map[string]quotamodel.Availability{
		"live":     {Ready: true, Pressure: pressure, Source: quotamodel.SourceLive},
		"snapshot": {Ready: true, Pressure: pressure, Source: quotamodel.SourcePersistedSnapshot},
	}}
	accounts := []proxymodel.Account{
		{ID: "snapshot", Home: "/snapshot", Enabled: true, RouteOrder: 1},
		{ID: "live", Home: "/live", Enabled: true, RouteOrder: 2},
	}
	router, err := NewRouter(Config{
		Now: func() time.Time { return now }, QuotaPreflight: quota,
		Accounts: func(context.Context) ([]proxymodel.Account, error) {
			return append([]proxymodel.Account(nil), accounts...), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	responses := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}, now)
	if responses[0].ID != "live" {
		t.Fatalf("Responses source order = %s,%s", responses[0].ID, responses[1].ID)
	}
	standard := router.requestCandidates(accounts, quotamodel.Selection{RouteKind: quotamodel.RouteKindStandard}, now)
	if standard[0].ID != "snapshot" {
		t.Fatalf("Standard source order = %s,%s, want stable route order", standard[0].ID, standard[1].ID)
	}
}
