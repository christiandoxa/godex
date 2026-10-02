package quota

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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

func TestQuotaProfileTUIViewAndQuitKeys(t *testing.T) {
	used := int64(20)
	model := newQuotaTUIModel(context.Background(), &fakeStatus{}, showOptions{detail: true})
	updated, command := model.Update(quotaSnapshotMsg{reports: []quotamodel.Report{{
		ProfileName: "work", Provider: "openai", Auth: "chatgpt", Active: true, Enabled: true, State: "ready",
		Usage: quotamodel.Usage{PlanType: "plus", Primary: &quotamodel.Window{UsedPercent: &used}},
	}}})
	model = updated.(quotaTUIModel)
	if command != nil {
		t.Fatal("snapshot unexpectedly returned command")
	}
	for _, expected := range []string{"Godex Quota", "PROFILE", "work", "q/esc quit"} {
		if !strings.Contains(model.View(), expected) {
			t.Fatalf("view missing %q: %q", expected, model.View())
		}
	}
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if quit == nil {
		t.Fatal("esc did not quit")
	}
	if strings.Contains(model.View(), "refresh") {
		t.Fatalf("profile TUI unexpectedly exposes refresh control: %q", model.View())
	}
	unchanged, refresh := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if refresh != nil || unchanged.(quotaTUIModel).loading {
		t.Fatal("profile TUI accepted all-profile refresh key")
	}
}

