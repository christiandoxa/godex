package quota

import (
	"context"
	"errors"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type authenticatedQuotaStub043714 struct {
	usage   quotamodel.Usage
	receipt quotamodel.AuthenticatedReceipt
	err     error
	calls   int
}

func (s *authenticatedQuotaStub043714) Fetch(context.Context, string) (quotamodel.Usage, error) {
	return quotamodel.Usage{}, errors.New("ordinary quota cache is not authenticated evidence")
}
func (s *authenticatedQuotaStub043714) CurrentCredentialDigest(context.Context, string) ([32]byte, error) {
	return s.receipt.CredentialDigest, nil
}

func (s *authenticatedQuotaStub043714) FetchAuthenticatedAtPolicy(
	_ context.Context, _ string, _ string, _ bool,
) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error) {
	s.calls++
	return s.usage, s.receipt, s.err
}

func TestProdex043714AuthenticatedProbeReceiptRequiresRealHTTPIdentity(t *testing.T) {
	account := accountentity.Account{ID: "account-a", Name: "account-a", Enabled: true}
	now := time.Unix(2000, 0)
	used := int64(10)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses, RequestedModel: "gpt-6"}
	for _, tc := range []struct {
		name    string
		usage   quotamodel.Usage
		receipt quotamodel.AuthenticatedReceipt
		err     error
		want    bool
	}{
		{name: "trusted ready", usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, receipt: quotamodel.AuthenticatedReceipt{StartedAt: now.Add(-time.Second), CredentialDigest: [32]byte{1}, Confirmed: true}, want: true},
		{name: "quota missing", usage: quotamodel.Usage{}, receipt: quotamodel.AuthenticatedReceipt{StartedAt: now, CredentialDigest: [32]byte{1}, Confirmed: true}},
		{name: "no HTTP receipt", usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, receipt: quotamodel.AuthenticatedReceipt{StartedAt: now, CredentialDigest: [32]byte{1}}},
		{name: "zero fingerprint", usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, receipt: quotamodel.AuthenticatedReceipt{StartedAt: now, Confirmed: true}},
		{name: "401 cannot qualify", usage: quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}}, err: errors.New("HTTP 401")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &authenticatedQuotaStub043714{usage: tc.usage, receipt: tc.receipt, err: tc.err}
			status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, stub)
			status.now = func() time.Time { return now }
			defer status.Close()
			old := status.ProbeRefreshRevision()
			if err := status.ScheduleAuthenticatedRecoveryProbe(t.Context(), account, selection); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if !status.WaitProbeRefresh(ctx, old) {
				t.Fatal("bounded worker did not report completion")
			}
			availability, receipt, found := status.AuthenticatedRecoveryReceipt(account.ID, selection)
			if found != tc.want || found && (!availability.Ready || !receipt.Confirmed || receipt.CredentialDigest != tc.receipt.CredentialDigest) {
				t.Fatalf("cached/noncredential proof accepted: found=%t availability=%+v receipt=%+v", found, availability, receipt)
			}
			if stub.calls != 1 {
				t.Fatalf("authenticated probe calls=%d want 1", stub.calls)
			}
		})
	}
}

func TestProdex043714NewerQuotaObservationInvalidatesPreviousAuthProof(t *testing.T) {
	account := accountentity.Account{ID: "account-a", Name: "account-a", Enabled: true}
	now := time.Unix(5000, 0)
	used := int64(10)
	stub := &authenticatedQuotaStub043714{
		usage:   quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
		receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: [32]byte{1}, StartedAt: now.Add(-time.Second)},
	}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, stub)
	status.now = func() time.Time { return now }
	defer status.Close()
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	before := status.ProbeRefreshRevision()
	if err := status.ScheduleAuthenticatedRecoveryProbe(t.Context(), account, selection); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(waitCtx, before) {
		t.Fatal("authenticated probe failed to finish")
	}
	if _, _, ok := status.AuthenticatedRecoveryReceipt(account.ID, selection); !ok {
		t.Fatal("confirmed proof absent")
	}
	newer := int64(100)
	generation := status.probeSequence.Add(1)
	if !status.applySuccessfulUsageProbe043714(t.Context(), account.ID, "/managed/account-a", quotamodel.Usage{
		Primary: &quotamodel.Window{UsedPercent: &newer},
	}, now, generation, true) {
		t.Fatal("newer independent quota result was rejected")
	}
	if _, _, ok := status.AuthenticatedRecoveryReceipt(account.ID, selection); ok {
		t.Fatal("stale ready quota credential receipt survived newer exhausted observation")
	}
}

type rotatingDuringApplyQuota043714 struct {
	authenticatedQuotaStub043714
	current [32]byte
	reads   int
}

func (gateway *rotatingDuringApplyQuota043714) FetchAuthenticatedAtPolicy(
	ctx context.Context, home, upstream string, noProxy bool,
) (quotamodel.Usage, quotamodel.AuthenticatedReceipt, error) {
	usage, receipt, err := gateway.authenticatedQuotaStub043714.FetchAuthenticatedAtPolicy(ctx, home, upstream, noProxy)
	gateway.current = [32]byte{9, 8, 7}
	return usage, receipt, err
}

func (gateway *rotatingDuringApplyQuota043714) CurrentCredentialDigest(context.Context, string) ([32]byte, error) {
	gateway.reads++
	return gateway.current, nil
}

func TestProdex043714CredentialRotationBeforeApplyDoesNotStoreStaleQuota(t *testing.T) {
	now := time.Unix(6000, 0)
	used := int64(10)
	account := accountentity.Account{ID: "account-a", Enabled: true}
	reader := &rotatingDuringApplyQuota043714{
		authenticatedQuotaStub043714: authenticatedQuotaStub043714{
			usage:   quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
			receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: [32]byte{1, 2, 3}, StartedAt: now.Add(-time.Second)},
		},
		current: [32]byte{1, 2, 3},
	}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, reader)
	status.now = func() time.Time { return now }
	defer status.Close()
	store := &memoryUsageSnapshotStore{}
	status.SetUsageSnapshotStore(store)
	selection := quotamodel.Selection{RouteKind: quotamodel.RouteKindResponses}
	revision := status.ProbeRefreshRevision()
	if err := status.ScheduleAuthenticatedRecoveryProbe(t.Context(), account, selection); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if !status.WaitProbeRefresh(ctx, revision) {
		t.Fatal("quota probe did not complete")
	}
	if reader.reads != 1 {
		t.Fatalf("credential identity checks = %d, want 1 before applying the probe", reader.reads)
	}
	if _, ok := store.values[account.ID]; ok {
		t.Fatal("quota result from a rotated credential was persisted")
	}
	if status.QuotaObservationRevision(account.ID) != 0 {
		t.Fatal("quota result from a rotated credential advanced the observation revision")
	}
	if _, _, ok := status.AuthenticatedRecoveryReceipt(account.ID, selection); ok {
		t.Fatal("quota result from a rotated credential authorized recovery")
	}
}
