package routing

import (
	"errors"
	"time"
)

const (
	RouteMemoryBadPairing    = "bad_pairing"
	RouteMemoryPerformance   = "performance"
	RouteMemorySuccessStreak = "success_streak"
	RouteMemoryRetention     = 14 * 24 * time.Hour
)

type RouteMemoryScore struct {
	AccountID   string `json:"account_id"`
	Route       string `json:"route"`
	Kind        string `json:"kind"`
	Score       uint8  `json:"score"`
	UpdatedUnix int64  `json:"updated_unix"`
}

func (score RouteMemoryScore) Validate() error {
	if !validAccountID(score.AccountID) || !validRoute(score.Route) || score.UpdatedUnix < 0 {
		return errors.New("invalid routing memory score")
	}
	if score.Score > score.MaxScore() {
		return errors.New("invalid routing memory score")
	}
	return nil
}

func (score RouteMemoryScore) MaxScore() uint8 {
	switch score.Kind {
	case RouteMemoryBadPairing:
		return MaxRouteHealthScore
	case RouteMemoryPerformance:
		return 12
	case RouteMemorySuccessStreak:
		return 3
	default:
		return 0
	}
}

func (score RouteMemoryScore) DecayInterval() time.Duration {
	switch score.Kind {
	case RouteMemoryBadPairing:
		return 180 * time.Second
	case RouteMemoryPerformance, RouteMemorySuccessStreak:
		return 300 * time.Second
	default:
		return time.Second
	}
}

func (score RouteMemoryScore) Effective(now time.Time) uint8 {
	if score.Score == 0 || now.Unix() <= score.UpdatedUnix {
		return score.Score
	}
	decay := (now.Unix() - score.UpdatedUnix) / int64(score.DecayInterval()/time.Second)
	if decay >= int64(score.Score) {
		return 0
	}
	return score.Score - uint8(decay)
}
