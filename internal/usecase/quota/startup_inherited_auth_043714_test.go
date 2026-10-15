package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	routingentity "github.com/christiandoxa/godex/internal/entity/routing"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type syntheticInheritedAuthState043714 struct {
	entries []routingentity.AuthBackoff
	err     error
	reads   int
}

func (s *syntheticInheritedAuthState043714) LoadAuthBackoffs(context.Context, time.Time) ([]routingentity.AuthBackoff, error) {
	s.reads++
	return s.entries, s.err
}

func TestProdex043714StartupInheritedAuthRevalidatesDespiteFreshQuota(t *testing.T) {
	now := time.Unix(1700000000, 0)
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	used := int64(15)
	digest := [32]byte{1, 4, 7}
	stub := &authenticatedQuotaStub043714{usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
		receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: now.Add(-time.Second)},
	}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, stub)
	status.now = func() time.Time { return now }
	defer status.Close()
	source := &syntheticInheritedAuthState043714{entries: []routingentity.AuthBackoff{{
		AccountID: account.ID, FailedAtMillis: now.Add(-time.Minute).UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli(),
	}}}
	status.SetInheritedAuthPenaltySource043714(source)
	home := "/managed/one"
	status.storeUsage(home, quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, now)
	before := status.ProbeRefreshRevision()
	status.WarmupStartupProfiles(t.Context(), []proxymodel.Account{{ID: account.ID, Home: home, Enabled: true}}, "", false)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, before) {
		t.Fatal("inherited auth penalty was not revalidated")
	}
	if stub.calls != 1 || source.reads != 1 {
		t.Fatalf("fresh cached quota incorrectly bypassed a historical 401: calls=%d reads=%d", stub.calls, source.reads)
	}
	availability, receipt, valid := status.AuthenticatedRecoveryReceipt(account.ID, quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses})
	if !valid || !availability.Ready || !receipt.Confirmed || receipt.CredentialDigest != digest {
		t.Fatalf("verified inherited-auth receipt unavailable: valid=%t state=%+v proof=%+v", valid, availability, receipt)
	}
}

func TestProdex043714StartupInheritedAuthNeverTreatsFreshCacheAsProof(t *testing.T) {
	now := time.Unix(1700000000, 0)
	account := accountentity.Account{ID: "one", Name: "one", Enabled: true}
	used := int64(10)
	for _, scenario := range []struct {
		name    string
		entries []routingentity.AuthBackoff
		err     error
	}{
		{name: "no inherited auth"},
		{name: "auth store unavailable", err: errors.New("synthetic auth store failure")},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			stub := &authenticatedQuotaStub043714{usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}}
			status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, stub)
			status.now = func() time.Time { return now }
			defer status.Close()
			status.storeUsage("/managed/one", quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, now)
			status.SetInheritedAuthPenaltySource043714(&syntheticInheritedAuthState043714{entries: scenario.entries, err: scenario.err})
			status.WarmupStartupProfiles(t.Context(), []proxymodel.Account{{ID: "one", Home: "/managed/one", Enabled: true}}, "", false)
			if stub.calls != 0 {
				t.Fatal("regular fresh cached quota incorrectly triggered an authenticated probe")
			}
			if _, _, ok := status.AuthenticatedRecoveryReceipt("one", quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}); ok {
				t.Fatal("inherited auth validation was fabricated from a cached DTO")
			}
		})
	}
}

