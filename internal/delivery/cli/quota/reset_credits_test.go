package quota

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestResetCreditSummaryAppearsInOnceAndWatch(t *testing.T) {
	epoch := int64(1792693425)
	for _, credits := range []*quotamodel.ResetCredits{{AvailableCount: 3, NextExpiry: &epoch}, {AvailableCount: 2, NeverExpires: true}, {AvailableCount: 1}, {AvailableCount: 0}} {
		var output strings.Builder
		reports := []quotamodel.Report{{ProfileName: "main", Provider: "openai", Enabled: true, State: "ready", Usage: quotamodel.Usage{ResetCredits: credits}}}
		if err := writeQuotaReports(&output, reports, false); err != nil {
			t.Fatal(err)
		}
		want := "next expiry: unavailable"
		if credits.NextExpiry != nil {
			want = "next expiry: " + time.Unix(epoch, 0).Local().Format("2006-01-02 15:04:05 -07:00")
		} else if credits.NeverExpires {
			want = "next expiry: never"
		} else if credits.AvailableCount == 0 {
			want = "0 available"
		}
		model := newQuotaTUIModel(t.Context(), &fakeStatus{}, showOptions{})
		updated, _ := model.Update(quotaSnapshotMsg{reports: reports})
		for _, view := range []string{output.String(), updated.View()} {
			if !strings.Contains(view, want) || strings.Count(view, "next expiry:") != map[bool]int{true: 1, false: 0}[credits.AvailableCount > 0] {
				t.Fatalf("missing or repeated expiry: %s", view)
			}
		}
		narrow, _ := updated.Update(tea.WindowSizeMsg{Width: 30, Height: 24})
		view := narrow.View()
		if !strings.Contains(strings.Join(strings.Fields(view), " "), strings.Join(strings.Fields(want), " ")) {
			t.Fatalf("narrow view lost expiry: %s", view)
		}
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > 30 {
				t.Fatalf("narrow credit row overflowed: %q", line)
			}
		}
	}
}
