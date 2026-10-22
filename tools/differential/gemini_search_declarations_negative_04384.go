package main

import (
	"encoding/json"
	"strings"
)

func geminiSearchDeclarationNegativeControl04384() bool {
	for _, suffix := range geminiSearchDeclarationSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		for _, mutation := range [][2]string{
			{"tool_search", "client_supplied"},
			{`"tools"`, `"wrong_tools"`},
			{`"type":"function"`, `"type":"tool_search"`},
			{`"additionalProperties":false`, `"additionalProperties":true`},
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
