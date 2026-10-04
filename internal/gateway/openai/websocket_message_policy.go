package openai

import (
	"strings"
	"time"

	proxymodel "github.com/christiandoxa/godex/internal/model/proxy"
)

const (
	websocketPrecommitLookaheadBytes           = 8 << 10
	websocketPrecommitHardAffinityBytes        = 512 << 10
	websocketPrecommitProgressTimeout          = 8 * time.Second
	websocketCommittedStreamIdleTimeout        = 300 * time.Second
	websocketUpstreamMaxFrameBytes      uint64 = 16 << 20
	websocketUpstreamMaxMessageBytes    uint64 = 64 << 20
)

type websocketResponsePlan struct {
	holdPromotionAllowed  bool
	transportRetryAllowed bool
}

func websocketResponsePlanFor(input proxymodel.Request, reusedSession bool) websocketResponsePlan {
	policy := input.WebSocketPolicy
	overridePresent := policy.TurnStateOverride ||
		strings.TrimSpace(input.Header.Get("x-codex-turn-state")) != ""
	fresh := !reusedSession &&
		!policy.RequestPreviousResponse &&
		!policy.RequestTurnState &&
		!overridePresent &&
		policy.PromoteCommittedProfile
	return websocketResponsePlan{
		holdPromotionAllowed:  fresh && !policy.RequestSession,
		transportRetryAllowed: fresh,
	}
}
