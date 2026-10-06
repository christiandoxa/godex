package quota

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"

	quotaentity "github.com/christiandoxa/godex/internal/entity/quota"
	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type autoRedeemGateway interface {
	FetchAtPolicy(context.Context, string, string, bool) (quotamodel.Usage, error)
	ConsumeResetCredit(context.Context, string, string, bool, string) (quotamodel.RedeemOutcome, error)
}

type AutoRedeemer struct {
	gateway autoRedeemGateway
	now     func() time.Time
	random  io.Reader
}

type autoRedeemSnapshot struct {
	account proxymodel.Account
	usage   quotamodel.Usage
	order   int
}

func NewAutoRedeemer(gateway autoRedeemGateway) *AutoRedeemer {
	return &AutoRedeemer{gateway: gateway, now: time.Now, random: rand.Reader}
}

// Try redeems at most one OpenAI reset credit. preferredID is empty for fresh
// pool selection; a non-empty value models hard conversation affinity and limits
// redemption to that owner profile.
func (redeemer *AutoRedeemer) Try(
	ctx context.Context,
	accounts []proxymodel.Account,
	preferredID string,
	request proxymodel.Request,
) (string, bool, error) {
	if redeemer == nil || redeemer.gateway == nil {
		return "", false, nil
	}
	if !quotaentity.AutoRedeemModelAllowed(autoRedeemRequestModel(request.Body)) {
		return "", false, nil
	}
	snapshots, complete, err := redeemer.snapshots(ctx, accounts, preferredID)
	if err != nil {
		return "", false, err
	}
	if !complete || len(snapshots) == 0 {
		return "", false, nil
	}
	if preferredID == "" && autoRedeemPoolHasWeeklyRemaining(snapshots, redeemer.now()) {
		return "", false, nil
	}
	selected, err := selectAutoRedeemSnapshot(snapshots, redeemer.now())
	if err != nil || selected < 0 {
		return "", false, err
	}
	account := snapshots[selected].account
	return redeemer.redeemAccount(ctx, account, redeemer.now())
}

func (redeemer *AutoRedeemer) snapshots(
	ctx context.Context,
	accounts []proxymodel.Account,
	preferredID string,
) ([]autoRedeemSnapshot, bool, error) {
	snapshots := make([]autoRedeemSnapshot, 0, len(accounts))
	complete := true
	for index, account := range accounts {
		if !autoRedeemOpenAIAccount(account) || preferredID != "" && account.ID != preferredID {
			continue
		}
		usage, err := redeemer.gateway.FetchAtPolicy(ctx, account.Home, "", false)
		if err != nil {
			if ctx.Err() != nil {
				return nil, false, ctx.Err()
			}
			complete = false
			continue
		}
		snapshots = append(snapshots, autoRedeemSnapshot{account: account, usage: usage, order: index})
	}
	return snapshots, complete, nil
}

func autoRedeemPoolHasWeeklyRemaining(snapshots []autoRedeemSnapshot, now time.Time) bool {
	for _, snapshot := range snapshots {
		window := snapshot.usage.Secondary
		if window == nil || window.UsedPercent == nil {
			continue
		}
		if !autoRedeemWindowExhausted(window, now) {
			return true
		}
	}
	return false
}

func selectAutoRedeemSnapshot(snapshots []autoRedeemSnapshot, now time.Time) (int, error) {
	candidates := make([]quotaentity.AutoRedeemCandidate, 0, len(snapshots))
	for _, snapshot := range snapshots {
		candidates = append(candidates, autoRedeemCandidate(snapshot, now))
	}
	return quotaentity.SelectAutoRedeemCandidate(candidates, now.Unix())
}

