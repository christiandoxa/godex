package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type cliActivityLog struct {
	events []runtimemodel.Event
}

func (log *cliActivityLog) Append(_ context.Context, event runtimemodel.Event) error {
	log.events = append(log.events, event)
	return nil
}

func (log *cliActivityLog) Tail(_ context.Context, limit int) ([]runtimemodel.Event, error) {
	start := 0
	if len(log.events) > limit {
		start = len(log.events) - limit
	}
	return append([]runtimemodel.Event(nil), log.events[start:]...), nil
}

type cliActivityAccounts struct {
	accounts []accountentity.Account
	current  accountentity.Account
}

func (accounts cliActivityAccounts) List(context.Context) ([]accountentity.Account, error) {
	return append([]accountentity.Account(nil), accounts.accounts...), nil
}

func (accounts cliActivityAccounts) Current(context.Context) (accountentity.Account, error) {
	if accounts.current.ID == "" {
		return accountentity.Account{}, errors.New("no current account")
	}
	return accounts.current, nil
}

type cliActivityVersion struct{}

func (cliActivityVersion) Version(context.Context) (string, error) {
	return "codex-cli 0.159.2", nil
}

func newCLIActivity() *runtimeusecase.Activity {
	current := accountentity.Account{ID: "one", Name: "work", Enabled: true}
	log := &cliActivityLog{events: []runtimemodel.Event{
		{TimestampUnixMilli: 1000, Kind: "request_started", RequestID: "request-1", Method: "POST", Path: "/responses"},
		{TimestampUnixMilli: 1200, Kind: "request_completed", RequestID: "request-1", Method: "POST", Path: "/responses", AccountID: "one", StatusCode: 200, DurationMillis: 200},
	}}
	return runtimeusecase.NewActivity("/managed", log, cliActivityAccounts{
		accounts: []accountentity.Account{current, {ID: "two", Name: "off"}}, current: current,
	}, cliActivityVersion{})
}

func TestProdex04356InfoProcessSummaryMatchesTaggedShape(t *testing.T) {
	if got := formatInfoProcessSummary(nil); got != "No" {
		t.Fatalf("empty process summary = %q, want No", got)
	}
	processes := []statusProcessInfo{
		{pid: 10, command: "run", runtime: true},
		{pid: 11, command: "status"},
		{pid: 12, command: "super", runtime: true},
		{pid: 13, command: "doctor"},
		{pid: 14, command: "gateway", runtime: true},
		{pid: 15, command: "quota"},
		{pid: 16, command: "run", runtime: true},
	}
	want := "Yes (7 total, 4 runtime; processes: 10/run, 11/status, 12/super, 13/doctor, 14/gateway, 15/quota (+1 more))"
	if got := formatInfoProcessSummary(processes); got != want {
		t.Fatalf("process summary = %q, want %q", got, want)
	}
	total, runtime := statusProcessCounts(processes)
	if total != 7 || runtime != 4 {
		t.Fatalf("process counts = %d/%d, want 7/4", total, runtime)
	}
}

func TestInfoTextAndJSON(t *testing.T) {
	activity := newCLIActivity()
	var text bytes.Buffer
	if err := Info(context.Background(), activity, &text, nil); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Profiles: 2", "Active profile: work", "Codex version: codex-cli 0.159.2"} {
		if !strings.Contains(text.String(), expected) {
			t.Fatalf("text info missing %q: %s", expected, text.String())
		}
	}
	var raw bytes.Buffer
	if err := Info(context.Background(), activity, &raw, []string{"--json", "--tokens"}); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value["active_profile"] != "work" || value["token_usage"] != "unavailable" {
		t.Fatalf("json info = %#v", value)
	}
}

func TestStatusNonTerminalUsesOneSnapshot(t *testing.T) {
	var output bytes.Buffer
	if err := Status(context.Background(), newCLIActivity(), &output, []string{"--interval", "2"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Active profile: work") || !strings.Contains(output.String(), "Inflight: 0") {
		t.Fatalf("status = %q", output.String())
	}
}

func TestLogLastAndJSON(t *testing.T) {
	activity := newCLIActivity()
	var output bytes.Buffer
	if err := Log(context.Background(), activity, &output, []string{"last"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "request_completed") || !strings.Contains(output.String(), "status=200") {
		t.Fatalf("last log = %q", output.String())
	}
	output.Reset()
	if err := Log(context.Background(), activity, &output, []string{"last", "--json"}); err != nil {
		t.Fatal(err)
	}
	var event runtimemodel.Event
	if err := json.Unmarshal(output.Bytes(), &event); err != nil || event.Kind != "request_completed" {
		t.Fatalf("json log = %#v, %v", event, err)
	}
}

func TestObservabilityArgumentsRejectInvalidInput(t *testing.T) {
	activity := newCLIActivity()
	for _, test := range []func() error{
		func() error { return Info(context.Background(), activity, &bytes.Buffer{}, []string{"--wat"}) },
		func() error {
			return Status(context.Background(), activity, &bytes.Buffer{}, []string{"--interval", "0"})
		},
		func() error {
			return Log(context.Background(), activity, &bytes.Buffer{}, []string{"last", "upstream"})
		},
	} {
		if err := test(); err == nil {
			t.Fatal("invalid observability arguments unexpectedly accepted")
		}
	}
}

func TestLogTUIStateUsesBubbleTeaControls(t *testing.T) {
	model := newLogTUIModel(context.Background(), newCLIActivity(), logOptions{mode: "stream"})
	updated, command := model.Update(logSnapshotMsg{events: []runtimemodel.Event{
		{TimestampUnixMilli: 1000, Kind: "request_started", RequestID: "a", Path: "/responses", Message: "alpha"},
		{TimestampUnixMilli: 2000, Kind: "request_completed", RequestID: "b", Path: "/responses", Message: "beta"},
	}})
	model = updated.(logTUIModel)
	if command != nil || !strings.Contains(model.View(), "Godex Log") || !strings.Contains(model.View(), "beta") {
		t.Fatalf("initial log TUI = %q, command=%v", model.View(), command)
	}

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	model = updated.(logTUIModel)
	for _, ch := range "alpha" {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		model = updated.(logTUIModel)
	}
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = updated.(logTUIModel)
	if !strings.Contains(model.View(), "alpha") || strings.Contains(model.View(), "beta") {
		t.Fatalf("search view = %q", model.View())
	}
	_, quit := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quit == nil {
		t.Fatal("q did not quit log TUI")
	}
}