func TestProdex043714StartupInheritedAuthReceiptCoversRouteAndModelSelections(t *testing.T) {
	now := time.Unix(1700000000, 0)
	account := accountentity.Account{ID: "one", Enabled: true}
	used := int64(15)
	digest := [32]byte{2, 5, 8}
	stub := &authenticatedQuotaStub043714{
		usage:   quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
		receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: digest, StartedAt: now.Add(-time.Second)},
	}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, stub)
	status.now = func() time.Time { return now }
	defer status.Close()
	status.SetInheritedAuthPenaltySource043714(&syntheticInheritedAuthState043714{entries: []routingentity.AuthBackoff{{
		AccountID: account.ID, FailedAtMillis: now.Add(-time.Minute).UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli(),
	}}})
	before := status.ProbeRefreshRevision()
	status.WarmupStartupProfiles(t.Context(), []proxymodel.Account{{ID: account.ID, Home: "/external/isolated-home", Enabled: true}}, "", false)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, before) {
		t.Fatal("inherited auth probe not executed")
	}
	for _, selection := range []quotamodel.Selection{
		{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-6.1-sol"},
		{RouteKind: quotamodel.RouteKindCompact, RequestedModel: "gpt-6.1-sol"},
	} {
		availability, receipt, valid := status.AuthenticatedRecoveryReceipt(account.ID, selection)
		if !valid || !availability.Ready || availability.Source != quotamodel.SourceLive || receipt.CredentialDigest != digest {
			t.Fatalf("startup proof did not cover selection %+v: valid=%t availability=%+v receipt=%+v", selection, valid, availability, receipt)
		}
	}
}

type inheritedAuthPolicyProbe043714 struct {
	authenticatedQuotaStub043714
	capturedHome    string
	capturedURL     string
	capturedNoProxy bool
	currentHomes    []string
}

func (p *inheritedAuthPolicyProbe043714) FetchAuthenticatedAtPolicy(ctx context.Context, home, url string, noProxy bool) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error) {
	p.capturedHome, p.capturedURL, p.capturedNoProxy = home, url, noProxy
	return p.authenticatedQuotaStub043714.FetchAuthenticatedAtPolicy(ctx, home, url, noProxy)
}

func (p *inheritedAuthPolicyProbe043714) CurrentCredentialDigest(_ context.Context, home string) ([32]byte, error) {
	p.currentHomes = append(p.currentHomes, home)
	return p.receipt.CredentialDigest, nil
}

func TestProdex043714StartupInheritedAuthRespectsProfileHomeAndTransportPolicy(t *testing.T) {
	now := time.Unix(1700000000, 0)
	a := accountentity.Account{ID: "one", Enabled: true}
	used := int64(11)
	stub := &inheritedAuthPolicyProbe043714{authenticatedQuotaStub043714: authenticatedQuotaStub043714{
		usage:   quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
		receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: [32]byte{7}, StartedAt: now.Add(-time.Second)},
	}}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{a}, current: a}, stub)
	status.now = func() time.Time { return now }
	defer status.Close()
	source := &syntheticInheritedAuthState043714{entries: []routingentity.AuthBackoff{{
		AccountID: a.ID, FailedAtMillis: now.Add(-time.Minute).UnixMilli(), UntilMillis: now.Add(time.Hour).UnixMilli(),
	}}}
	status.SetInheritedAuthPenaltySource043714(source)
	revision := status.ProbeRefreshRevision()
	url := "http://127.0.0.1:49151/backend-api"
	status.WarmupStartupProfiles(t.Context(), []proxymodel.Account{{ID: a.ID, Home: "/external/isolated-home", Enabled: true}}, url, true)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, revision) {
		t.Fatal("inherited auth probe not executed")
	}
	if stub.capturedHome != "/external/isolated-home" || stub.capturedURL != url || !stub.capturedNoProxy || stub.calls != 1 {
		t.Fatalf("startup revalidation ignored original home or transport policy: %+v", stub)
	}
	if _, _, ok := status.AuthenticatedRecoveryReceipt(a.ID, quotamodel.Selection{
		RouteKind: quotamodel.RouteKindCompact, RequestedModel: "gpt-6.1-sol",
	}); !ok {
		t.Fatal("startup proof did not survive route/model lookup")
	}
	if len(stub.currentHomes) != 1 || stub.currentHomes[0] != "/external/isolated-home" {
		t.Fatalf("credential recheck used the wrong profile home: %#v", stub.currentHomes)
	}
}
