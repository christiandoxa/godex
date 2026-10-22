package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDeepSeekPreviousResponseIDRejectsMatchingCorruption04384(t *testing.T) {
	for _, scenario := range deepSeekPreviousResponseCases04384() {
		t.Run(scenario, func(t *testing.T) {
			seed, previous, known := deepSeekPreviousResponseIDs04384(scenario)
			if !known || !deepSeekHistoryCase04384(scenario) {
				t.Fatal("previous ID fixture is absent from the default registry")
			}
			var input map[string]any
			if err := json.Unmarshal(deepSeekHistoryInput04384(scenario, 1), &input); err != nil {
				t.Fatal(err)
			}
			if input["previous_response_id"] != previous || !strings.Contains(deepSeekHistorySeed04384(scenario), `"id":"`+seed+`"`) {
				t.Fatal("previous ID fixture changed original request or response identity")
			}
			good := deepSeekHistoryExpected04384(scenario)
			if failures := deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekHistoryExpected04384(scenario)
			bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"content":"history request"`, `"content":"forged"`, 1)
			if len(deepSeekHistoryInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching previous ID history corruption")
			}
		})
	}
	if !deepSeekHistoryNegativeControl04384() {
		t.Fatal("previous ID fixtures weakened history/owner/state invariants")
	}
}
