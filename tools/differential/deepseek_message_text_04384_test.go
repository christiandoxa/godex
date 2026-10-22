package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekMessageTextRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekMessageTextSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			if !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) {
				t.Fatal("message text fixture is absent from the default registry")
			}
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("message text fixture lost valid original input or golden wire")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"model":`, `"forged_model":`, 1)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching message text corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("message text fixtures weakened primitive wire/state assertions")
	}
}
