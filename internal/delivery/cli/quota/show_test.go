package quota

import (
	"context"
	"strings"
	"testing"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type fakeStatus struct {
	options quotausecase.Options
}

func (fake *fakeStatus) Run(_ context.Context, options quotausecase.Options) ([]quotamodel.Report, error) {
	fake.options = options
	used := int64(20)
	return []quotamodel.Report{{
		AccountName: "work", Active: true, Enabled: true, State: "ready",
		Usage: quotamodel.Usage{PlanType: "plus", Primary: &quotamodel.Window{UsedPercent: &used}},
	}}, nil
}

func TestShowRendersOneShotQuotaTable(t *testing.T) {
	status := &fakeStatus{}
	var output strings.Builder
	if err := Show(context.Background(), status, &output, []string{"--all", "--once"}); err != nil {
		t.Fatal(err)
	}
	if !status.options.All || !strings.Contains(output.String(), "work\t*\tready\tplus\t80%\t-") {
		t.Fatalf("options/output = %+v / %q", status.options, output.String())
	}
}

func TestShowRejectsSelectorWithAll(t *testing.T) {
	if err := Show(context.Background(), &fakeStatus{}, &strings.Builder{}, []string{"--all", "work"}); err == nil {
		t.Fatal("selector with --all unexpectedly accepted")
	}
}
