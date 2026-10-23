package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekSchemaAliasesRejectMatchingCorruption04390(t *testing.T) {
	for _, suffix := range deepSeekSchemaAliasSuffixes04390() {
		t.Run(suffix, func(t *testing.T) {
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("schema alias fixture lost registry or JSON contract")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			if fixture.reject {
				bad.Client.Status = 200
			} else {
				bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, "lookup", "wrong", 1)
			}
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching schema alias corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("schema alias fixtures weakened original guards")
	}
}
