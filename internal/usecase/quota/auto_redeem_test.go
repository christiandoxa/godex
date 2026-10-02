package quota

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type autoRedeemGatewayFake struct {
	usage      map[string][]quotamodel.Usage
	outcomes   map[string]quotamodel.RedeemOutcome
	fetchErr   map[string]error
	consumeErr map[string]error
	fetches    []string
	consumes   []string
	requestID  string
}

func (fake *autoRedeemGatewayFake) FetchAtPolicy(_ context.Context, home, _ string, _ bool) (quotamodel.Usage, error) {
	fake.fetches = append(fake.fetches, home)
	if err := fake.fetchErr[home]; err != nil {
		return quotamodel.Usage{}, err
	}
	values := fake.usage[home]
	if len(values) == 0 {
		return quotamodel.Usage{}, errors.New("missing usage fixture")
	}
	value := values[0]
	if len(values) > 1 {
		fake.usage[home] = values[1:]
	}
	return value, nil
}

func (fake *autoRedeemGatewayFake) ConsumeResetCredit(_ context.Context, home, _ string, _ bool, requestID string) (quotamodel.RedeemOutcome, error) {
	fake.consumes = append(fake.consumes, home)
	fake.requestID = requestID
	if err := fake.consumeErr[home]; err != nil {
		return "", err
	}
	return fake.outcomes[home], nil
}

func TestAutoRedeemerDefersWhenAnotherProfileHasWeeklyRemaining(t *testing.T) {
	now := time.Unix(1_000, 0)
	gateway := &autoRedeemGatewayFake{
		usage: map[string][]quotamodel.Usage{
			"/a": {autoRedeemUsage("plus", 100, 100, now.Unix()+10_000, 1)},
			"/b": {autoRedeemUsage("pro", 20, 50, now.Unix()+20_000, 1)},
		},
		outcomes: map[string]quotamodel.RedeemOutcome{"/a": quotamodel.RedeemReset},
	}
	redeemer := NewAutoRedeemer(gateway)
	redeemer.now = func() time.Time { return now }
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{
		{ID: "a", Home: "/a", Enabled: true},
		{ID: "b", Home: "/b", Enabled: true},
	}, "", proxymodel.Request{Body: []byte(`{"model":"gpt-5.3-codex"}`)})
	if err != nil || redeemed || id != "" || len(gateway.consumes) != 0 {
		t.Fatalf("result = id:%q redeemed:%t err:%v consumes:%v", id, redeemed, err, gateway.consumes)
	}
}

func TestAutoRedeemerSelectsBestPoolCandidateAndRefreshesBeforeRetry(t *testing.T) {
	now := time.Unix(1_000, 0)
	gateway := &autoRedeemGatewayFake{
		usage: map[string][]quotamodel.Usage{
			"/pro": {
				autoRedeemUsage("pro", 100, 100, now.Unix()+20_000, 1),
			},
			"/plus-old": {
				autoRedeemUsage("plus", 100, 100, now.Unix()+5_000, 1),
			},
			"/plus-best": {
				autoRedeemUsage("plus", 100, 100, now.Unix()+10_000, 1), // pool snapshot
				autoRedeemUsage("plus", 100, 100, now.Unix()+10_000, 1), // pre-consume refresh
				autoRedeemUsage("plus", 20, 50, now.Unix()+10_000, 0),   // post-consume refresh
			},
		},
		outcomes: map[string]quotamodel.RedeemOutcome{"/plus-best": quotamodel.RedeemReset},
	}
	redeemer := NewAutoRedeemer(gateway)
	redeemer.now = func() time.Time { return now }
	redeemer.random = bytes.NewReader(make([]byte, 16))
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{
		{ID: "pro", Home: "/pro", Enabled: true, RouteOrder: 1},
		{ID: "plus-old", Home: "/plus-old", Enabled: true, RouteOrder: 2},
		{ID: "plus-best", Home: "/plus-best", Enabled: true, RouteOrder: 3},
	}, "", proxymodel.Request{Body: []byte(`{"model":"gpt-5.3-codex"}`)})
	if err != nil || !redeemed || id != "plus-best" {
		t.Fatalf("result = id:%q redeemed:%t err:%v", id, redeemed, err)
	}
	if len(gateway.consumes) != 1 || gateway.consumes[0] != "/plus-best" {
		t.Fatalf("consumes = %#v", gateway.consumes)
	}
	if !strings.HasPrefix(gateway.requestID, "prodex-auto-redeem-") {
		t.Fatalf("request id = %q", gateway.requestID)
	}
	parts := strings.Split(strings.TrimPrefix(gateway.requestID, "prodex-auto-redeem-"), "-")
	if len(parts) != 5 || len(parts[2]) != 4 || parts[2][0] != '7' {
		t.Fatalf("request id is not UUIDv7: %q", gateway.requestID)
	}
}

