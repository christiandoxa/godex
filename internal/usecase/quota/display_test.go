package quota

import (
	"context"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type displayQuotaFixture struct {
	countingUsage
	displays       int
	home, upstream string
}

func (fixture *displayQuotaFixture) FetchForDisplayAt(_ context.Context, home, upstream string) (quotamodel.Usage, error) {
	fixture.displays++
	fixture.home, fixture.upstream = home, upstream
	return quotamodel.Usage{ResetCredits: &quotamodel.ResetCredits{AvailableCount: 2, NeverExpires: true}}, nil
}

func TestDisplayEnrichmentDoesNotEnterRuntimeQuotaProbe(t *testing.T) {
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	gateway := &displayQuotaFixture{}
	status := NewStatus(fakeAccounts{current: account, accounts: []accountentity.Account{account}}, gateway)
	reports, err := status.Run(t.Context(), Options{BaseURL: "http://127.0.0.1:1/backend-api"})
	if err != nil || len(reports) != 1 || reports[0].Usage.ResetCredits == nil || !reports[0].Usage.ResetCredits.NeverExpires || gateway.displays != 1 || gateway.calls != 0 || gateway.home != "/managed/one" || gateway.upstream != "http://127.0.0.1:1/backend-api" {
		t.Fatalf("display contract failed: reports=%+v gateway=%+v err=%v", reports, gateway, err)
	}
	if _, err := status.Availability(t.Context(), account); err != nil {
		t.Fatal(err)
	}
	if gateway.displays != 1 || gateway.calls != 1 {
		t.Fatalf("display request entered runtime probe: %+v", gateway)
	}
}
