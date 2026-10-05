package routing

import (
	"encoding/hex"
	"errors"
	"time"
)

const (
	PreviousResponseFailureThreshold = 2
	PreviousResponseFailureMaxScore  = 16
	PreviousResponseFailureDecay     = 180 * time.Second
	PreviousResponseFailureRetention = 14 * 24 * time.Hour
	MaxPreviousResponseFailures      = 4096
)

type PreviousResponseFailure struct {
	AccountID   string `json:"account_id"`
	ResponseKey string `json:"response_key"`
	Route       string `json:"route"`
	Score       uint8  `json:"score"`
	UpdatedUnix int64  `json:"updated_unix"`
}

func (failure PreviousResponseFailure) Validate() error {
	if !validAccountID(failure.AccountID) || len(failure.ResponseKey) != 64 {
		return errors.New("invalid previous response failure")
	}
	if _, err := hex.DecodeString(failure.ResponseKey); err != nil {
		return errors.New("invalid previous response key")
	}
	if !validRoute(failure.Route) || failure.Score > PreviousResponseFailureMaxScore || failure.UpdatedUnix < 0 {
		return errors.New("invalid previous response failure")
	}
	return nil
}

func (failure PreviousResponseFailure) Effective(now time.Time) uint8 {
	if failure.Score == 0 || now.Unix() <= failure.UpdatedUnix {
		return failure.Score
	}
	decay := (now.Unix() - failure.UpdatedUnix) / int64(PreviousResponseFailureDecay/time.Second)
	if decay >= int64(failure.Score) {
		return 0
	}
	return failure.Score - uint8(decay)
}

func (failure PreviousResponseFailure) Record(now time.Time) (PreviousResponseFailure, error) {
	if err := failure.Validate(); err != nil {
		return PreviousResponseFailure{}, err
	}
	failure.Score = min(failure.Effective(now)+1, uint8(PreviousResponseFailureMaxScore))
	failure.UpdatedUnix = max(failure.UpdatedUnix, now.Unix())
	return failure, nil
}
