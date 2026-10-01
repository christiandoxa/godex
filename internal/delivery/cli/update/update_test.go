package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type fakeUpdater struct {
	report updatemodel.Report
	err    error
}

func (fake fakeUpdater) Run(context.Context) (updatemodel.Report, error) {
	return fake.report, fake.err
}

func TestUpdateRendersReferenceStatesAndDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		report updatemodel.Report
		want   string
	}{
		{"up to date", updatemodel.Report{Installed: "1.2.3", Latest: "1.2.3", Status: updatemodel.UpToDate}, "Status: up to date"},
		{"local newer", updatemodel.Report{Installed: "1.3.0", Latest: "1.2.3", Status: updatemodel.LocalNewer}, "local version is newer"},
		{"updated", updatemodel.Report{Installed: "1.2.4", Latest: "1.2.4", Status: updatemodel.Updated, Stdout: "verified output", Stderr: "safe diagnostic"}, "Status: updated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr strings.Builder
			if err := Run(context.Background(), fakeUpdater{report: test.report}, &stdout, &stderr, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout.String(), test.want) {
				t.Fatalf("stdout = %q", stdout.String())
			}
			if test.report.Stdout != "" && !strings.Contains(stdout.String(), test.report.Stdout) {
				t.Fatalf("missing installer stdout: %q", stdout.String())
			}
			if stderr.String() != test.report.Stderr+map[bool]string{true: "\n"}[test.report.Stderr != ""] {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
	}
}

func TestUpdatePreservesInstallerErrorAfterRenderingDiagnostics(t *testing.T) {
	want := errors.New("installer failed")
	report := updatemodel.Report{Installed: "1.2.3", Latest: "1.2.4", Status: updatemodel.UpdateAvailable, Stderr: "bounded diagnostic"}
	var stdout, stderr strings.Builder
	err := Run(context.Background(), fakeUpdater{report: report, err: want}, &stdout, &stderr, nil)
	if !errors.Is(err, want) || !strings.Contains(stdout.String(), "Status: updating") || stderr.String() != "bounded diagnostic\n" {
		t.Fatalf("error/output = %v / %q / %q", err, stdout.String(), stderr.String())
	}
}

func TestUpdateRejectsArguments(t *testing.T) {
	if err := Run(context.Background(), fakeUpdater{}, &strings.Builder{}, &strings.Builder{}, []string{"extra"}); err == nil {
		t.Fatal("update argument unexpectedly accepted")
	}
}
