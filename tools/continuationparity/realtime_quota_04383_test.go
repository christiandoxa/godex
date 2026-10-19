package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProdex04383RealtimeFixtureKeepsEveryRequiredQuotaField(t *testing.T) {
	p := product{name: "prodex", state: t.TempDir()}
	if err := seedProductState(p); err != nil {
		t.Fatal(err)
	}
	if err := seedRealtimeQuota04383(p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(p.state, "runtime-usage-snapshots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Value map[string]map[string]json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Value) != 3 {
		t.Fatalf("quota profiles=%d", len(snapshot.Value))
	}
	// Tagged RuntimeProfileUsageSnapshot requires each of these fields.
	for name, profile := range snapshot.Value {
		for _, key := range []string{"checked_at", "five_hour_status", "five_hour_remaining_percent", "five_hour_reset_at", "weekly_status", "weekly_remaining_percent", "weekly_reset_at"} {
			if len(profile[key]) == 0 {
				t.Errorf("%s quota snapshot lacks required %s", name, key)
			}
		}
		want := `"Exhausted"`
		if name == "main" {
			want = `"Ready"`
		}
		if string(profile["weekly_status"]) != want {
			t.Errorf("%s weekly status=%s want=%s", name, profile["weekly_status"], want)
		}
	}
}
