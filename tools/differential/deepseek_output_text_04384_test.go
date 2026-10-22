package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekOutputTextRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekOutputTextSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			if !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) {
				t.Fatal("output text fixture is absent from the default registry")
			}
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("output text fixture lost valid original input or golden wire")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"role":"tool"`, `"role":"forged"`, 1)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching tool output corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("output text fixtures weakened primitive wire/state assertions")
	}
}
