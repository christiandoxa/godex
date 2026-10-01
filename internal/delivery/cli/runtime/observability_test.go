package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

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
