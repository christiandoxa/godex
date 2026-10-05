package routing

import (
	"errors"
	"strings"
	"time"
)

const (
	MaxRouteHealthScore      = 16
	RouteHealthDecayInterval = time.Minute
	RouteHealthRetention     = 14 * 24 * time.Hour
)

type RouteHealthScore struct {
	AccountID   string `json:"account_id"`
	Route       string `json:"route"`
	Score       uint8  `json:"score"`
	UpdatedUnix int64  `json:"updated_unix"`
}

func (score RouteHealthScore) Validate() error {
	if !validAccountID(score.AccountID) {
		return errors.New("invalid routing health account")
	}
	if !validRoute(score.Route) {
		return errors.New("invalid routing health route")
	}
	if score.Score > MaxRouteHealthScore || score.UpdatedUnix < 0 {
		return errors.New("invalid routing health score")
	}
	return nil
}

func validRoute(route string) bool {
	switch route {
	case "standard", "responses", "compact", "websocket":
		return true
	default:
		return false
	}
}

func validAccountID(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if !((character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '-' || character == '_' || character == '.') {
			return false
		}
	}
	return true
}

func ValidateAccountID(value string) error {
	if !validAccountID(value) {
		return errors.New("invalid routing account")
	}
	return nil
}

func (score RouteHealthScore) Effective(now time.Time) uint8 {
	if score.Score == 0 || now.Unix() <= score.UpdatedUnix {
		return score.Score
	}
	decay := (now.Unix() - score.UpdatedUnix) / int64(RouteHealthDecayInterval/time.Second)
	if decay >= int64(score.Score) {
		return 0
	}
	return score.Score - uint8(decay)
}

func (score RouteHealthScore) Adjust(delta int, now time.Time) (RouteHealthScore, error) {
	if err := score.Validate(); err != nil {
		return RouteHealthScore{}, err
	}
	if delta != -1 && delta != 1 {
		return RouteHealthScore{}, errors.New("invalid routing health adjustment")
	}
	value := int(score.Effective(now)) + delta
	value = max(0, min(MaxRouteHealthScore, value))
	score.Score = uint8(value)
	score.UpdatedUnix = max(score.UpdatedUnix, now.Unix())
	return score, nil
}
