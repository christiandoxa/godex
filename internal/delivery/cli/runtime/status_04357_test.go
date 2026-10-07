package runtime

import (
	"fmt"
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestProdex04357StatusFieldsMatchTaggedOverviewSurface(t *testing.T) {
	reset := time.Date(2026, 6, 20, 2, 30, 0, 0, time.UTC).Unix()
	cpu := 12.5
	overview := runtimemodel.Overview{
		ActiveProfile:  "configured-main",
		RuntimeProfile: "runtime-backup",
		ProfileCount:   3,
		Quota: quotamodel.StatusSummary{
			CompatibleProfiles: 2, UnavailableProfiles: 1,
			FiveHour: quotamodel.StatusWindowSummary{Profiles: 2, TotalRemaining: 140, EarliestResetAt: reset},
			Weekly:   quotamodel.StatusWindowSummary{Profiles: 1, TotalRemaining: 80},
		},
		TokenSummary: runtimemodel.TokenUsageSummary{
			LogCount: 1, EventCount: 2,
			Total: runtimemodel.TokenUsageCounts{InputTokens: 110, CachedInputTokens: 25, OutputTokens: 44, ReasoningTokens: 9},
			ByProfile: map[string]runtimemodel.TokenUsageCounts{
				"backup": {InputTokens: 10, OutputTokens: 4, ReasoningTokens: 1},
				"main":   {InputTokens: 100, CachedInputTokens: 25, OutputTokens: 40, ReasoningTokens: 8},
			},
		},
		TokenHistory: []uint64{1, 2, 3},
		TokenFirstAt: "2026-06-20 01:00:00",
		TokenLastAt:  "2026-06-20 01:05:00",
		RuntimeLoad: runtimemodel.RuntimeLoadSummary{
			LogCount: 1, ActiveInflightUnits: 3, RecentSelectionEvents: 4,
			RecentFirstUnixMilli: 100_000, RecentLastUnixMilli: 3_760_000,
		},
		UpdatedAt: "2026-06-20 01:06:00",
	}
	resources := statusResourceSnapshot{
		available: true, processCount: 3, runtimeProcessCount: 1, cpuPercent: &cpu,
		residentBytes: 1536, memoryTotalBytes: 3072,
		socketCount: 2, networkRXQueueBytes: 512, networkTXQueueBytes: 1024,
		diskReadBytes: 2048, diskReadBytesPerSecond: 512,
		diskWriteBytes: 4096, diskWriteBytesPerSecond: 1024,
	}
	fields := statusFields(overview, resources)
	wantLabels := []string{
		"Profile", "5h quota", "5h runway", "Weekly quota", "Weekly runway",
		"Token usage", "Token efficiency", "Token history", "Processes", "Memory",
		"Network", "Disk I/O", "Recent load", "Updated",
	}
	if len(fields) != len(wantLabels) {
		t.Fatalf("status field count = %d: %#v", len(fields), fields)
	}
	values := make(map[string]string, len(fields))
	for index, field := range fields {
		if field[0] != wantLabels[index] {
			t.Fatalf("field %d label = %q, want %q; %#v", index, field[0], wantLabels[index], fields)
		}
		values[field[0]] = field[1]
	}
	resetText := time.Unix(reset, 0).In(time.Local).Format("2006-01-02 15:04:05")
	want := map[string]string{
		"Profile":          "runtime=runtime-backup, configured=configured-main, pool=3, quota-compatible=2, unavailable=1",
		"5h quota":         "140% across 2 profile(s); earliest reset " + resetText,
		"5h runway":        "Unavailable (no recent quota decay observed in active runtime logs)",
		"Weekly quota":     "80% across 1 profile(s)",
		"Weekly runway":    "Unavailable (no recent quota decay observed in active runtime logs)",
		"Token usage":      "2 event(s), logs=1: input=110, cached_input=25, output=44, reasoning=9; by profile: backup:10 in/0 cached/4 out/1 reasoning; main:100 in/25 cached/40 out/8 reasoning",
		"Token efficiency": "cache hit 22.7% · output share 28.6%",
		"Token history":    "▃▅█ 2026-06-20 01:00:00 → 2026-06-20 01:05:00",
		"Processes":        "3 total, 1 runtime; CPU 12.5%",
		"Memory":           "1.5 KiB (50.0% host)",
		"Network":          "2 sockets; RX queue 512 B, TX queue 1.0 KiB",
		"Disk I/O":         "read 2.0 KiB total (512 B/s), write 4.0 KiB total (1.0 KiB/s)",
		"Recent load":      "4 selection event(s) over 1h 1m; inflight units 3; 1 active runtime log(s)",
		"Updated":          "2026-06-20 01:06:00",
	}
	for label, expected := range want {
		if got := values[label]; got != expected {
			t.Fatalf("%s = %q, want %q; all=%s", label, got, expected, fmt.Sprint(fields))
		}
	}
}
