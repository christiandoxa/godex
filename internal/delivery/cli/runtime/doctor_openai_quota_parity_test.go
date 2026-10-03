package runtime

import (
	"encoding/json"
	"strings"
	"testing"

	runtimemodel "github.com/christiandoxa/godex/internal/model/runtime"
)

func TestDoctorOpenAIQuotaUsesProdexHumanAndJSONShape(t *testing.T) {
	quota := runtimemodel.DoctorQuota{
		Profile: "work", Provider: "openai", Auth: "chatgpt", State: "ready", Active: true, Enabled: true,
		OpenAI: &runtimemodel.DoctorOpenAIQuota{
			Status: "ready", HumanStatus: "Ready",
			Main: "5h: 80% left (20% used), resets - | weekly: 60% left (40% used), resets -",
		},
	}

	var human strings.Builder
	if err := writeDoctorPanels(&human, []doctorPanel{doctorQuotaPanel(quota)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human.String(), "Quota: Ready") ||
		!strings.Contains(human.String(), "Main: 5h: 80% left (20% used), resets - | weekly: 60% left (40% used), resets -") {
		t.Fatalf("OpenAI human quota = %q", human.String())
	}
	for _, forbidden := range []string{"Plan:", "Weekly:"} {
		if strings.Contains(human.String(), forbidden) {
			t.Fatalf("OpenAI human quota retained legacy field %q: %q", forbidden, human.String())
		}
	}

	report := runtimemodel.DoctorDiagnostics{Quota: []runtimemodel.DoctorQuota{quota}}
	var output strings.Builder
	if err := writeDoctorJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	row := document["quota_probes"].([]any)[0].(map[string]any)
	if len(row) != 3 || row["profile"] != "work" || row["provider"] != "openai" {
		t.Fatalf("OpenAI quota row = %#v", row)
	}
	probe, ok := row["quota"].(map[string]any)
	if !ok || probe["status"] != "ready" ||
		probe["main"] != "5h: 80% left (20% used), resets - | weekly: 60% left (40% used), resets -" {
		t.Fatalf("OpenAI quota JSON = %#v", row["quota"])
	}
}

func TestDoctorQuotaErrorUsesProdexHumanAndJSONShape(t *testing.T) {
	quota := runtimemodel.DoctorQuota{
		Profile: "work", Provider: "openai", Auth: "chatgpt", State: "error", Active: true, Enabled: true,
		Error: "Error (quota endpoint unavailable)",
	}
	var human strings.Builder
	if err := writeDoctorPanels(&human, []doctorPanel{doctorQuotaPanel(quota)}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(human.String(), "Quota: Error (quota endpoint unavailable)") || strings.Contains(human.String(), "Plan:") {
		t.Fatalf("quota error human = %q", human.String())
	}

	var output strings.Builder
	if err := writeDoctorJSON(&output, runtimemodel.DoctorDiagnostics{Quota: []runtimemodel.DoctorQuota{quota}}); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
		t.Fatal(err)
	}
	row := document["quota_probes"].([]any)[0].(map[string]any)
	probe := row["quota"].(map[string]any)
	if len(row) != 3 || len(probe) != 1 || probe["error"] != "Error (quota endpoint unavailable)" {
		t.Fatalf("quota error JSON = %#v", row)
	}
}
