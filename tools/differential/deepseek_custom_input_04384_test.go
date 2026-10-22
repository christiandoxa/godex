package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestDeepSeekCustomInputRejectsMatchingCorruption04384(t *testing.T) {
	for _, suffix := range deepSeekCustomInputSuffixes04384() {
		t.Run(suffix, func(t *testing.T) {
			if !slices.Contains(deepSeekAdmissionSuffixes04384(), suffix) {
				t.Fatal("custom input fixture is absent from the default registry")
			}
			scenario := "deepseek-request-primitive-" + suffix
			fixture, known := requestPrimitiveCase04384(scenario)
			if !known || !json.Valid([]byte(fixture.body)) || !json.Valid([]byte(fixture.wireBody)) {
				t.Fatal("custom input fixture lost valid original input or golden wire")
			}
			var wire struct {
				Messages []struct {
					ToolCalls []struct{ Function struct{ Arguments string } } `json:"tool_calls"`
				}
			}
			if err := json.Unmarshal([]byte(fixture.wireBody), &wire); err != nil {
				t.Fatal(err)
			}
			calls := 0
			for _, message := range wire.Messages {
				for _, call := range message.ToolCalls {
					var wrapper map[string]any
					if err := json.Unmarshal([]byte(call.Function.Arguments), &wrapper); err != nil {
						t.Fatal(err)
					}
					if _, ok := wrapper["input"].(string); !ok || len(wrapper) != 1 {
						t.Fatal("custom input golden lost its exact string wrapper")
					}
					calls++
				}
			}
			if calls != 1 {
				t.Fatal("custom input golden lost its single call")
			}
			good := deepSeekPrimitiveExpected04384(scenario)
			if failures := deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{good, good}}); len(failures) != 0 {
				t.Fatal(failures)
			}
			bad := deepSeekPrimitiveExpected04384(scenario)
			bad.Upstream[0].Body = strings.Replace(bad.Upstream[0].Body, `"arguments":`, `"forged_arguments":`, 1)
			if len(deepSeekPrimitiveInvariants04384(scenarioResult{Name: scenario, Runs: []productRun{bad, bad}})) != 2 {
				t.Fatal("accepted matching custom input corruption")
			}
		})
	}
	if !requestPrimitiveNegativeControl04384() {
		t.Fatal("custom input fixtures weakened primitive wire/state assertions")
	}
}
