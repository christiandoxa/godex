package quota

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

const manualRedeemNearReset = time.Hour

type redeemProfileSource interface {
	QuotaTargets(context.Context) ([]profilemodel.QuotaTarget, error)
}

type redeemGateway interface {
	FetchAtPolicy(context.Context, string, string, bool) (quotamodel.Usage, error)
	ConsumeResetCredit(context.Context, string, string, bool, string) (quotamodel.RedeemOutcome, error)
}

type Redeemer struct {
	profiles redeemProfileSource
	gateway  redeemGateway
	now      func() time.Time
	random   io.Reader
}

func NewRedeemer(profiles redeemProfileSource, gateway redeemGateway) *Redeemer {
	return &Redeemer{profiles: profiles, gateway: gateway, now: time.Now, random: rand.Reader}
}

func (redeemer *Redeemer) Prepare(ctx context.Context, input quotamodel.RedeemInput) (quotamodel.RedeemPlan, error) {
	if redeemer == nil || redeemer.profiles == nil || redeemer.gateway == nil {
		return quotamodel.RedeemPlan{}, errors.New("redeem support is not configured")
	}
	target, err := redeemer.resolveTarget(ctx, input.Profile)
	if err != nil {
		return quotamodel.RedeemPlan{}, err
	}
	if target.Provider != "openai" {
		return quotamodel.RedeemPlan{}, fmt.Errorf("profile %q is not an OpenAI/Codex profile and cannot redeem reset credits", input.Profile)
	}
	if !target.Compatible {
		return quotamodel.RedeemPlan{}, fmt.Errorf("profile %q is not authenticated with ChatGPT", input.Profile)
	}
	usage, err := redeemer.gateway.FetchAtPolicy(ctx, target.CodexHome, input.BaseURL, input.NoProxy)
	if err != nil {
		return quotamodel.RedeemPlan{}, err
	}
	return quotamodel.RedeemPlan{
		Profile: input.Profile, CodexHome: target.CodexHome,
		BaseURL: input.BaseURL, NoProxy: input.NoProxy,
		NearReset: nearestRedeemReset(usage, redeemer.now(), manualRedeemNearReset),
	}, nil
}

func (redeemer *Redeemer) Execute(ctx context.Context, plan quotamodel.RedeemPlan) (quotamodel.RedeemResult, error) {
	if redeemer == nil || redeemer.profiles == nil || redeemer.gateway == nil {
		return quotamodel.RedeemResult{}, errors.New("redeem support is not configured")
	}
	target, err := redeemer.resolveTarget(ctx, plan.Profile)
	if err != nil {
		return quotamodel.RedeemResult{}, err
	}
	if target.Provider != "openai" || !target.Compatible || target.CodexHome != plan.CodexHome {
		return quotamodel.RedeemResult{}, errors.New("redeem profile changed after confirmation; retry the command")
	}
	requestID, err := manualRedeemRequestID(redeemer.now(), redeemer.random)
	if err != nil {
		return quotamodel.RedeemResult{}, err
	}
	outcome, err := redeemer.gateway.ConsumeResetCredit(ctx, plan.CodexHome, plan.BaseURL, plan.NoProxy, requestID)
	if err != nil {
		return quotamodel.RedeemResult{}, err
	}
	return quotamodel.RedeemResult{Profile: plan.Profile, Outcome: outcome, RequestID: requestID}, nil
}

func (redeemer *Redeemer) resolveTarget(ctx context.Context, name string) (profilemodel.QuotaTarget, error) {
	if name == "" {
		return profilemodel.QuotaTarget{}, errors.New("redeem requires a profile name")
	}
	targets, err := redeemer.profiles.QuotaTargets(ctx)
	if err != nil {
		return profilemodel.QuotaTarget{}, err
	}
	for _, target := range targets {
		if target.Name == name {
			return target, nil
		}
	}
	return profilemodel.QuotaTarget{}, fmt.Errorf("profile %q is missing", name)
}

func nearestRedeemReset(usage quotamodel.Usage, now time.Time, threshold time.Duration) *quotamodel.NearReset {
	var nearest *quotamodel.NearReset
	for _, candidate := range []struct {
		label  string
		window *quotamodel.Window
	}{{"5h", usage.Primary}, {"weekly", usage.Secondary}} {
		if candidate.window == nil || candidate.window.ResetAt == nil {
			continue
		}
		resetAt := *candidate.window.ResetAt
		secondsUntil := resetAt - now.Unix()
		if secondsUntil > int64(threshold/time.Second) {
			continue
		}
		if nearest == nil || resetAt < nearest.ResetAt {
			nearest = &quotamodel.NearReset{Label: candidate.label, ResetAt: resetAt}
		}
	}
	return nearest
}

func manualRedeemRequestID(now time.Time, random io.Reader) (string, error) {
	id, err := redeemUUIDv7(now, random)
	if err != nil {
		return "", err
	}
	return "prodex-manual-redeem-" + formatUUID(id), nil
}

func formatUUID(id [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(id[0:4]),
		binary.BigEndian.Uint16(id[4:6]),
		binary.BigEndian.Uint16(id[6:8]),
		binary.BigEndian.Uint16(id[8:10]),
		id[10:16],
	)
}
