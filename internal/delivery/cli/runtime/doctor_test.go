package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	accountentity "github.com/christiandoxa/godex/internal/entity/account"
	"github.com/christiandoxa/godex/internal/gateway/codex"
	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
	runtimeusecase "github.com/christiandoxa/godex/internal/usecase/runtime"
)

type fakeDoctorRunner struct {
	options      runtimemodel.DoctorOptions
	report       runtimemodel.DoctorDiagnostics
	err          error
	repairErr    error
	repairCalls  int
	savedPath    string
	savedContent []byte
}

func (fake *fakeDoctorRunner) Diagnose(_ context.Context, options runtimemodel.DoctorOptions) (runtimemodel.DoctorDiagnostics, error) {
	fake.options = options
	return fake.report, fake.err
}

func (fake *fakeDoctorRunner) SaveBundle(path string, content []byte) (string, error) {
	fake.savedPath = path
	fake.savedContent = append([]byte(nil), content...)
	return "/absolute/doctor.json", nil
}

func (fake *fakeDoctorRunner) RepairSessionIndex(context.Context) error {
	fake.repairCalls++
	return fake.repairErr
}

func TestDoctorParsesRuntimeQuotaInstallAndJSON(t *testing.T) {
	runner := &fakeDoctorRunner{report: doctorFixture()}
	var output strings.Builder
	arguments := []string{"--runtime", "--quota", "--install", "--tail-bytes", "4096", "--json"}
	if err := Doctor(context.Background(), runner, &output, arguments); err != nil {
		t.Fatal(err)
	}
	want := runtimemodel.DoctorOptions{Runtime: true, Quota: true, Install: true, TailBytes: 4096}
	if runner.options != want {
		t.Fatalf("options=%+v want=%+v", runner.options, want)
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(output.String()), &value); err != nil || value["codex_version"] != "codex-cli synthetic" {
		t.Fatalf("json=%q err=%v", output.String(), err)
	}
}

func TestDoctorParsesAndReportsImportAuthJournalRepair(t *testing.T) {
	count := 1
	report := doctorFixture()
	report.ImportAuthJournals = &runtimemodel.DoctorImportAuthJournals{OrphanCount: 0, RepairPerformed: true, Repaired: count, Status: "ok"}
	runner := &fakeDoctorRunner{report: report}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--repair-import-auth-journals"}); err != nil {
		t.Fatal(err)
	}
	if !runner.options.RepairImportAuthJournals || !strings.Contains(output.String(), "Import auth journals: Repaired 1 orphan journal(s).") {
		t.Fatalf("repair output = %q, options = %+v", output.String(), runner.options)
	}

	output.Reset()
	runner = &fakeDoctorRunner{report: report}
	if err := Doctor(context.Background(), runner, &output, []string{"--repair-import-auth-journals", "--runtime", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"repaired": 1`) {
		t.Fatalf("repair json = %q", output.String())
	}

	output.Reset()
	runner = &fakeDoctorRunner{report: report}
	if err := Doctor(context.Background(), runner, &output, []string{"--repair-import-auth-journals", "--bundle", "--redacted"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"repaired": 1`) {
		t.Fatalf("repair bundle = %q", output.String())
	}
}

func TestDoctorBundleRequiresRedactionAndSupportsStdoutOrFile(t *testing.T) {
	if err := Doctor(context.Background(), &fakeDoctorRunner{}, &strings.Builder{}, []string{"--bundle"}); err == nil {
		t.Fatal("unredacted bundle unexpectedly accepted")
	}

	runner := &fakeDoctorRunner{report: doctorFixture()}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--bundle", "--redacted", "--quota"}); err != nil {
		t.Fatal(err)
	}
	if runner.savedPath != "" || !strings.Contains(output.String(), `"kind": "godex_doctor"`) || !strings.Contains(output.String(), `"redacted": true`) {
		t.Fatalf("stdout bundle=%q saved=%q", output.String(), runner.savedPath)
	}

	output.Reset()
	runner = &fakeDoctorRunner{report: doctorFixture()}
	if err := Doctor(context.Background(), runner, &output, []string{"--bundle=doctor.json", "--redacted", "--install"}); err != nil {
		t.Fatal(err)
	}
	if runner.savedPath != "doctor.json" || len(runner.savedContent) == 0 || output.String() != "Doctor bundle: /absolute/doctor.json\n" {
		t.Fatalf("saved path/content/output = %q/%d/%q", runner.savedPath, len(runner.savedContent), output.String())
	}
}

