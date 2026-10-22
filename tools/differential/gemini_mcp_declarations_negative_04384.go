package main

import (
	"encoding/json"
	"strings"
)

func geminiMCPDeclarationNegativeControl04384() bool {
	for _, suffix := range geminiMCPDeclarationSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		for _, mutation := range [][2]string{
			{"mcp__fs__read", "mcp__other__read"},
			{"git_tools", "gittools"},
			{"MCP tool read from fs.", "wrong description"},
			{`"additionalProperties":true`, `"additionalProperties":false`},
			{`"tools"`, `"wrong_tools"`},
			{"tools[0]", "tools[1]"},
			{"is not a supported tool declaration", "must be a string"},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			if len(geminiCompare(scenario, run, run)) != 0 {
				return false
			}
			wire := ""
			if !fixture.reject {
				wire = run.Upstream[0].RawBody
			}
			changedWire := strings.ReplaceAll(wire, mutation[0], mutation[1])
			body := strings.ReplaceAll(run.Client.Body, mutation[0], mutation[1])
			if changedWire == wire && body == run.Client.Body {
				continue
			}
			if !fixture.reject {
				run.Upstream[0].RawBody, run.Upstream[0].Body = changedWire, nil
				_ = json.Unmarshal([]byte(changedWire), &run.Upstream[0].Body)
			}
			run.Client.Body, run.Stdout = body, body+"\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
