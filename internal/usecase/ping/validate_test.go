package ping

import (
	"context"
	"errors"
	"strings"
	"testing"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
)

func TestPingJSONLProtocolBoundaries(t *testing.T) {
	pass := []byte("{\"type\":\"thread.started\",\"thread_id\":\"t\"}\n" +
		"{\"type\":\"turn.started\"}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello\"}}\n" +
		"{\"type\":\"turn.completed\"}\n")
	status, _, _ := validateJSONL(pass)
	if status != pingmodel.Pass {
		t.Fatalf("pass status = %s", status)
	}

	tool := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"command_execution\",\"command\":\"touch file\"}}\n" +
		"{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello\"}}\n{\"type\":\"turn.completed\"}\n")
	status, _, _ = validateJSONL(tool)
	if status != pingmodel.ProtocolFailed {
		t.Fatalf("tool status = %s", status)
	}

	noAgent := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"turn.completed\"}\n")
	status, _, _ = validateJSONL(noAgent)
	if status != pingmodel.UnexpectedResponse {
		t.Fatalf("no-agent status = %s", status)
	}

	noCompletion := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello\"}}\n")
	status, _, _ = validateJSONL(noCompletion)
	if status != pingmodel.ProtocolFailed {
		t.Fatalf("no-completion status = %s", status)
	}
}

func TestPingFailureTaxonomyMatchesProdexPrecedence(t *testing.T) {
	cases := map[string]pingmodel.Status{
		"HTTP 503 quota exceeded":      pingmodel.UpstreamOverloaded,
		"HTTP 429 insufficient_quota":  pingmodel.QuotaExhausted,
		"HTTP 429 quota exceeded":      pingmodel.RateLimited,
		"usage_limit_reached":          pingmodel.QuotaExhausted,
		"failed to start codex child":  pingmodel.SpawnFailed,
		"x509 certificate expired":     pingmodel.TLSFailed,
		"lookup api.openai.com failed": pingmodel.DNSFailed,
		"HTTP 401 unauthorized":        pingmodel.AuthFailed,
	}
	for input, want := range cases {
		if got := classifyFailure(input); got != want {
			t.Fatalf("classify %q = %s, want %s", input, got, want)
		}
	}

	failure := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"usage_limit_reached\"}}\n")
	status, _, _ := validateJSONL(failure)
	if status != pingmodel.QuotaExhausted {
		t.Fatalf("turn failure status = %s", status)
	}
}

type fakePingProfiles struct{ targets []profilemodel.QuotaTarget }

func (fake fakePingProfiles) QuotaTargets(context.Context) ([]profilemodel.QuotaTarget, error) {
	return fake.targets, nil
}

type fakePingProcess struct {
	results map[string]pingmodel.ProcessResult
	errs    map[string]error
	options map[string]pingmodel.Options
}

func (fake fakePingProcess) PingOpenAI(_ context.Context, target pingmodel.Target, options pingmodel.Options) (pingmodel.ProcessResult, error) {
	if fake.options != nil {
		fake.options[target.Name] = options
	}
	if err := fake.errs[target.Name]; err != nil {
		return pingmodel.ProcessResult{}, err
	}
	return fake.results[target.Name], nil
}

func TestOpenAIPingPropagatesReasoningEffort(t *testing.T) {
	pass := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello\"}}\n{\"type\":\"turn.completed\"}\n")
	options := map[string]pingmodel.Options{}
	process := fakePingProcess{results: map[string]pingmodel.ProcessResult{"work": {Stdout: pass, ExitCode: 0, LatencyMS: 10}}, options: options}
	probe := NewOpenAI(fakePingProfiles{targets: []profilemodel.QuotaTarget{{Name: "work", CodexHome: "/work", Provider: "openai"}}}, process)
	report, err := probe.Run(context.Background(), pingmodel.Options{Model: "gpt-6-astra", Effort: "MAX"})
	if err != nil {
		t.Fatal(err)
	}
	if options["work"].Effort != "max" {
		t.Fatalf("child options = %+v, want normalized effort max", options["work"])
	}
	if report.Profiles[0].Effort != "max" || report.Profiles[0].RequestedEffort != "max" {
		t.Fatalf("report profile = %+v, want effort fields", report.Profiles[0])
	}
}

func TestOpenAIPingRejectsUnsupportedReasoningEffort(t *testing.T) {
	process := fakePingProcess{options: map[string]pingmodel.Options{}}
	probe := NewOpenAI(fakePingProfiles{targets: []profilemodel.QuotaTarget{{Name: "work", CodexHome: "/work", Provider: "openai"}}}, process)
	if _, err := probe.Run(context.Background(), pingmodel.Options{Model: "gpt-5.6-luna", Effort: "ultra"}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported effort error = %v", err)
	}
}

func TestOpenAIPingSelectsProfilesAndSummarizes(t *testing.T) {
	pass := []byte("{\"type\":\"thread.started\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"type\":\"agent_message\",\"text\":\"Hello\"}}\n{\"type\":\"turn.completed\"}\n")
	process := fakePingProcess{results: map[string]pingmodel.ProcessResult{
		"a": {Stdout: pass, ExitCode: 0, LatencyMS: 10},
		"b": {Stderr: []byte("HTTP 429 insufficient_quota"), ExitCode: 1, LatencyMS: 20},
	}, errs: map[string]error{"c": errors.New("spawn")}}
	probe := NewOpenAI(fakePingProfiles{targets: []profilemodel.QuotaTarget{
		{Name: "a", CodexHome: "/a", Provider: "openai"},
		{Name: "b", CodexHome: "/b", Provider: "openai"},
		{Name: "c", CodexHome: "/c", Provider: "anthropic"},
	}}, process)
	report, err := probe.Run(context.Background(), pingmodel.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Profiles) != 2 || report.Profiles[0].Profile != "a" || report.Profiles[0].Status != pingmodel.Pass || report.Profiles[1].Status != pingmodel.QuotaExhausted {
		t.Fatalf("report profiles = %+v", report.Profiles)
	}
	if report.Summary.Healthy != 1 || report.Summary.Exhausted != 1 || report.Summary.PoolUsable != true || !Failed(report) {
		t.Fatalf("summary = %+v", report.Summary)
	}
}

func TestPingFailureDetailsAreBoundedAndRedacted(t *testing.T) {
	secret := "secret-sentinel-value"
	input := "Authorization: Bearer " + secret + ` {"access_token":"` + secret + `"} https://user:` + secret + `@example.test ` + strings.Repeat("x", pingErrorDetailMaxBytes)
	detail := boundedPingDetail(input)
	if strings.Contains(detail, secret) {
		t.Fatalf("secret leaked from bounded detail: %q", detail)
	}
	if len(detail) > pingErrorDetailMaxBytes {
		t.Fatalf("detail length = %d", len(detail))
	}
}

func TestPingCleanupFailureOverridesModelResult(t *testing.T) {
	status, detail, _ := classifyProcessResult(pingmodel.ProcessResult{
		CleanupFailed: true,
		ExitCode:      0,
		Stderr:        []byte("OPENAI_API_KEY=secret-sentinel-value"),
	})
	if status != pingmodel.ProcessFailed || !strings.Contains(detail, "cleanup failed") {
		t.Fatalf("cleanup status/detail = %s / %q", status, detail)
	}
	if strings.Contains(detail, "secret-sentinel-value") {
		t.Fatalf("cleanup detail leaked secret: %q", detail)
	}
}
