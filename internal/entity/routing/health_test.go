package routing

import (
	"strings"
	"testing"
	"time"
)

func TestRouteHealthScoreDecaysAndAdjusts(t *testing.T) {
	updated := time.Unix(100, 0)
	score := RouteHealthScore{AccountID: strings.Repeat("a", 32), Route: "responses", Score: 4, UpdatedUnix: updated.Unix()}
	if got := score.Effective(updated.Add(2 * time.Minute)); got != 2 {
		t.Fatalf("effective score = %d, want 2", got)
	}
	if got := score.Effective(updated.Add(4 * time.Minute)); got != 0 {
		t.Fatalf("expired score = %d, want 0", got)
	}
	adjusted, err := score.Adjust(1, updated.Add(2*time.Minute))
	if err != nil || adjusted.Score != 3 || adjusted.UpdatedUnix != updated.Add(2*time.Minute).Unix() {
		t.Fatalf("adjusted score = %+v, error = %v", adjusted, err)
	}
}

func TestRouteHealthScoreBoundsAndValidates(t *testing.T) {
	base := RouteHealthScore{AccountID: strings.Repeat("b", 32), Route: "standard"}
	for range MaxRouteHealthScore + 2 {
		var err error
		base, err = base.Adjust(1, time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
	}
	if base.Score != MaxRouteHealthScore {
		t.Fatalf("bounded score = %d, want %d", base.Score, MaxRouteHealthScore)
	}
	for _, value := range []RouteHealthScore{
		{AccountID: "not an id", Route: "standard"},
		{AccountID: strings.Repeat("c", 32), Route: "other"},
		{AccountID: strings.Repeat("c", 32), Route: "standard", Score: MaxRouteHealthScore + 1},
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid score accepted: %+v", value)
		}
	}
}

func TestProdex04355RouteHealthMaxScoreIsSixteen(t *testing.T) {
	if MaxRouteHealthScore != 16 {
		t.Fatalf("route health max score = %d, want exact Prodex value 16", MaxRouteHealthScore)
	}
}