func TestQuotaAllTUISortsAndScrollsLikeProdex(t *testing.T) {
	status := quotaAllTUITestStatus()
	model := newQuotaTUIModel(context.Background(), status, showOptions{Options: quotausecase.Options{All: true}})
	updated, _ := model.Update(quotaSnapshotMsg{reports: status.reports})
	model = updated.(quotaTUIModel)
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 9})
	model = updated.(quotaTUIModel)

	if model.sortMode != quotaSortCurrent || model.providerFilter != quotaProviderAll {
		t.Fatalf("initial policy = sort:%s provider:%s", model.sortMode.label(), model.providerFilter.label())
	}
	if reports := model.sortedReports(); len(reports) != 4 || reports[0].ProfileName != "alpha" {
		t.Fatalf("current sort = %#v", reports)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	model = updated.(quotaTUIModel)
	if model.scrollOffset != 1 {
		t.Fatalf("scroll offset after j = %d", model.scrollOffset)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(quotaTUIModel)
	if model.scrollOffset != 0 {
		t.Fatalf("scroll offset after up = %d", model.scrollOffset)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	model = updated.(quotaTUIModel)
	if model.sortMode != quotaSortRemaining || model.scrollOffset != 0 {
		t.Fatalf("sort after s = %s offset=%d", model.sortMode.label(), model.scrollOffset)
	}
	if reports := model.sortedReports(); len(reports) != 4 || reports[0].ProfileName != "beta" {
		t.Fatalf("remaining sort = %#v", reports)
	}
}

func TestQuotaAllTUIFiltersAndRefreshesLikeProdex(t *testing.T) {
	status := quotaAllTUITestStatus()
	model := newQuotaTUIModel(context.Background(), status, showOptions{Options: quotausecase.Options{All: true}})
	updated, _ := model.Update(quotaSnapshotMsg{reports: status.reports})
	model = updated.(quotaTUIModel)

	updated, filterCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(quotaTUIModel)
	if model.providerFilter != quotaProviderOpenAI || filterCmd == nil || !model.loading {
		t.Fatalf("filter transition = provider:%s loading:%t cmd:%v", model.providerFilter.label(), model.loading, filterCmd)
	}
	message := filterCmd()
	snapshot, ok := message.(quotaSnapshotMsg)
	if !ok {
		t.Fatalf("filter command message = %#v", message)
	}
	updated, _ = model.Update(snapshot)
	model = updated.(quotaTUIModel)
	if status.options.ProviderFilter != "openai" || len(model.sortedReports()) != 1 || model.sortedReports()[0].ProfileName != "zeta" {
		t.Fatalf("filtered options/reports = %+v / %#v", status.options, model.sortedReports())
	}

	updated, refreshCmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	model = updated.(quotaTUIModel)
	if refreshCmd == nil || !model.loading {
		t.Fatal("u did not start all-profile refresh")
	}
	for _, expected := range []string{"Sort: current", "Provider: openai", "j/k scroll", "s sort", "f filter", "u refresh"} {
		if !strings.Contains(model.View(), expected) {
			t.Fatalf("all-profile view missing %q: %q", expected, model.View())
		}
	}
}

func quotaAllTUITestStatus() *fakeStatus {
	resetSoon, resetLater := int64(100), int64(200)
	usedReady, usedBlocked := int64(20), int64(100)
	return &fakeStatus{reports: []quotamodel.Report{
		{ProfileName: "zeta", Provider: "openai", Auth: "chatgpt", Active: false, Enabled: true, State: "ready", Email: "z@example.test", Usage: quotamodel.Usage{PlanType: "plus", Primary: &quotamodel.Window{UsedPercent: &usedReady, ResetAt: &resetLater}}},
		{ProfileName: "alpha", Provider: "anthropic", Auth: "anthropic", Active: true, Enabled: true, State: "unsupported", Email: "a@example.test"},
		{ProfileName: "beta", Provider: "gemini", Auth: "gemini", Active: false, Enabled: true, State: "ready", Email: "b@example.test", Usage: quotamodel.Usage{PlanType: "pro", Primary: &quotamodel.Window{UsedPercent: &usedReady, ResetAt: &resetSoon}}},
		{ProfileName: "gamma", Provider: "copilot", Auth: "copilot", Active: false, Enabled: true, State: "exhausted", Usage: quotamodel.Usage{PlanType: "business", Primary: &quotamodel.Window{UsedPercent: &usedBlocked}}},
	}}
}

func TestQuotaAllTUILocksExplicitProviderFilter(t *testing.T) {
	model := newQuotaTUIModel(context.Background(), &fakeStatus{}, showOptions{
		Options: quotausecase.Options{All: true, ProviderFilter: "anthropic"},
	})
	if !model.providerFilterLocked || model.providerFilter != quotaProviderAnthropic {
		t.Fatalf("locked filter = locked:%t provider:%s", model.providerFilterLocked, model.providerFilter.label())
	}
	updated, command := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	model = updated.(quotaTUIModel)
	if command != nil || model.providerFilter != quotaProviderAnthropic {
		t.Fatalf("locked filter changed = provider:%s cmd:%v", model.providerFilter.label(), command)
	}
	if !strings.Contains(model.footer(), "filter locked") {
		t.Fatalf("locked footer = %q", model.footer())
	}
}

func TestQuotaProviderAliasesCanonicalizeLikeProdex(t *testing.T) {
	fixtures := map[string]string{
		"chatgpt":           "openai",
		"codex":             "openai",
		"google":            "gemini",
		"google_gemini":     "gemini",
		"claude":            "anthropic",
		"github":            "copilot",
		"github-copilot":    "copilot",
		"kiro-cli":          "kiro",
		"openai_compatible": "local",
		"anti-gravity":      "agy",
	}
	for input, want := range fixtures {
		options, err := parseArguments([]string{"--all", "--provider", input, "--once"})
		if err != nil {
			t.Fatalf("provider %q: %v", input, err)
		}
		if options.ProviderFilter != want {
			t.Fatalf("provider %q canonical = %q, want %q", input, options.ProviderFilter, want)
		}
	}
}

func TestQuotaSortPolicyMatchesProdexOrderAndTextRules(t *testing.T) {
	modes := []quotaReportSort{
		quotaSortCurrent, quotaSortRemaining, quotaSortProfile,
		quotaSortAuth, quotaSortAccount, quotaSortPlan,
	}
	current := quotaSortCurrent
	for _, want := range modes {
		if current != want {
			t.Fatalf("sort cycle = %s, want %s", current.label(), want.label())
		}
		current = current.next()
	}
	if current != quotaSortCurrent {
		t.Fatalf("sort cycle did not wrap: %s", current.label())
	}
	if compareQuotaText("  Beta ", "alpha") <= 0 {
		t.Fatal("case-insensitive trimmed quota text ordering drift")
	}
}
