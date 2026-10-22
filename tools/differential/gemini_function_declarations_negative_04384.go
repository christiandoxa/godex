package main

import (
	"encoding/json"
	"strings"
)

func geminiFunctionDeclarationNegativeControl04384() bool {
	for _, suffix := range geminiFunctionDeclarationSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		for _, mutation := range [][2]string{
			{"synthetic_lookup", "different_lookup"},
			{`"tools"`, `"wrong_tools"`},
			{`"strict":false`, `"strict":true`},
			{"-0.0", "0.0"},
			{`"additionalProperties":true`, `"additionalProperties":false`},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			if len(geminiCompare(scenario, run, run)) != 0 {
				return false
			}
			wire := strings.ReplaceAll(run.Upstream[0].RawBody, mutation[0], mutation[1])
			if wire == run.Upstream[0].RawBody {
				continue
			}
			run.Upstream[0].RawBody, run.Upstream[0].Body = wire, nil
			_ = json.Unmarshal([]byte(wire), &run.Upstream[0].Body)
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
