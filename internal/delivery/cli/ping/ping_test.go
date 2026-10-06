package ping

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	pingmodel "github.com/christiandoxa/godex/internal/model/ping"
)

type fakeRunner struct {
	options pingmodel.Options
	report  pingmodel.Report
	err     error
}

func (fake *fakeRunner) Run(_ context.Context, options pingmodel.Options) (pingmodel.Report, error) {
	fake.options = options
	return fake.report, fake.err
}

func TestProdex04356PingHelpShowsVisibleProfileAlias(t *testing.T) {
	_, err := parseArguments([]string{"openai", "--help"})
	if err == nil {
		t.Fatal("ping --help unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "-p|--profile") && !strings.Contains(err.Error(), "-p NAME|--profile") {
		t.Fatalf("ping help missing visible --profile alias: %q", err.Error())
	}
}

func TestPingParsesOpenAIOptionsAndRendersHumanOutput(t *testing.T) {
	first := int64(12)
	completion := int64(34)
	runner := &fakeRunner{report: pingmodel.Report{
		Provider: "openai", Status: "ok", Detail: "1/1 profiles healthy",
		Profiles: []pingmodel.Result{{
			Profile: "work", Status: pingmodel.Pass, RequestedModel: "gpt-test", EffectiveModel: "gpt-effective",
			CredentialValidation: "valid", FirstResponseLatencyMS: &first, CompletionLatencyMS: &completion, LatencyMS: &completion,
			Detail: "valid model response received",
		}},
		Summary: pingmodel.Summary{ProfilesDiscovered: 1, ProfilesTested: 1, Healthy: 1, DurationMS: 40, PoolUsable: true},
	}}
	var output strings.Builder
	arguments := []string{"openai", "--profile", "work", "--model", "gpt-test", "--base-url", "https://example.test/backend-api", "--no-proxy"}
	if err := Run(context.Background(), runner, &output, arguments); err != nil {
		t.Fatal(err)
	}
	want := pingmodel.Options{Profile: "work", Model: "gpt-test", BaseURL: "https://example.test/backend-api", NoProxy: true}
	if runner.options != want {
		t.Fatalf("options = %+v, want %+v", runner.options, want)
	}
	for _, text := range []string{"OpenAI application ping", "work", "OK", "first=12ms", "completion=34ms", "Pool usable: yes"} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("output missing %q: %q", text, output.String())
		}
	}
}

func TestPingJSONOutputAndFailureExit(t *testing.T) {
	runner := &fakeRunner{report: pingmodel.Report{
		Provider: "openai", Status: "failed", Detail: "0/1 profiles healthy",
		Profiles: []pingmodel.Result{{Profile: "work", Status: pingmodel.AuthFailed, CredentialValidation: "failed", Detail: "OpenAI authentication failed"}},
		Summary:  pingmodel.Summary{ProfilesDiscovered: 1, ProfilesTested: 1, AuthFailures: 1},
	}}
	var output strings.Builder
	err := Run(context.Background(), runner, &output, []string{"openai", "--json"})
	if err == nil || !strings.Contains(err.Error(), "one or more profiles") {
		t.Fatalf("ping error = %v", err)
	}
	var value map[string]any
	if json.Unmarshal([]byte(output.String()), &value) != nil || value["provider"] != "openai" || value["status"] != "failed" {
		t.Fatalf("json output = %q", output.String())
	}
}

func TestPingRejectsInvalidArgumentsAndRunnerErrors(t *testing.T) {
	for _, arguments := range [][]string{nil, {"gemini"}, {"openai", "--profile"}, {"openai", "--unknown"}} {
		if err := Run(context.Background(), &fakeRunner{}, &strings.Builder{}, arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly accepted", arguments)
		}
	}
	want := errors.New("synthetic runner failure")
	if err := Run(context.Background(), &fakeRunner{err: want}, &strings.Builder{}, []string{"openai"}); !errors.Is(err, want) {
		t.Fatalf("runner error = %v", err)
	}
}

type fakeObservedRunner struct {
	report   pingmodel.Report
	observed []pingmodel.Result
}

func (fake *fakeObservedRunner) Run(context.Context, pingmodel.Options) (pingmodel.Report, error) {
	return fake.report, nil
}

func (fake *fakeObservedRunner) RunObserved(_ context.Context, _ pingmodel.Options, observe func(pingmodel.Result) error) (pingmodel.Report, error) {
	for _, result := range fake.observed {
		if err := observe(result); err != nil {
			return pingmodel.Report{}, err
		}
	}
	return fake.report, nil
}

func TestPingHumanOutputUsesCompletionOrder(t *testing.T) {
	completion := int64(10)
	runner := &fakeObservedRunner{
		observed: []pingmodel.Result{
			{Profile: "slow-name-b", Status: pingmodel.Pass, CompletionLatencyMS: &completion, LatencyMS: &completion},
			{Profile: "fast-name-a", Status: pingmodel.Pass, CompletionLatencyMS: &completion, LatencyMS: &completion},
		},
		report: pingmodel.Report{
			Provider: "openai", Status: "ok",
			Profiles: []pingmodel.Result{
				{Profile: "fast-name-a", Status: pingmodel.Pass, CompletionLatencyMS: &completion, LatencyMS: &completion},
				{Profile: "slow-name-b", Status: pingmodel.Pass, CompletionLatencyMS: &completion, LatencyMS: &completion},
			},
			Summary: pingmodel.Summary{ProfilesDiscovered: 2, ProfilesTested: 2, Healthy: 2, PoolUsable: true},
		},
	}
	var output strings.Builder
	if err := Run(context.Background(), runner, &output, []string{"openai"}); err != nil {
		t.Fatal(err)
	}
	if strings.Index(output.String(), "slow-name-b") > strings.Index(output.String(), "fast-name-a") {
		t.Fatalf("human output lost completion order: %q", output.String())
	}
}

func TestPingJSONKeepsNullableReferenceFields(t *testing.T) {
	completion := int64(8)
	runner := &fakeRunner{report: pingmodel.Report{
		Provider: "openai", Status: "ok", Detail: "1/1 profiles healthy",
		Profiles: []pingmodel.Result{{
			Profile: "work", Status: pingmodel.Pass, CredentialValidation: "valid",
			CompletionLatencyMS: &completion, LatencyMS: &completion, Detail: "valid model response received",
		}},
		Summary: pingmodel.Summary{ProfilesDiscovered: 1, ProfilesTested: 1, Healthy: 1, PoolUsable: true},
	}}
	var output strings.Builder
	if err := Run(context.Background(), runner, &output, []string{"openai", "--json"}); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"model":null`, `"requested_model":null`, `"effective_model":null`, `"first_response_latency_ms":null`} {
		if !strings.Contains(output.String(), field) {
			t.Fatalf("JSON output missing %s: %s", field, output.String())
		}
	}
}
