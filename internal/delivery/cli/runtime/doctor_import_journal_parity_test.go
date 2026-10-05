package runtime

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

type importJournalDoctorRunner struct {
	report runtimemodel.DoctorDiagnostics
	opts   runtimemodel.DoctorOptions
	saved  []byte
}

func (runner *importJournalDoctorRunner) Diagnose(_ context.Context, options runtimemodel.DoctorOptions) (runtimemodel.DoctorDiagnostics, error) {
	runner.opts = options
	return runner.report, nil
}

func (*importJournalDoctorRunner) RepairSessionIndex(context.Context) error { return nil }

func (runner *importJournalDoctorRunner) SaveBundle(_ string, content []byte) (string, error) {
	runner.saved = append([]byte(nil), content...)
	return "/tmp/doctor.json", nil
}

func TestDoctorImportJournalRepairUsesProdexHumanStatus(t *testing.T) {
	repaired := 2
	runner := &importJournalDoctorRunner{report: runtimemodel.DoctorDiagnostics{
		GodexHome: "/godex", CodexVersion: "codex 0.160.0",
		ImportAuthJournals: &runtimemodel.DoctorImportAuthJournals{OrphanCount: 0, RepairPerformed: true, Repaired: repaired, Status: "ok"},
	}}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--repair-import-auth-journals"}); err != nil {
		t.Fatal(err)
	}
	if !runner.opts.RepairImportAuthJournals {
		t.Fatal("repair option was not forwarded")
	}
	if !strings.Contains(output.String(), "Import auth journals: Repaired 2 orphan journal(s).") {
		t.Fatalf("doctor output = %q", output.String())
	}
}

func TestDoctorImportJournalRepairUsesProdexJSONShape(t *testing.T) {
	repaired := 1
	runner := &importJournalDoctorRunner{report: runtimemodel.DoctorDiagnostics{
		GeneratedAt: "2026-10-03T00:00:00Z", GodexHome: "/godex", CodexVersion: "codex 0.160.0",
		ImportAuthJournals: &runtimemodel.DoctorImportAuthJournals{OrphanCount: 0, RepairPerformed: true, Repaired: repaired, Status: "ok"},
	}}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{"--repair-import-auth-journals", "--runtime", "--json"}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	if _, exists := document["repaired_import_auth_journals"]; exists {
		t.Fatalf("non-reference repair field leaked: %#v", document)
	}
	status, ok := document["import_auth_journals"].(map[string]any)
	if !ok || status["orphan_count"] != float64(0) || status["repair_performed"] != true ||
		status["repaired"] != float64(1) || status["status"] != "ok" {
		t.Fatalf("import journal JSON = %#v", document["import_auth_journals"])
	}
}

func TestDoctorBundleNestsImportJournalStatusUnderConfig(t *testing.T) {
	repaired := 1
	runner := &importJournalDoctorRunner{report: runtimemodel.DoctorDiagnostics{
		GeneratedAt: "2026-10-03T00:00:00Z", GodexHome: "/godex", CodexVersion: "codex 0.160.0",
		ImportAuthJournals: &runtimemodel.DoctorImportAuthJournals{OrphanCount: 0, RepairPerformed: true, Repaired: repaired, Status: "ok"},
	}}
	var output strings.Builder
	if err := Doctor(context.Background(), runner, &output, []string{
		"--repair-import-auth-journals", "--bundle", "-", "--redacted",
	}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	config, ok := document["config"].(map[string]any)
	if !ok {
		t.Fatalf("bundle config = %#v", document["config"])
	}
	status, ok := config["import_auth_journals"].(map[string]any)
	if !ok || status["repaired"] != float64(1) || status["status"] != "ok" {
		t.Fatalf("bundle import journal status = %#v", config["import_auth_journals"])
	}
	if _, exists := document["repaired_import_auth_journals"]; exists {
		t.Fatalf("bundle leaked non-reference top-level repair field: %#v", document)
	}
	if _, err := io.WriteString(io.Discard, output.String()); err != nil {
		t.Fatal(err)
	}
}
