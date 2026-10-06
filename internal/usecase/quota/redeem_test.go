package quota

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type fakeRedeemGateway struct {
	usage     quotamodel.Usage
	fetchHome string
	consume   string
	requestID string
	outcome   quotamodel.RedeemOutcome
}

func (fake *fakeRedeemGateway) FetchAtPolicy(_ context.Context, home, _ string, _ bool) (quotamodel.Usage, error) {
	fake.fetchHome = home
	return fake.usage, nil
}

func (fake *fakeRedeemGateway) ConsumeResetCredit(_ context.Context, home, _ string, _ bool, requestID string) (quotamodel.RedeemOutcome, error) {
	fake.consume, fake.requestID = home, requestID
	return fake.outcome, nil
}

func TestRedeemerPreparesNearResetAndExecutesSelectedProfile(t *testing.T) {
	now := time.Unix(1_000, 0)
	near := int64(1_030)
	far := int64(1_000 + 3_601)
	gateway := &fakeRedeemGateway{usage: quotamodel.Usage{
		Primary: &quotamodel.Window{ResetAt: &far}, Secondary: &quotamodel.Window{ResetAt: &near},
	}, outcome: quotamodel.RedeemReset}
	redeemer := NewRedeemer(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "main", CodexHome: "/profiles/main", Provider: "openai", Auth: "chatgpt", Compatible: true,
	}}}, gateway)
	redeemer.now = func() time.Time { return now }
	redeemer.random = bytes.NewReader(make([]byte, 16))

	plan, err := redeemer.Prepare(context.Background(), quotamodel.RedeemInput{Profile: "main", BaseURL: "https://quota.test", NoProxy: true})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NearReset == nil || plan.NearReset.Label != "weekly" || plan.NearReset.ResetAt != near || gateway.fetchHome != "/profiles/main" {
		t.Fatalf("plan = %+v, fetch home = %q", plan, gateway.fetchHome)
	}
	result, err := redeemer.Execute(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != quotamodel.RedeemReset || gateway.consume != "/profiles/main" || !strings.HasPrefix(result.RequestID, "godex-manual-redeem-") {
		t.Fatalf("result = %+v, consume = %q", result, gateway.consume)
	}
	if !strings.Contains(result.RequestID, "-") || result.RequestID != gateway.requestID {
		t.Fatalf("request id = %q / %q", result.RequestID, gateway.requestID)
	}
}

func TestRedeemerRejectsNonOpenAIAndUnauthenticatedProfiles(t *testing.T) {
	for _, target := range []profilemodel.QuotaTarget{
		{Name: "claude", Provider: "anthropic", Auth: "claude-oauth"},
		{Name: "logged-out", Provider: "openai", Auth: "no-auth"},
	} {
		redeemer := NewRedeemer(fakeProfileSource{targets: []profilemodel.QuotaTarget{target}}, &fakeRedeemGateway{})
		if _, err := redeemer.Prepare(context.Background(), quotamodel.RedeemInput{Profile: target.Name}); err == nil {
			t.Fatalf("target %+v unexpectedly redeemable", target)
		}
	}
}

func TestNearestRedeemResetMatchesOneHourGuard(t *testing.T) {
	now := time.Unix(1_000, 0)
	inside := int64(1_000 + 3_600)
	outside := int64(1_000 + 3_601)
	if got := nearestRedeemReset(quotamodel.Usage{Primary: &quotamodel.Window{ResetAt: &outside}}, now, time.Hour); got != nil {
		t.Fatalf("outside reset = %+v", got)
	}
	got := nearestRedeemReset(quotamodel.Usage{Primary: &quotamodel.Window{ResetAt: &inside}}, now, time.Hour)
	if got == nil || got.Label != "5h" || got.ResetAt != inside {
		t.Fatalf("inside reset = %+v", got)
	}
}
