package routing

import (
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func quotaDSTLocation04383(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("IANA timezone %s unavailable: %v", name, err)
	}
	return loc
}

// The tagged chrono::Local.from_local_datetime(...).single().or_else(earliest)
// returns None on nonexistent DST spring-forward times. On fall-back
// ambiguous times, chrono 0.4.45's earliest() selects the first/standard
// local offset (-05 NY, +01 Berlin, +10 Sydney), not Go time.Date's
// preference for the earlier DST occurrence.
func TestProdex04383QuotaRetryLocalDateTimeMatchesChronoDST(t *testing.T) {
	for _, tc := range []struct {
		name, zone, message string
		nowY                int
		nowM                time.Month
		nowD, nowH, nowMin  int
		epoch               int64
		valid               bool
	}{
		{"new-york-spring-gap", "America/New_York", "Mar 8th, 2026 2:30 AM", 2026, time.March, 8, 1, 45, 0, false},
		{"new-york-valid-after-gap", "America/New_York", "Mar 8th, 2026 3:30 AM", 2026, time.March, 8, 1, 45, 1772955000, true},
		{"new-york-fall-repeated-hour", "America/New_York", "Nov 1st, 2026 1:30 AM", 2026, time.November, 1, 0, 30, 1793514600, true},
		{"berlin-fall-repeated-hour", "Europe/Berlin", "Oct 25th, 2026 2:30 AM", 2026, time.October, 25, 0, 30, 1792891800, true},
		{"sydney-fall-repeated-hour", "Australia/Sydney", "Apr 5th, 2026 2:30 AM", 2026, time.April, 5, 0, 30, 1775320200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			loc := quotaDSTLocation04383(t, tc.zone)
			now := time.Date(tc.nowY, tc.nowM, tc.nowD, tc.nowH, tc.nowMin, 0, 0, loc)
			epoch, valid := quotaResetAtFromText("Usage limit; try again at "+tc.message+".", now)
			if valid != tc.valid || (valid && epoch != tc.epoch) {
				t.Fatalf("chrono local %s: got=%d valid=%t want=%d valid=%t", tc.message, epoch, valid, tc.epoch, tc.valid)
			}
		})
	}
}

func TestProdex04383QuotaRetryLocalClockUsesChronoDST(t *testing.T) {
	loc := quotaDSTLocation04383(t, "America/New_York")
	for _, tc := range []struct {
		name, clock       string
		year              int
		month             time.Month
		day, hour, minute int
		expect            int64
		valid             bool
	}{
		{"spring-gap", "2:30 AM", 2026, time.March, 8, 1, 45, 0, false},
		{"spring-valid", "3:30 AM", 2026, time.March, 8, 1, 45, 1772955000, true},
		{"fall-standard-first", "1:30 AM", 2026, time.November, 1, 0, 30, 1793514600, true},
		{"fall-from-earlier-fold", "1:30 AM", 2026, time.November, 1, 1, 45, 1793514600, true},
		{"previous-day-next-spring-gap", "2:30 AM", 2026, time.March, 7, 23, 30, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(tc.year, tc.month, tc.day, tc.hour, tc.minute, 0, 0, loc)
			epoch, valid := quotaResetAtFromText("Try again at "+tc.clock+".", now)
			if valid != tc.valid || (valid && epoch != tc.expect) {
				t.Fatalf("DST clock %s got=%d valid=%t want=%d valid=%t", tc.clock, epoch, valid, tc.expect, tc.valid)
			}
		})
	}
}

func TestProdex04383SpringGapCannotImposeNextDayQuotaQuarantine(t *testing.T) {
	loc := quotaDSTLocation04383(t, "America/New_York")
	now := time.Date(2026, time.March, 8, 1, 45, 0, 0, loc)
	router := &Router{now: func() time.Time { return now }}
	for _, body := range []string{"try again at 2:30 AM.", "try again at Mar 8th, 2026 2:30 AM."} {
		reset := quotaResetAtFromMessage([]byte(body), now)
		wait := router.quotaQuarantineDuration("account-a", quotamodel.Selection{}, reset)
		if reset != 0 || wait != prodex04357QuotaQuarantineFallback {
			t.Fatalf("DST gap %q produced reset=%d delay=%s", body, reset, wait)
		}
	}
}
