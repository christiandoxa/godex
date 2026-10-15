package quota

import (
	"context"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type currentQuotaCredential043714 struct {
	authenticatedQuotaStub043714
	current [32]byte
}

func (g *currentQuotaCredential043714) CurrentCredentialDigest(context.Context, string) ([32]byte, error) {
	return g.current, nil
}

// Even after a valid 2xx quota receipt, the user may replace the managed
// credentials before recovery is applied. The old receipt may not authorize
// a new unrelated bearer; it must be invalidated, not merely time-limited.
func TestProdex043714QuotaRecoveryReceiptRejectsCredentialChangedAfterProbe(t *testing.T) {
	now := time.Unix(5000, 0)
	used := int64(15)
	first := [32]byte{1, 2, 3}
	second := [32]byte{9, 8, 7}
	account := accountentity.Account{ID: "synthetic-one", Enabled: true}
	reader := &currentQuotaCredential043714{
		authenticatedQuotaStub043714: authenticatedQuotaStub043714{
			usage:   quotamodel.Usage{Primary: &quotamodel.Window{UsedPercent: &used}},
			receipt: quotamodel.AuthenticatedReceipt{Confirmed: true, CredentialDigest: first, StartedAt: now.Add(-time.Second)},
		},
		current: first,
	}
	status := NewStatus(fakeAccounts{accounts: []accountentity.Account{account}, current: account}, reader)
	status.now = func() time.Time { return now }
	defer status.Close()
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
	if _, _, found := status.AuthenticatedRecoveryReceipt(account.ID, selection); !found {
		t.Fatal("fresh 2xx with matching current credentials was rejected")
	}
	reader.current = second
	if _, _, found := status.AuthenticatedRecoveryReceipt(account.ID, selection); found {
		t.Fatal("old receipt survived a profile credential change")
	}
	reader.current = first
	if _, _, found := status.AuthenticatedRecoveryReceipt(account.ID, selection); found {
		t.Fatal("already revoked receipt was silently reactivated")
	}
}
