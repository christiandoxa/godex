package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestDoctorExternalQuotaUsesProdexHumanFields(t *testing.T) {
	quota := runtimemodel.DoctorQuota{
		Profile: "work", Provider: "openai", Auth: "api-key", State: "configured",
		External: &runtimemodel.DoctorExternalQuota{Status: "Configured", Main: "quota handled by provider/Codex", Reset: "monthly"},
	}
	var output strings.Builder
	if err := writeDoctorPanels(&output, []doctorPanel{doctorQuotaPanel(quota)}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"Quota: Configured", "Main: quota handled by provider/Codex", "Reset: monthly"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("doctor quota output missing %q: %q", expected, text)
		}
	}
	if strings.Contains(text, "Status:") {
		t.Fatalf("doctor quota emitted non-reference Status field: %q", text)
	}
}

func TestDoctorExternalQuotaUsesProdexJSONShape(t *testing.T) {
	report := runtimemodel.DoctorDiagnostics{Quota: []runtimemodel.DoctorQuota{{
		Profile: "work", Provider: "openai", Auth: "api-key", State: "configured",
		External: &runtimemodel.DoctorExternalQuota{Status: "Configured", Main: "quota handled by provider/Codex"},
	}}}
	var output strings.Builder
	if err := writeDoctorJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	rows := document["quota_probes"].([]any)
	row := rows[0].(map[string]any)
	if len(row) != 3 || row["profile"] != "work" || row["provider"] != "openai" {
		t.Fatalf("external quota row = %#v", row)
	}
	quota, ok := row["quota"].(map[string]any)
	if !ok || quota["status"] != "Configured" || quota["main"] != "quota handled by provider/Codex" {
		t.Fatalf("external quota JSON = %#v", row["quota"])
	}
	if reset, exists := quota["reset"]; !exists || reset != nil {
		t.Fatalf("external quota reset = %#v, exists=%t", reset, exists)
	}
}
