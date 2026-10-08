package runtime

import (
	"strings"

	sessionentity "github.com/christiandoxa/godex/internal/entity/session"
	sessionmodel "github.com/christiandoxa/godex/internal/model/session"
)

// A new Codex child may legitimately create several session files
// concurrently. Without exactly one newly discovered identity, recovery
// cannot determine which turn the failing process owns and must fail
// closed rather than replay into an arbitrary session.
func newSessionAfter04360(before, after []sessionmodel.Report) (sessionmodel.Report, bool) {
	known := make(map[string]bool, len(before))
	for _, report := range before {
		if report.ID != "" {
			known[report.ID] = true
		}
	}
	var candidate sessionmodel.Report
	found := false
	for _, report := range after {
		if report.ID == "" || known[report.ID] {
			continue
		}
		if found {
			return sessionmodel.Report{}, false
		}
		if !sessionentity.ValidID(report.ID) || strings.TrimSpace(report.Path) == "" ||
			(report.Source != "" && report.Source != "exec") {
			return sessionmodel.Report{}, false
		}
		candidate = report
		found = true
	}
	return candidate, found
}