func TestAutoRedeemerHardAffinityIgnoresBetterPoolProfile(t *testing.T) {
	now := time.Unix(1_000, 0)
	gateway := &autoRedeemGatewayFake{
		usage: map[string][]quotamodel.Usage{
			"/owner": {
				autoRedeemUsage("pro", 100, 100, now.Unix()+10_000, 1),
				autoRedeemUsage("pro", 100, 100, now.Unix()+10_000, 1),
				autoRedeemUsage("pro", 10, 10, now.Unix()+10_000, 0),
			},
			"/other": {autoRedeemUsage("plus", 10, 10, now.Unix()+50_000, 5)},
		},
		outcomes: map[string]quotamodel.RedeemOutcome{"/owner": quotamodel.RedeemAlreadyRedeemed},
	}
	redeemer := NewAutoRedeemer(gateway)
	redeemer.now = func() time.Time { return now }
	redeemer.random = bytes.NewReader(make([]byte, 16))
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{
		{ID: "owner", Home: "/owner", Enabled: true},
		{ID: "other", Home: "/other", Enabled: true},
	}, "owner", proxymodel.Request{Body: []byte(`{"model":"gpt-5.3-codex"}`)})
	if err != nil || !redeemed || id != "owner" || len(gateway.consumes) != 1 || gateway.consumes[0] != "/owner" {
		t.Fatalf("hard-affinity result = id:%q redeemed:%t err:%v consumes:%v", id, redeemed, err, gateway.consumes)
	}
}

func TestAutoRedeemerRejectsRetiredSparkWithoutQuotaCalls(t *testing.T) {
	gateway := &autoRedeemGatewayFake{}
	redeemer := NewAutoRedeemer(gateway)
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, "", proxymodel.Request{Body: []byte(`{"model":"gpt-5.3-codex-spark"}`)})
	if err != nil || redeemed || id != "" || len(gateway.fetches) != 0 {
		t.Fatalf("Spark result = id:%q redeemed:%t err:%v fetches:%v", id, redeemed, err, gateway.fetches)
	}
}

func TestAutoRedeemerDoesNotRetryWhenPostRedeemQuotaStillBlocked(t *testing.T) {
	now := time.Unix(1_000, 0)
	blocked := autoRedeemUsage("plus", 100, 100, now.Unix()+10_000, 1)
	gateway := &autoRedeemGatewayFake{
		usage:    map[string][]quotamodel.Usage{"/a": {blocked, blocked, blocked}},
		outcomes: map[string]quotamodel.RedeemOutcome{"/a": quotamodel.RedeemReset},
	}
	redeemer := NewAutoRedeemer(gateway)
	redeemer.now = func() time.Time { return now }
	redeemer.random = bytes.NewReader(make([]byte, 16))
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{{ID: "a", Home: "/a", Enabled: true}}, "", proxymodel.Request{})
	if err != nil || redeemed || id != "" || len(gateway.consumes) != 1 {
		t.Fatalf("blocked result = id:%q redeemed:%t err:%v consumes:%v", id, redeemed, err, gateway.consumes)
	}
}

func TestAutoRedeemerDefersFreshPoolWhenAnyOpenAIProbeIsUnavailable(t *testing.T) {
	now := time.Unix(1_000, 0)
	gateway := &autoRedeemGatewayFake{
		usage: map[string][]quotamodel.Usage{
			"/openai": {autoRedeemUsage("plus", 100, 100, now.Unix()+10_000, 1)},
		},
		fetchErr: map[string]error{"/unknown": errors.New("probe failed")},
		outcomes: map[string]quotamodel.RedeemOutcome{"/openai": quotamodel.RedeemReset},
	}
	redeemer := NewAutoRedeemer(gateway)
	redeemer.now = func() time.Time { return now }
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{
		{ID: "anthropic", Home: "/anthropic", Enabled: true, Provider: proxymodel.Provider{Kind: "anthropic"}},
		{ID: "unknown", Home: "/unknown", Enabled: true},
		{ID: "openai", Home: "/openai", Enabled: true},
	}, "", proxymodel.Request{})
	if err != nil || redeemed || id != "" || len(gateway.consumes) != 0 {
		t.Fatalf("result = id:%q redeemed:%t err:%v consumes:%v", id, redeemed, err, gateway.consumes)
	}
}

func TestAutoRedeemerHardAffinityProbeFailureDoesNotRedeem(t *testing.T) {
	gateway := &autoRedeemGatewayFake{fetchErr: map[string]error{"/owner": errors.New("probe failed")}}
	redeemer := NewAutoRedeemer(gateway)
	id, redeemed, err := redeemer.Try(context.Background(), []proxymodel.Account{
		{ID: "owner", Home: "/owner", Enabled: true},
	}, "owner", proxymodel.Request{})
	if err != nil || redeemed || id != "" {
		t.Fatalf("result = id:%q redeemed:%t err:%v", id, redeemed, err)
	}
}

func autoRedeemUsage(plan string, primaryUsed, weeklyUsed, weeklyReset, credits int64) quotamodel.Usage {
	primaryReset := weeklyReset - 1_000
	return quotamodel.Usage{
		PlanType:     plan,
		Primary:      &quotamodel.Window{UsedPercent: &primaryUsed, ResetAt: &primaryReset},
		Secondary:    &quotamodel.Window{UsedPercent: &weeklyUsed, ResetAt: &weeklyReset},
		ResetCredits: &quotamodel.ResetCredits{AvailableCount: credits},
	}
}
