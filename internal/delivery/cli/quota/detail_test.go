package quota

import (
	"context"
	"errors"
	"strings"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

func TestShowDetailedQuota(t *testing.T) {
	used, reset, seconds := int64(20), int64(1790812800), int64(18000)
	weeklyUsed, weeklyReset, weeklySeconds := int64(100), int64(1791417600), int64(604800)
	zero := int64(0)
	usage := quotamodel.Usage{
		PlanType:  "plus",
		Primary:   &quotamodel.Window{UsedPercent: &used, ResetAt: &reset, LimitWindowSeconds: &seconds},
		Secondary: &quotamodel.Window{UsedPercent: &weeklyUsed, ResetAt: &weeklyReset, LimitWindowSeconds: &weeklySeconds},
	}
	tests := []struct {
		name   string
		report quotamodel.Report
		row    string
	}{
		{"both windows", quotamodel.Report{AccountName: "work", Active: true, Enabled: true, State: "exhausted", Usage: usage},
			"work\t*\texhausted\tplus\t80%\t0%\t2026-10-01T00:00:00Z\t18000\t2026-10-08T00:00:00Z\t604800\n"},
		{"missing windows", quotamodel.Report{AccountName: "work", Enabled: true, State: "ready"},
			"work\t\tready\t-\t-\t-\t-\t-\t-\t-\n"},
		{"partial window", quotamodel.Report{AccountName: "work", Enabled: true, State: "ready", Usage: quotamodel.Usage{Primary: &quotamodel.Window{ResetAt: &reset}}},
			"work\t\tready\t-\t-\t-\t2026-10-01T00:00:00Z\t-\t-\t-\n"},
		{"zero values", quotamodel.Report{AccountName: "work", Enabled: true, State: "ready", Usage: quotamodel.Usage{Secondary: &quotamodel.Window{ResetAt: &zero, LimitWindowSeconds: &zero}}},
			"work\t\tready\t-\t-\t-\t-\t-\t1970-01-01T00:00:00Z\t0\n"},
		{"probe failure", quotamodel.Report{AccountName: "work", Enabled: true, State: "error", Usage: usage, Err: errors.New("synthetic-secret-must-not-be-displayed")},
			"work\t\terror\t-\t-\t-\t-\t-\t-\t-\n"},
		{"disabled account", quotamodel.Report{AccountName: "work", State: "disabled"},
			"work\t\tdisabled\t-\t-\t-\t-\t-\t-\t-\n"},
	}
	const header = "ACCOUNT\tCURRENT\tSTATE\tPLAN\t5H\tWEEKLY\t5H_RESET_AT\t5H_WINDOW_SECONDS\tWEEKLY_RESET_AT\tWEEKLY_WINDOW_SECONDS\n"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := &fakeStatus{reports: []quotamodel.Report{test.report}}
			var output strings.Builder
			if err := Show(context.Background(), status, &output, []string{"--all", "--detail", "--once"}); err != nil {
				t.Fatal(err)
			}
			if output.String() != header+test.row {
				t.Fatalf("output = %q; want %q", output.String(), header+test.row)
			}
			if !status.options.All {
				t.Fatalf("options = %+v", status.options)
			}
		})
	}
}

func TestDetailPreservesQuotaSelection(t *testing.T) {
	for _, selector := range []string{"", "work"} {
		status := &fakeStatus{}
		if err := Show(context.Background(), status, &strings.Builder{}, append([]string{"--detail"}, strings.Fields(selector)...)); err != nil {
			t.Fatal(err)
		}
		if status.options != (quotausecase.Options{Selector: selector}) {
			t.Fatalf("options = %+v", status.options)
		}
	}
}

type failingWriter struct{ err error }

func (writer failingWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestDetailPropagatesFailures(t *testing.T) {
	want := errors.New("synthetic failure")
	if err := Show(context.Background(), &fakeStatus{}, failingWriter{want}, []string{"--detail"}); !errors.Is(err, want) {
		t.Fatalf("output error = %v", err)
	}
	var output strings.Builder
	if err := Show(context.Background(), &fakeStatus{err: want}, &output, []string{"--detail"}); !errors.Is(err, want) || output.Len() != 0 {
		t.Fatalf("status error/output = %v / %q", err, output.String())
	}
	for _, arguments := range [][]string{{"--detail", "--all", "work"}, {"--detail", "work", "personal"}, {"--detail=true"}} {
		if err := Show(context.Background(), &fakeStatus{}, &output, arguments); err == nil || output.Len() != 0 {
			t.Fatalf("invalid arguments %q: error/output = %v / %q", arguments, err, output.String())
		}
	}
}
