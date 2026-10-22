package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeepSeekToolIDRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekToolIDSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("tool ID fixture lost complete input or exact upstream JSON")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			body := bad.Upstream[0].Body
			if strings.Contains(body, `"id":`) {
				body = strings.ReplaceAll(body, `"id":`, `"forged_id":`)
			} else {
				body = strings.Replace(body, `"messages":[`, `"messages":[{"role":"tool","tool_call_id":"forged","content":"unexpected"},`, 1)
			}
			bad.Upstream[0].Body = body
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}}); len(failures) != 2 {
				t.Fatal("accepted matching tool identity corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("primitive controls accepted matching wire/state mutations")
	}
}
