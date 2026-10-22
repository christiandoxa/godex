package main

import (
	"encoding/json"
	"testing"
)

func TestProdex04384RequestPrimitiveOracleFixtures(t *testing.T) {
	for _, suffix := range append(requestPrimitiveSuffixes04384(), deepSeekAdmissionSuffixes04384()...) {
		scenario := "request-primitive-" + suffix
		if !json.Valid(requestPrimitiveInput04384(scenario, "gemini-3.1-pro-preview")) {
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
