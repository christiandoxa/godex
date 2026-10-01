package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type fakeDoctorRunner struct {
	options      runtimemodel.DoctorOptions
	report       runtimemodel.DoctorDiagnostics
	err          error
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

func TestDoctorRejectsUnimplementedRepairAndPolicyActions(t *testing.T) {
	for _, arguments := range [][]string{
		{"--repair-import-auth-journals"},
		{"--repair-session-index"},
		{"--runtime", "--suggest-policy"},
	} {
		err := Doctor(context.Background(), &fakeDoctorRunner{}, &strings.Builder{}, arguments)
		if err == nil || !strings.Contains(err.Error(), "not available") {
			t.Fatalf("arguments %v error=%v", arguments, err)
		}
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

func doctorFixture() runtimemodel.DoctorDiagnostics {
	return runtimemodel.DoctorDiagnostics{
		GeneratedAt: "2026-10-01T00:00:00Z", GodexHome: "/godex", CodexVersion: "codex-cli synthetic",
		AccountCount: 2, EnabledCount: 1,
		Install: []runtimemodel.DoctorCheck{{Name: "Codex CLI", Status: "ok"}},
		Runtime: &runtimemodel.DoctorRuntime{Overview: runtimemodel.Overview{ProfileCount: 2, ActiveProfile: "work"}, TailBytes: 4096},
		Quota:   []runtimemodel.DoctorQuota{{Profile: "work", Provider: "openai", Auth: "chatgpt", State: "ready", Active: true, Enabled: true}},
	}
}