func TestDoctorValidatesReferenceFlagRelationships(t *testing.T) {
	for _, arguments := range [][]string{
		{"--json"},
		{"--runtime", "--json", "--bundle", "-", "--redacted"},
		{"--redacted"},
		{"--suggest-policy"},
		{"--tail-bytes", "9000000"},
		{"--unknown"},
	} {
		if err := Doctor(context.Background(), &fakeDoctorRunner{}, &strings.Builder{}, arguments); err == nil {
			t.Fatalf("arguments %v unexpectedly accepted", arguments)
		}
	}
}

func TestDoctorRepairsSessionIndexAndReportsOnStderr(t *testing.T) {
	runner := &fakeDoctorRunner{report: doctorFixture()}
	var output, diagnostic strings.Builder
	if err := DoctorWithErrorOutput(context.Background(), runner, &output, &diagnostic, []string{"--repair-session-index", "--install"}); err != nil {
		t.Fatal(err)
	}
	if runner.repairCalls != 1 || !runner.options.Install {
		t.Fatalf("doctor options = %+v", runner.options)
	}
	if diagnostic.String() != "godex doctor: session index repair completed.\n" {
		t.Fatalf("diagnostic output = %q", diagnostic.String())
	}
	if !strings.Contains(output.String(), "Doctor\n") || !strings.Contains(output.String(), "Install Checks\n") {
		t.Fatalf("doctor report = %q", output.String())
	}
}

func TestDoctorReportsRepairBeforeLaterDiagnosticFailure(t *testing.T) {
	want := errors.New("synthetic diagnostics failure")
	runner := &fakeDoctorRunner{err: want}
	var output, diagnostic strings.Builder
	err := DoctorWithErrorOutput(context.Background(), runner, &output, &diagnostic, []string{"--repair-session-index"})
	if !errors.Is(err, want) {
		t.Fatalf("doctor error = %v", err)
	}
	if diagnostic.String() != "godex doctor: session index repair completed.\n" {
		t.Fatalf("diagnostic output = %q", diagnostic.String())
	}
}

