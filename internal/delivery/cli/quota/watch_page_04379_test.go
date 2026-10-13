package quota

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

// Tagged Prodex 0.437.9 clamps scroll offset to total-1, regardless
// of how many profile sections could fit in the current viewport.
func TestProdex04379QuotaTUIScrollAllowsLastProfileAsViewportStart(t *testing.T) {
	reports := make([]quotamodel.Report, 7)
	for i := range reports {
		reports[i] = quotamodel.Report{
			ProfileName: fmt.Sprintf("profile-%02d", i+1),
			Provider:    "openai", Auth: "chatgpt", State: "ready",
		}
	}
	model := newQuotaTUIModel(t.Context(), &fakeStatus{reports: reports},
		showOptions{Options: quotausecase.Options{All: true}})
	model.reports = reports
	model.height = 19
	for i := 0; i < 50; i++ {
		updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		model = updated.(quotaTUIModel)
	}
	if model.scrollOffset != len(reports)-1 {
		t.Fatalf("last-profile scroll start = %d, want %d", model.scrollOffset, len(reports)-1)
	}
	view := model.View()
	if !strings.Contains(view, "profile-07") ||
		strings.Contains(view, "profile-06\t") ||
		!strings.Contains(view, "Showing 7-7 of 7 profiles") {
		t.Fatalf("last-page viewport did not clamp to last profile: %q", view)
	}
	model.reports = nil
	model.clampScroll()
	if model.scrollOffset != 0 {
		t.Fatalf("empty profile scroll = %d, want 0", model.scrollOffset)
	}
}

func TestProdex04379QuotaPagePlanMatchesTaggedMojoPagination(t *testing.T) {
	limit := func(value int) *int { return &value }
	for _, test := range []struct {
		name                                   string
		total, start, shown, existing, section int
		limit                                  *int
		want                                   quotaPagePlan04379
		valid                                  bool
	}{
		{
			name: "empty extreme scroll", total: 0, start: int(^uint(0) >> 1),
			want: quotaPagePlan04379{}, valid: true,
		},
		{
			name: "hidden both ends", total: 4, start: 1, shown: 0, existing: 5, section: 3,
			limit: limit(12),
			want:  quotaPagePlan04379{startProfile: 1, hiddenBefore: 1, hiddenAfter: 3, showNotice: true, appendSection: true},
			valid: true,
		},
		{
			name: "reserve hidden notice", total: 4, start: 1, shown: 1, existing: 10, section: 2,
			limit: limit(12),
			want:  quotaPagePlan04379{startProfile: 1, hiddenBefore: 1, hiddenAfter: 2, showNotice: true},
			valid: true,
		},
		{
			name: "last section still reserves above notice", total: 4, start: 1, shown: 2, existing: 10, section: 1,
			limit: limit(12),
			want:  quotaPagePlan04379{startProfile: 1, hiddenBefore: 1, hiddenAfter: 1, showNotice: true, appendSection: true},
			valid: true,
		},
		{
			name: "max scroll clamp", total: 4, start: int(^uint(0) >> 1), section: 1,
			want:  quotaPagePlan04379{startProfile: 3, hiddenBefore: 3, hiddenAfter: 1, showNotice: true, appendSection: true},
			valid: true,
		},
		{
			name: "no room at all", total: 4, start: 1, existing: 12, section: 1, limit: limit(12),
			want:  quotaPagePlan04379{startProfile: 1, hiddenBefore: 1, hiddenAfter: 3},
			valid: true,
		},
		{
			name: "unbounded overflow safe", total: 4, start: 2, existing: int(^uint(0) >> 1), section: int(^uint(0) >> 1),
			want:  quotaPagePlan04379{startProfile: 2, hiddenBefore: 2, hiddenAfter: 2, showNotice: true, appendSection: true},
			valid: true,
		},
		{
			name: "invalid shown outside remainder", total: 4, start: 2, shown: 3, section: 1, valid: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, valid := planQuotaPage04379(test.total, test.start, test.shown, test.existing, test.section, test.limit)
			if valid != test.valid || (valid && got != test.want) {
				t.Fatalf("page plan: got=%+v valid=%t; want=%+v valid=%t", got, valid, test.want, test.valid)
			}
		})
	}
}

func TestProdex04379QuotaTUIViewportReservesNoticeAndFooter(t *testing.T) {
	reports := make([]quotamodel.Report, 12)
	for i := range reports {
		reports[i] = quotamodel.Report{
			ProfileName: fmt.Sprintf("scroll-%02d", i+1),
			Provider:    "openai", Auth: "chatgpt", State: "ready",
		}
	}
	model := newQuotaTUIModel(t.Context(), &fakeStatus{reports: reports},
		showOptions{Options: quotausecase.Options{All: true}})
	model.reports = reports
	model.height = 19
	for _, start := range []int{0, 3, len(reports) - 1} {
		model.scrollOffset = start
		page := model.View()
		lines := strings.Count(page, "\n") + 1
		if lines > model.height {
			t.Fatalf("viewport start=%d used %d rows, limit %d: %q",
				start, lines, model.height, page)
		}
		if !strings.Contains(page, model.footer()) {
			t.Fatalf("viewport start=%d dropped footer: %q", start, page)
		}
		if !strings.Contains(page, "Showing ") {
			t.Fatalf("viewport start=%d omitted hidden profile notice: %q", start, page)
		}
		if start == len(reports)-1 && !strings.Contains(page, "scroll-12") {
			t.Fatalf("last report inaccessible at maximum scroll: %q", page)
		}
	}
	// A line-breaking label must not be allowed to exceed the viewport
	// budget or displace the footer.
	reports[0].ProfileName = "line-break\n" + strings.Repeat("row\n", 15)
	model.scrollOffset = 0
	model.reports = reports
	if page := model.View(); strings.Count(page, "\n")+1 > model.height {
		t.Fatalf("long report expanded beyond quota viewport: %q", page)
	}
}
