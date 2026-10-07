package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

func TestProdex04357LogHumanEventNamesMatchTaggedRenderer(t *testing.T) {
	for input, want := range map[string]string{
		"stream_read_error":         "stream read failed",
		"custom_event_name":         "custom event name",
		"super_expose_exec_started": "MCP",
		"sub_agent_started":         "sub-agent",
	} {
		if got := humanLogEventName(input); got != want {
			t.Fatalf("human event %q = %q, want %q", input, got, want)
		}
	}
}

func TestProdex04357LogPlainOutputUsesBlockRenderingAndSafeMetadata(t *testing.T) {
	event := runtimemodel.Event{
		TimestampUnixMilli: time.Date(2026, 6, 20, 1, 0, 0, 123_000_000, time.UTC).UnixMilli(),
		RequestID:          "request-7",
		Kind:               "profile_health",
		Method:             "POST",
		Path:               "/backend-api/codex/responses",
		AccountID:          "profile-a",
		StatusCode:         429,
		DurationMillis:     17,
		Message:            "route degraded",
		Fields: map[string]string{
			"profile": "profile-a",
			"route":   "responses",
			"score":   "3",
			"reason":  "stream_read_error",
			"secret":  "sk-synthetic-never-render-this-value",
		},
	}
	rendered := formatLogEvent(event)
	for _, want := range []string{
		"[" + time.UnixMilli(event.TimestampUnixMilli).In(time.Local).Format("2006-01-02 15:04:05.000 -07:00") + "] HEALTH",
		"request=request-7",
		"profile=profile-a",
		"route=responses",
		"status=429",
		"method=POST",
		"path=/backend-api/codex/responses",
		"duration_ms=17",
		"| health penalty",
		"| route degraded",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("plain log block missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "	profile_health	") {
		t.Fatalf("plain log retained raw TSV event format:\n%s", rendered)
	}
	if strings.Contains(rendered, "sk-synthetic-never-render-this-value") {
		t.Fatalf("plain log leaked secret-like metadata: %s", rendered)
	}
}

func TestProdex04357LogJSONKeepsRawEventContract(t *testing.T) {
	event := runtimemodel.Event{
		TimestampUnixMilli: 1234,
		Kind:               "stream_read_error",
		Message:            "synthetic",
	}
	log := &cliActivityLog{events: []runtimemodel.Event{event}}
	current := accountentity.Account{ID: "one", Name: "work", Enabled: true}
	activity := runtimeusecase.NewActivity("/managed", log, cliActivityAccounts{
		accounts: []accountentity.Account{current}, current: current,
	}, cliActivityVersion{})
	var out bytes.Buffer
	if err := Log(context.Background(), activity, &out, []string{"last", "--json"}); err != nil {
		t.Fatal(err)
	}
	var got runtimemodel.Event
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Kind != "stream_read_error" || got.TimestampUnixMilli != 1234 {
		t.Fatalf("JSON event transformed unexpectedly: %#v", got)
	}
}
