package quota

// quotaPagePlan04379 mirrors the profile-window inclusion and hidden-profile
// notice policy in tagged Prodex 0.437.9. All sizes are nonnegative and a nil
// limit means an unbounded noninteractive report.
type quotaPagePlan04379 struct {
	startProfile  int
	hiddenBefore  int
	hiddenAfter   int
	showNotice    bool
	appendSection bool
}

func planQuotaPage04379(total, requestedStart, shown, existingLines, sectionLines int, limit *int) (quotaPagePlan04379, bool) {
	if total < 0 || requestedStart < 0 || shown < 0 || existingLines < 0 || sectionLines < 0 ||
		(limit != nil && *limit < 0) {
		return quotaPagePlan04379{}, false
	}
	start := 0
	if total > 0 {
		start = min(requestedStart, total-1)
	}
	remaining := total - start
	if shown > remaining {
		return quotaPagePlan04379{}, false
	}
	after := remaining - shown
	plan := quotaPagePlan04379{
		startProfile:  start,
		hiddenBefore:  start,
		hiddenAfter:   after,
		showNotice:    (start > 0 || after > 0) && (limit == nil || existingLines < *limit),
		appendSection: shown < remaining,
	}
	if plan.appendSection && limit != nil {
		available := max(0, *limit-existingLines)
		needNotice := start > 0 || shown+1 < remaining
		plan.appendSection = sectionLines <= available && (!needNotice || sectionLines < available)
	}
	return plan, true
}
