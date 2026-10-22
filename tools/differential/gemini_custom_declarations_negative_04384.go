package main

import (
	"encoding/json"
	"strings"
)

func geminiCustomDeclarationNegativeControl04384() bool {
	for _, suffix := range geminiCustomDeclarationSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		for _, mutation := range [][2]string{
			{"synthetic_custom", "other_custom"},
			{"*** Begin Patch", "*** Wrong Patch"},
			{"Exact raw input for the custom/freeform tool.", "Wrong input guidance."},
			{"Original custom tool format JSON", "Wrong format label"},
			{"1000.0", "1000"},
			{`"additionalProperties":false`, `"additionalProperties":true`},
			{`\n\n`, `\n`},
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
