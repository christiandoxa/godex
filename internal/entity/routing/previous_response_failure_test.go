package routing

import (
	"strings"
	"testing"
	"time"
)

func TestPreviousResponseFailureThresholdAndDecay(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	failure := PreviousResponseFailure{
		AccountID: "account-a", ResponseKey: strings.Repeat("a", 64), Route: "responses",
	}
	first, err := failure.Record(now)
	if err != nil || first.Score != 1 {
		t.Fatalf("first failure = %#v, %v", first, err)
	}
	second, err := first.Record(now)
	if err != nil || second.Score != PreviousResponseFailureThreshold {
		t.Fatalf("second failure = %#v, %v", second, err)
	}
	if got := second.Effective(now.Add(179 * time.Second)); got != 2 {
		t.Fatalf("score before decay interval = %d, want 2", got)
	}
	if got := second.Effective(now.Add(180 * time.Second)); got != 1 {
		t.Fatalf("score after one decay interval = %d, want 1", got)
	}
	if got := second.Effective(now.Add(360 * time.Second)); got != 0 {
		t.Fatalf("score after two decay intervals = %d, want 0", got)
	}
}

func TestPreviousResponseFailureExpiredScoreStartsAtOne(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	failure := PreviousResponseFailure{
		AccountID: "account-a", ResponseKey: strings.Repeat("b", 64), Route: "responses",
		Score: 1, UpdatedUnix: now.Add(-PreviousResponseFailureDecay - time.Second).Unix(),
	}
	updated, err := failure.Record(now)
	if err != nil || updated.Score != 1 {
		t.Fatalf("failure after expiry = %#v, %v", updated, err)
	}
}

func TestPreviousResponseFailureValidation(t *testing.T) {
	valid := PreviousResponseFailure{
		AccountID: "account-a", ResponseKey: strings.Repeat("c", 64), Route: "responses", UpdatedUnix: 1,
	}
	for _, fixture := range []struct {
		name   string
		change func(*PreviousResponseFailure)
	}{
		{name: "raw response id", change: func(value *PreviousResponseFailure) { value.ResponseKey = "resp-secret" }},
		{name: "unknown route", change: func(value *PreviousResponseFailure) { value.Route = "other" }},
		{name: "score over limit", change: func(value *PreviousResponseFailure) { value.Score = PreviousResponseFailureMaxScore + 1 }},
		{name: "negative timestamp", change: func(value *PreviousResponseFailure) { value.UpdatedUnix = -1 }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			failure := valid
			fixture.change(&failure)
			if err := failure.Validate(); err == nil {
				t.Fatal("invalid failure validated")
			}
		})
	}
}
