package quota

import (
	"errors"
	"strings"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

func TestSingleProfileOnceReturnsProbeFailureWithoutOutput(t *testing.T) {
	failure := errors.New("synthetic quota failure")
	for _, arguments := range [][]string{{"--profile", "main", "--once"}, {"main", "--once"}, {"-p", "main", "--detail", "--once"}} {
		status := &fakeStatus{reports: []quotamodel.Report{{ProfileName: "main", State: "error", Err: failure}}}
		var output strings.Builder
		if err := Show(t.Context(), status, &output, arguments); !errors.Is(err, failure) || output.Len() != 0 {
			t.Fatalf("args=%q err=%v output=%q", arguments, err, output.String())
		}
	}
}

func TestAggregateOnceKeepsIndividualProbeFailures(t *testing.T) {
	status := &fakeStatus{reports: []quotamodel.Report{{ProfileName: "main", State: "error", Err: errors.New("synthetic quota failure")}}}
	var output strings.Builder
	if err := Show(t.Context(), status, &output, []string{"--all", "--once"}); err != nil || !strings.Contains(output.String(), "main") || !strings.Contains(output.String(), "error") {
		t.Fatalf("err=%v output=%q", err, output.String())
	}
}
