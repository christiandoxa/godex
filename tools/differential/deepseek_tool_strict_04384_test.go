package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekToolStrictRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekToolStrictSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("tool strict fixture lost registry or JSON contract")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			if fixture.reject {
				bad.Client.Status = 200
			} else if suffix == "tool-strict-absent" {
				bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"parameters":`, `"strict":false,"parameters":`, 1)
			} else {
				bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"strict":false`, `"strict":true`, 1)
			}
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching strict-flag corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("tool strict fixtures weakened original assertions")
	}
}
