package runtime

import (
	"context"
	"errors"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type modelReadyQuota04380 struct {
	last   quotamodel.Selection
	ready  bool
	source quotamodel.Source
	err    error
	called bool
}

func (q *modelReadyQuota04380) Ready(context.Context, accountentity.Account) (bool, error) {
	return true, nil // deliberately optimistic: route-specific policy wins
}
func (q *modelReadyQuota04380) AvailabilityForRoute(_ context.Context, _ accountentity.Account, s quotamodel.Selection) (quotamodel.Availability, error) {
	q.last = s
	q.called = true
	return quotamodel.Availability{Ready: q.ready, Source: q.source}, q.err
}

func TestProdex04380NativeRecoveryRequiresModelSpecificQuotaEvidence(t *testing.T) {
	accounts := &fakeLaunchAccounts{accounts: []accountentity.Account{{ID: "a", Name: "a", Enabled: true}}}
	quota := &modelReadyQuota04380{ready: true, source: quotamodel.SourceLive}
	runner := NewRunner(accounts, nil, nil)
	runner.SetQuotaPreflight(quota)
	if !runner.RecoveryQuotaReadyForModel(t.Context(), "a", "gpt-6-luna") || !quota.called ||
		quota.last.RequestedModel != "gpt-6-luna" || quota.last.RouteKind != quotamodel.RouteKindResponses {
		t.Fatalf("model-specific quota not consulted: called=%t selection=%+v", quota.called, quota.last)
	}
	for _, tc := range []struct {
		name   string
		ready  bool
		source quotamodel.Source
		err    error
	}{
		{name: "model quota exhausted", ready: false, source: quotamodel.SourceLive},
		{name: "source unknown", ready: true, source: quotamodel.SourceUnknown},
		{name: "quota transport error", ready: true, source: quotamodel.SourceLive, err: errors.New("synthetic network failure")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			quota.ready, quota.source, quota.err = tc.ready, tc.source, tc.err
			if runner.RecoveryQuotaReadyForModel(t.Context(), "a", "gpt-6-luna") {
				t.Fatal("ordinary Ready=true must not authorize a model-unknown recovery")
			}
		})
	}
	quota.ready, quota.source, quota.err = true, quotamodel.SourceLive, nil
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if runner.RecoveryQuotaReadyForModel(ctx, "a", "gpt-6-luna") {
		t.Fatal("cancelled quota check authorized native recovery")
	}
	if runner.RecoveryQuotaReadyForModel(t.Context(), "unknown", "gpt-6-luna") {
		t.Fatal("unknown profile authorized native recovery")
	}
	accounts.accounts[0].Enabled = false
	if runner.RecoveryQuotaReadyForModel(t.Context(), "a", "gpt-6-luna") {
		t.Fatal("disabled profile authorized native recovery")
	}
}
