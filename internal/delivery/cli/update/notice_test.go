package update

import (
	"context"
	"errors"
	"strings"
	"testing"

	updatemodel "github.com/christiandoxa/godex/internal/model/update"
)

type fakeStatusRunner struct {
	report updatemodel.Report
	err    error
}

func (fake fakeStatusRunner) Status(context.Context) (updatemodel.Report, error) {
	return fake.report, fake.err
}

func TestUpdateNoticeRendersOnlyWhenNewerReleaseExists(t *testing.T) {
	for _, test := range []struct {
		name   string
		report updatemodel.Report
		want   bool
	}{
		{"available", updatemodel.Report{Installed: "1.0.0", Latest: "1.1.0", Status: updatemodel.UpdateAvailable}, true},
		{"current", updatemodel.Report{Installed: "1.1.0", Latest: "1.1.0", Status: updatemodel.UpToDate}, false},
		{"newer local", updatemodel.Report{Installed: "1.2.0", Latest: "1.1.0", Status: updatemodel.LocalNewer}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output strings.Builder
			if err := Notice(context.Background(), fakeStatusRunner{report: test.report}, &output); err != nil {
				t.Fatal(err)
			}
			shown := strings.Contains(output.String(), "Update Available")
			if shown != test.want {
				t.Fatalf("notice shown=%t output=%q", shown, output.String())
			}
		})
	}
}

func TestUpdateNoticeReturnsProbeErrorForCallerToIgnore(t *testing.T) {
	want := errors.New("network unavailable")
	if err := Notice(context.Background(), fakeStatusRunner{err: want}, &strings.Builder{}); !errors.Is(err, want) {
		t.Fatalf("notice error = %v", err)
	}
}
