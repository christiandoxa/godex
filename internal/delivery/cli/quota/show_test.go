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
	reports []quotamodel.Report
	err     error
}

func (fake *fakeStatus) Raw(_ context.Context, selector string) ([]byte, error) {
	fake.options.Selector = selector
	if fake.err != nil {
		return nil, fake.err
	}
	return []byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":20}}}`), nil
}

func (fake *fakeStatus) Run(_ context.Context, options quotausecase.Options) ([]quotamodel.Report, error) {
	fake.options = options
	if fake.reports != nil || fake.err != nil {
		return fake.reports, fake.err
	}
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
	const want = "ACCOUNT\tCURRENT\tSTATE\tPLAN\t5H\tWEEKLY\nwork\t*\tready\tplus\t80%\t-\n"
	if !status.options.All || output.String() != want {
		t.Fatalf("options/output = %+v / %q", status.options, output.String())
	}
}

func TestShowRejectsSelectorWithAll(t *testing.T) {
	if err := Show(context.Background(), &fakeStatus{}, &strings.Builder{}, []string{"--all", "work"}); err == nil {
		t.Fatal("selector with --all unexpectedly accepted")
	}
}

func TestShowRawQuota(t *testing.T) {
	status := &fakeStatus{}
	var output strings.Builder
	if err := Show(context.Background(), status, &output, []string{"--raw", "work"}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"plan_type\": \"plus\",\n  \"rate_limit\": {\n    \"primary_window\": {\n      \"used_percent\": 20\n    }\n  }\n}\n"
	if output.String() != want || status.options.Selector != "work" {
		t.Fatalf("raw output/options = %q / %+v", output.String(), status.options)
	}
}

func TestShowRawQuotaRejectsAggregateAndDetail(t *testing.T) {
	for _, arguments := range [][]string{{"--raw", "--all"}, {"--raw", "--detail"}} {
		if err := Show(context.Background(), &fakeStatus{}, &strings.Builder{}, arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly accepted", arguments)
		}
	}
}