func TestDoctorRepairsSessionIndexAcrossDeliveryUsecaseAndCodexGateway(t *testing.T) {
	if goruntime.GOOS == "windows" {
		t.Skip("helper uses a POSIX shell")
	}
	root := t.TempDir()
	activeHome := filepath.Join(root, "active-codex")
	godexHome := filepath.Join(root, "godex")
	sessionID := "01900000-0000-7000-8000-000000000041"
	session := filepath.Join(activeHome, "sessions", "2026", "10", "03", "rollout-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0o700); err != nil {
		t.Fatal(err)
	}
	oldRoot := filepath.Join(root, "deleted-overlay")
	oldAttachment := filepath.Join(oldRoot, "attachments", "thread-1", "pasted-text-1.txt")
	if err := os.MkdirAll(filepath.Dir(oldAttachment), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldAttachment, []byte("doctor attachment"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := strings.Join([]string{
		"{\"timestamp\":\"2026-10-03T12:00:00Z\",\"type\":\"event\",\"payload\":{\"message\":\"read " + oldAttachment + "\"}}",
		"{\"timestamp\":\"2026-10-03T12:01:00Z\",\"type\":\"session_meta\",\"payload\":{\"id\":\"" + sessionID + "\",\"thread_id\":\"thread-1\"}}",
	}, "\n") + "\n"
	if err := os.WriteFile(session, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	stableAttachment := filepath.Join(activeHome, "attachments", "thread-1", "pasted-text-1.txt")
	record := filepath.Join(root, "child-environment")
	script := filepath.Join(root, "codex")
	content := strings.Join([]string{
		"#!/bin/sh",
		"if [ \"$1\" = --version ]; then printf '%s\\n' 'codex 0.160.0'; exit 0; fi",
		"for argument in \"$@\"; do [ \"$argument\" = exec-server ] && exit 0; done",
		"[ \"$1\" = app-server ] || exit 42",
		"head -n 1 \"$GODEX_DOCTOR_SESSION\" | grep -q '\"type\":\"session_meta\"' || exit 43",
		"[ -f \"$GODEX_DOCTOR_STABLE_ATTACHMENT\" ] || exit 44",
		"printf '%s|%s' \"$CODEX_HOME\" \"$CODEX_SQLITE_HOME\" > \"$GODEX_DOCTOR_REPAIR_RECORD\"",
		"read line",
		"printf '%s\\n' '{\"id\":1,\"result\":{}}'",
		"read line",
		"read line",
		"printf '%s\\n' '{\"id\":2,\"result\":{\"nextCursor\":null}}'",
		"read line",
		"printf '%s\\n' '{\"id\":3,\"result\":{\"nextCursor\":null}}'",
	}, "\n") + "\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GODEX_DOCTOR_REPAIR_RECORD", record)
	t.Setenv("GODEX_DOCTOR_SESSION", session)
	t.Setenv("GODEX_DOCTOR_STABLE_ATTACHMENT", stableAttachment)

	var timing strings.Builder
	t.Setenv("PRODEX_RUNTIME_TIMINGS", "")
	process := codex.NewCodexProcess(script, codex.Terminal{Stderr: &timing})
	doctor := runtimeusecase.NewDoctor(integrationDoctorAccounts{root: godexHome}, process)
	doctor.SetSessionIndexRepairer(process)
	doctor.SetActiveCodexHomeResolver(integrationCodexHome(activeHome))
	doctor.SetSharedCodexHome(activeHome)
	var output, diagnostic strings.Builder
	if err := DoctorWithErrorOutput(t.Context(), doctor, &output, &diagnostic, []string{"--repair-session-index"}); err != nil {
		t.Fatal(err)
	}
	if diagnostic.String() != "godex doctor: session index repair completed.\n" || !strings.Contains(output.String(), "Doctor\n") {
		t.Fatalf("doctor output = %q, diagnostic = %q", output.String(), diagnostic.String())
	}
	childEnvironment, err := os.ReadFile(record)
	if err != nil {
		t.Fatal(err)
	}
	if string(childEnvironment) != activeHome+"|"+activeHome {
		t.Fatalf("Codex app-server environment = %q", childEnvironment)
	}
	if got, err := os.ReadFile(stableAttachment); err != nil || string(got) != "doctor attachment" {
		t.Fatalf("stable attachment = %q, err=%v", got, err)
	}
	repaired, err := os.ReadFile(session)
	if err != nil {
		t.Fatal(err)
	}
	if first := strings.Split(strings.TrimSpace(string(repaired)), "\n")[0]; !strings.Contains(first, "\"type\":\"session_meta\"") {
		t.Fatalf("session metadata was not repaired before app-server launch: %q", first)
	}
	if _, err := os.Stat(session + ".prodex-repair-bak"); err != nil {
		t.Fatalf("session repair backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(godexHome, "shared-codex-session-maintenance-v1.json")); err != nil {
		t.Fatalf("maintenance cache missing from Godex home: %v", err)
	}
	if !strings.Contains(timing.String(), "prodex_runtime_timing stage=startup.thread_index_reconcile_ms duration_ms=") {
		t.Fatalf("runtime timing = %q", timing.String())
	}
}

type integrationDoctorAccounts struct{ root string }

func (integrationDoctorAccounts) Prepare() error { return nil }
func (accounts integrationDoctorAccounts) Root() string {
	if accounts.root != "" {
		return accounts.root
	}
	return "/synthetic/godex"
}
func (integrationDoctorAccounts) List(context.Context) ([]accountentity.Account, error) {
	return nil, nil
}

type integrationCodexHome string

func (home integrationCodexHome) CurrentCodexHome(context.Context) (string, error) {
	return string(home), nil
}

func TestProdex04355DoctorRendersPolicySuggestions(t *testing.T) {
	count := 1
	suggestions := []runtimemodel.DoctorPolicySuggestion{{
		ID: "lane_pressure", Title: "Lane pressure", Severity: "medium",
		Reason:   "2 lane-limit marker(s) on lane=compact; apply only if host/network headroom exists",
		Markers:  []string{"runtime_proxy_lane_limit_reached"},
		Settings: []runtimemodel.DoctorPolicySettingSuggestion{{Section: "runtime_proxy", Key: "compact_active_limit", CurrentValue: 1, SuggestedValue: 12}},
		Snippet:  "[runtime_proxy]\ncompact_active_limit = 12",
	}}
	report := doctorFixture()
	report.Runtime.PolicySuggestionCount = &count
	report.Runtime.PolicySuggestions = &suggestions
	runner := &fakeDoctorRunner{report: report}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--runtime", "--suggest-policy"}); err != nil {
		t.Fatal(err)
	}
	if !runner.options.SuggestPolicy || !strings.Contains(output.String(), "Runtime Policy Suggestions") ||
		!strings.Contains(output.String(), "- Lane pressure: 2 lane-limit marker(s)") ||
		!strings.Contains(output.String(), "  [runtime_proxy]") || !strings.Contains(output.String(), "  compact_active_limit = 12") {
		t.Fatalf("policy output = %q options=%+v", output.String(), runner.options)
	}

	output.Reset()
	runner = &fakeDoctorRunner{report: report}
	if err := Doctor(context.Background(), runner, &output, []string{"--runtime", "--suggest-policy", "--json"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"policy_suggestion_count": 1`) || !strings.Contains(output.String(), `"id": "lane_pressure"`) {
		t.Fatalf("policy json = %q", output.String())
	}

	output.Reset()
	runner = &fakeDoctorRunner{report: report}
	if err := Doctor(context.Background(), runner, &output, []string{"--runtime", "--suggest-policy", "--bundle", "-", "--redacted"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"policy_suggestion_count": 1`) || !strings.Contains(output.String(), `"policy_suggestions": [`) || !strings.Contains(output.String(), `"lane_pressure"`) {
		t.Fatalf("policy bundle = %q", output.String())
	}
}

func TestProdex04355DoctorRendersEmptyPolicySuggestionResult(t *testing.T) {
	count := 0
	suggestions := []runtimemodel.DoctorPolicySuggestion{}
	report := doctorFixture()
	report.Runtime.PolicySuggestionCount = &count
	report.Runtime.PolicySuggestions = &suggestions
	var output strings.Builder
	if err := Doctor(context.Background(), &fakeDoctorRunner{report: report}, &output, []string{"--runtime", "--suggest-policy"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), noDoctorPolicySuggestion) {
		t.Fatalf("empty suggestion output = %q", output.String())
	}
}

func TestDoctorPropagatesDiagnosticError(t *testing.T) {
	want := errors.New("synthetic doctor failure")
	err := Doctor(context.Background(), &fakeDoctorRunner{err: want}, &strings.Builder{}, nil)
	if !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestDoctorTUIViewContainsPanels(t *testing.T) {
	view := (doctorTUIModel{panels: []doctorPanel{{title: "Doctor", fields: [][2]string{{"Runtime", "ready"}}}}}).View()
	for _, expected := range []string{"Godex Doctor", "1 panel(s)", "Doctor", "Runtime", "ready"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("view missing %q: %q", expected, view)
		}
	}
}

func TestDoctorQuotaPreservesExternalProviderDetails(t *testing.T) {
	quota := runtimemodel.DoctorQuota{
		Profile: "work", Provider: "openai", State: "configured",
		External: &runtimemodel.DoctorExternalQuota{Status: "Configured", Main: "quota handled by provider/Codex", Reset: "monthly"},
	}
	var human strings.Builder
	if err := writeDoctorPanels(&human, []doctorPanel{doctorQuotaPanel(quota)}); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Quota: Configured", "Main: quota handled by provider/Codex", "Reset: monthly"} {
		if !strings.Contains(human.String(), expected) {
			t.Fatalf("human quota output missing %q: %q", expected, human.String())
		}
	}

	report := doctorFixture()
	report.Quota = []runtimemodel.DoctorQuota{quota}
	runner := &fakeDoctorRunner{report: report}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--runtime", "--quota", "--json"}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	rows := document["quota_probes"].([]any)
	probe := rows[0].(map[string]any)["quota"].(map[string]any)
	if probe["status"] != "Configured" || probe["main"] != quota.External.Main || probe["reset"] != "monthly" {
		t.Fatalf("JSON quota = %#v", rows[0])
	}
}

func doctorFixture() runtimemodel.DoctorDiagnostics {
	return runtimemodel.DoctorDiagnostics{
		GeneratedAt: "2026-10-01T00:00:00Z", GodexHome: "/godex", CodexVersion: "codex-cli synthetic",
		AccountCount: 2, EnabledCount: 1,
		Install: []runtimemodel.DoctorCheck{{Name: "Codex CLI", Status: "ok"}},
		Runtime: &runtimemodel.DoctorRuntime{Overview: runtimemodel.Overview{ProfileCount: 2, ActiveProfile: "work"}, TailBytes: 4096},
		Quota:   []runtimemodel.DoctorQuota{{Profile: "work", Provider: "openai", Auth: "chatgpt", State: "ready", Active: true, Enabled: true}},
	}
}
