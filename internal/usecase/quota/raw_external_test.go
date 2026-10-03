package quota

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	geminigateway "github.com/christiandoxa/godex/internal/gateway/gemini"
	profilemodel "github.com/christiandoxa/godex/internal/model/profile"
	quotamodel "github.com/christiandoxa/godex/internal/model/quota"
)

type rawExternalFixture struct {
	info quotamodel.ExternalInfo
	raw  []byte
}

func (fixture rawExternalFixture) FetchQuota(context.Context, profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	return fixture.info, nil
}

func (fixture rawExternalFixture) FetchQuotaRaw(context.Context, profilemodel.QuotaTarget) ([]byte, error) {
	return append([]byte(nil), fixture.raw...), nil
}

type snapshotExternalFixture struct{ info quotamodel.ExternalInfo }

func (fixture snapshotExternalFixture) FetchQuota(context.Context, profilemodel.QuotaTarget) (quotamodel.ExternalInfo, error) {
	return fixture.info, nil
}

func TestStatusRawUsesProviderRawOverrideBeforeSnapshotSerialization(t *testing.T) {
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "copilot", Provider: "copilot", Enabled: true,
	}}})
	status.SetExternalProvider("copilot", rawExternalFixture{
		info: quotamodel.ExternalInfo{Provider: "formatted snapshot", Status: "Ready", Main: "-"},
		raw:  []byte(`{"login":"raw-login","unknown":{"big":9007199254740993}}`),
	})

	body, err := status.Raw(context.Background(), "copilot", "")
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		t.Fatal(err)
	}
	if value["login"] != "raw-login" || value["provider"] != nil ||
		value["unknown"].(map[string]any)["big"].(json.Number).String() != "9007199254740993" {
		t.Fatalf("raw provider body = %#v", value)
	}
}

func TestStatusRawSerializesExternalSnapshotLikeProdex(t *testing.T) {
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "kiro", Provider: "kiro", Enabled: true,
	}}})
	available := true
	status.SetExternalProvider("kiro", snapshotExternalFixture{info: quotamodel.ExternalInfo{
		Provider: "Kiro CLI", Status: "Ready (imported)", Main: "catalog unavailable",
		Available: &available, Details: []quotamodel.ExternalDetail{{Label: "Models", Value: "catalog unavailable"}},
	}})

	body, err := status.Raw(context.Background(), "kiro", "")
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"provider":"Kiro CLI","account":null,"plan":null,"status":"Ready (imported)","main":"catalog unavailable","reset":null,"available":true,"details":[{"label":"Models","value":"catalog unavailable"}]}`
	if string(body) != want {
		t.Fatalf("external raw = %s, want %s", body, want)
	}
}

func TestStatusRawExternalJSONDoesNotHTMLEscapeProdexValues(t *testing.T) {
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{Name: "kiro-html", Provider: "kiro", Enabled: true}}})
	status.SetExternalProvider("kiro", snapshotExternalFixture{info: quotamodel.ExternalInfo{
		Provider: "Kiro <CLI>", Status: "Ready", Main: "a&b",
		Details: []quotamodel.ExternalDetail{{Label: "Model <group>", Value: "a&b"}},
	}})
	body, err := status.Raw(context.Background(), "kiro-html", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `\\u003c`) || strings.Contains(string(body), `\\u0026`) ||
		!strings.Contains(string(body), `"provider":"Kiro <CLI>"`) || !strings.Contains(string(body), `"main":"a&b"`) {
		t.Fatalf("external raw HTML escaping = %s", body)
	}
}

func TestStatusRawGeminiProfilePreservesDisabledOAuthGuidance(t *testing.T) {
	status := NewStatus(fakeAccounts{}, &trackingUsage{})
	status.SetProfiles(fakeProfileSource{targets: []profilemodel.QuotaTarget{{
		Name: "legacy-gemini", Provider: "gemini", Enabled: true,
	}}})
	status.SetExternalProvider("gemini", geminigateway.ProfileQuota{})
	_, err := status.Raw(context.Background(), "legacy-gemini", "https://ignored.example.test")
	if err == nil || !strings.Contains(err.Error(), "unsupported and disabled") ||
		!strings.Contains(err.Error(), "Gemini API key") || !strings.Contains(err.Error(), "Vertex AI") {
		t.Fatalf("Gemini raw quota error = %v", err)
	}
}
