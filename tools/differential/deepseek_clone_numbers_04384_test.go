package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekCloneNumbersRejectMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekCloneNumberSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			if !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) {
				t.Fatal("clone-number fixture is absent from the default registry")
			}
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("clone-number fixture lost valid input or golden wire")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			switch {
			case fixture.reject:
				bad.Client.Status = 200
			case fixture.deepSeekMetadata != "":
				bad.Client.Body = strings.Replace(bad.Client.Body, `"value":`, `"wrong":`, 1)
			default:
				bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"enum":`, `"wrong":`, 1)
			}
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching clone-number corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("clone-number fixtures weakened primitive request/response/state guards")
	}
}