func autoRedeemCandidate(snapshot autoRedeemSnapshot, now time.Time) quotaentity.AutoRedeemCandidate {
	usage := snapshot.usage
	available := int64(0)
	if usage.ResetCredits != nil {
		available = usage.ResetCredits.AvailableCount
	}
	status := int64(4)
	resetAt := int64(math.MaxInt64)
	if usage.Secondary != nil {
		if usage.Secondary.ResetAt != nil {
			resetAt = *usage.Secondary.ResetAt
		}
		if usage.Secondary.UsedPercent != nil {
			status = 0
			if autoRedeemWindowExhausted(usage.Secondary, now) {
				status = quotaentity.AutoRedeemWeeklyStatusExhausted
			}
		}
	}
	order := int64(snapshot.order)
	if snapshot.account.RouteOrder > 0 {
		order = int64(snapshot.account.RouteOrder)
	}
	return quotaentity.AutoRedeemCandidate{
		PlanType: usage.PlanType, AvailableCount: available,
		WeeklyStatus: status, WeeklyResetAt: resetAt,
		InflightCount: 0, HealthSortKey: 0, OrderIndex: order,
	}
}

func (redeemer *AutoRedeemer) redeemAccount(
	ctx context.Context,
	account proxymodel.Account,
	now time.Time,
) (string, bool, error) {
	usage, err := redeemer.gateway.FetchAtPolicy(ctx, account.Home, "", false)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, nil
	}
	candidate := autoRedeemCandidate(autoRedeemSnapshot{account: account, usage: usage}, now)
	if !quotaentity.AutoRedeemCandidateEligible(candidate, now.Unix()) {
		return "", false, nil
	}
	requestID, err := autoRedeemRequestID(now, redeemer.random)
	if err != nil {
		return "", false, err
	}
	outcome, err := redeemer.gateway.ConsumeResetCredit(ctx, account.Home, "", false, requestID)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, nil
	}
	if outcome != quotamodel.RedeemReset && outcome != quotamodel.RedeemAlreadyRedeemed {
		return "", false, nil
	}
	after, err := redeemer.gateway.FetchAtPolicy(ctx, account.Home, "", false)
	if err != nil {
		if ctx.Err() != nil {
			return "", false, ctx.Err()
		}
		return "", false, nil
	}
	if autoRedeemUsageBlocked(after, redeemer.now()) {
		return "", false, nil
	}
	return account.ID, true, nil
}

func autoRedeemUsageBlocked(usage quotamodel.Usage, now time.Time) bool {
	return autoRedeemWindowExhausted(usage.Primary, now) || autoRedeemWindowExhausted(usage.Secondary, now)
}

func autoRedeemWindowExhausted(window *quotamodel.Window, now time.Time) bool {
	if window == nil || window.UsedPercent == nil || *window.UsedPercent < 100 {
		return false
	}
	return window.ResetAt == nil || *window.ResetAt > now.Unix()
}

func autoRedeemOpenAIAccount(account proxymodel.Account) bool {
	kind := strings.ToLower(strings.TrimSpace(account.Provider.Kind))
	return account.ID != "" && account.Home != "" && account.Enabled && (kind == "" || kind == "openai")
}

func autoRedeemRequestModel(body []byte) string {
	var request struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &request) != nil {
		return ""
	}
	return request.Model
}

func autoRedeemRequestID(now time.Time, random io.Reader) (string, error) {
	id, err := redeemUUIDv7(now, random)
	if err != nil {
		return "", err
	}
	return "godex-auto-redeem-" + formatUUID(id), nil
}

func redeemUUIDv7(now time.Time, random io.Reader) ([16]byte, error) {
	var id [16]byte
	if random == nil {
		return id, errors.New("generate redeem request id")
	}
	if _, err := io.ReadFull(random, id[:]); err != nil {
		return id, errors.New("generate redeem request id")
	}
	millis := uint64(now.UnixMilli())
	id[0] = byte(millis >> 40)
	id[1] = byte(millis >> 32)
	id[2] = byte(millis >> 24)
	id[3] = byte(millis >> 16)
	id[4] = byte(millis >> 8)
	id[5] = byte(millis)
	id[6] = (id[6] & 0x0f) | 0x70
	id[8] = (id[8] & 0x3f) | 0x80
	return id, nil
}
