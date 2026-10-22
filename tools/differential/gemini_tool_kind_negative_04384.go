package main

import (
	"encoding/json"
	"strings"
)

func geminiToolKindNegativeControl04384() bool {
	for _, suffix := range geminiToolKindSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		if fixture.reject {
			continue
		}
		run := geminiPrimitiveExpected04384(scenario)
		if len(geminiCompare(scenario, run, run)) != 0 {
			return false
		}
		run.Upstream[0].Body["tools"] = []any{}
		wire, err := json.Marshal(run.Upstream[0].Body)
		if err != nil {
			return false
		}
		run.Upstream[0].RawBody = string(wire)
		if len(geminiCompare(scenario, run, run)) == 0 {
			return false
		}
		run = geminiPrimitiveExpected04384(scenario)
		wire = []byte(strings.ReplaceAll(run.Upstream[0].RawBody, "mcp__fs__lookup", "wrong_alias"))
		if string(wire) != run.Upstream[0].RawBody {
			run.Upstream[0].RawBody, run.Upstream[0].Body = string(wire), nil
			_ = json.Unmarshal(wire, &run.Upstream[0].Body)
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
