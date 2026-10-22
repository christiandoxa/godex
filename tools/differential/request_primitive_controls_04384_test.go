package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProdex04384RequestPrimitiveOracleFixtures(t *testing.T) {
	for _, suffix := range append(requestPrimitiveSuffixes04384(), deepSeekAdmissionSuffixes04384()...) {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if (fixture.body == "" || !fixture.reject) && !json.Valid(requestPrimitiveInput04384(scenario, "gemini-3.1-pro-preview")) {
			t.Fatal("primitive fixture lost its original JSON")
		}
		gemini := geminiPrimitiveExpected04384(scenario)
		if len(geminiCompare(scenario, gemini, gemini)) != 0 {
			t.Fatalf("valid Gemini primitive fixture failed: %s", scenario)
		}
		deepseek := deepSeekPrimitiveExpected04384(scenario)
		if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: "deepseek-" + scenario, Runs: []productRun{deepseek, deepseek}})) != 0 {
			t.Fatalf("valid DeepSeek primitive fixture failed: %s", scenario)
		}
	}
	for _, name := range []string{"all", "output", "request-primitive-unknown"} {
		if _, known := requestPrimitiveCase04384(name); known {
			t.Fatal("unknown primitive scenario was accepted")
		}
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("matching primitive corruptions escaped independent invariants")
	}
}

func TestProdex04384RawRequestFixturesPreserveRejectedJSONBytes(t *testing.T) {
	for _, fixture := range []struct{ suffix, raw string }{
		{"syntax", `{"input":`},
		{"surrogate", `"unused":"\ud800"`},
		{"utf8", string([]byte{0xff})},
		{"trailing", `} {}`},
		{"overflow", `1e309`},
	} {
		body := requestPrimitiveInput04384("deepseek-request-primitive-json-"+fixture.suffix, "deepseek-v4-pro")
		if !strings.Contains(string(body), fixture.raw) {
			t.Fatalf("raw JSON fixture was repaired before admission: %s", fixture.suffix)
		}
	}
}
