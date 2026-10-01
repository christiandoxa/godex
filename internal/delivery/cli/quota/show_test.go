package quota

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
	quotausecase "github.com/christiandoxa/godex/internal/usecase/quota"
)

type fakeStatus struct {
	options quotausecase.Options
	reports []quotamodel.Report
	err     error
	calls   int
	cancel  context.CancelFunc
}

func (fake *fakeStatus) Raw(_ context.Context, selector, baseURL string) ([]byte, error) {
	fake.options.Selector = selector
	fake.options.BaseURL = baseURL
	if fake.err != nil {
		return nil, fake.err
	}
	return []byte(`{"plan_type":"plus","rate_limit":{"primary_window":{"used_percent":20}}}`), nil
}

func (fake *fakeStatus) Run(_ context.Context, options quotausecase.Options) ([]quotamodel.Report, error) {
	fake.options = options
	fake.calls++
	if fake.cancel != nil && fake.calls >= 2 {
		fake.cancel()
	}
	if fake.reports != nil || fake.err != nil {
		return fake.reports, fake.err
	}
	used := int64(20)
	return []quotamodel.Report{{
		AccountName: "work", Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true, State: "ready",
		Usage: quotamodel.Usage{PlanType: "plus", Primary: &quotamodel.Window{UsedPercent: &used}},
	}}, nil
}

func TestShowRendersOneShotQuotaTable(t *testing.T) {
	status := &fakeStatus{}
	var output strings.Builder
	if err := Show(context.Background(), status, &output, []string{"--all", "--once"}); err != nil {
		t.Fatal(err)
	}
	const want = "PROFILE\tCURRENT\tPROVIDER\tAUTH\tSTATE\tPLAN\t5H\tWEEKLY\nwork\t*\topenai\tchatgpt\tready\tplus\t80%\t-\n"
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
	if err := Show(context.Background(), status, &output, []string{"--raw", "--base-url", "https://quota.test/backend-api", "work"}); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"plan_type\": \"plus\",\n  \"rate_limit\": {\n    \"primary_window\": {\n      \"used_percent\": 20\n    }\n  }\n}\n"
	if output.String() != want || status.options.Selector != "work" || status.options.BaseURL != "https://quota.test/backend-api" {
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

func TestShowSupportsProfileAndBaseURL(t *testing.T) {
	status := &fakeStatus{}
	var output strings.Builder
	arguments := []string{"--profile", "work", "--base-url", "https://quota.test/backend-api", "--once"}
	if err := Show(context.Background(), status, &output, arguments); err != nil {
		t.Fatal(err)
	}
	want := quotausecase.Options{Selector: "work", BaseURL: "https://quota.test/backend-api"}
	if status.options != want {
		t.Fatalf("options = %+v, want %+v", status.options, want)
	}
}

func TestShowDefaultsToWatchUntilContextStops(t *testing.T) {
	original := quotaWatchInterval
	quotaWatchInterval = time.Millisecond
	defer func() { quotaWatchInterval = original }()
	ctx, cancel := context.WithCancel(context.Background())
	status := &fakeStatus{cancel: cancel}
	var output strings.Builder
	err := Show(ctx, status, &output, []string{"--all"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch error = %v", err)
	}
	if status.calls < 2 || strings.Count(output.String(), "PROFILE\tCURRENT") < 2 {
		t.Fatalf("watch calls/output = %d / %q", status.calls, output.String())
	}
}

func TestShowRejectsWatchConflicts(t *testing.T) {
	for _, arguments := range [][]string{{"--watch", "--once"}, {"--watch", "--raw"}, {"--all", "--profile", "work"}} {
		if err := Show(context.Background(), &fakeStatus{}, &strings.Builder{}, arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly accepted", arguments)
		}
	}
}

func TestShowSupportsAuthAndProviderFilters(t *testing.T) {
	status := &fakeStatus{}
	var output strings.Builder
	arguments := []string{"--all", "--auth", "quota-compatible", "--provider", "openai", "--once"}
	if err := Show(context.Background(), status, &output, arguments); err != nil {
		t.Fatal(err)
	}
	want := quotausecase.Options{All: true, AuthFilter: "quota-compatible", ProviderFilter: "openai"}
	if status.options != want {
		t.Fatalf("options = %+v, want %+v", status.options, want)
	}
}

func TestShowRejectsFilterConflictsAndUnknownProvider(t *testing.T) {
	for _, arguments := range [][]string{
		{"--profile", "work", "--auth", "chatgpt", "--once"},
		{"--profile", "work", "--provider", "openai", "--once"},
		{"--raw", "--auth", "chatgpt"},
		{"--provider", "unknown", "--all", "--once"},
	} {
		if err := Show(context.Background(), &fakeStatus{}, &strings.Builder{}, arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly accepted", arguments)
		}
	}
}
