package main

import (
	"encoding/json"
	"strings"
)

func geminiNamespaceDeclarationNegativeControl04384() bool {
	for _, suffix := range geminiNamespaceDeclarationSuffixes04384() {
		scenario := "request-primitive-" + suffix
		fixture, _ := requestPrimitiveCase04384(scenario)
		for _, mutation := range [][2]string{
			{"fs--read", "fs__read"},
			{"mcp__fs__read", "mcp__fs--read"},
			{"mcp__fs--group__read", "mcp__fs__group__read"},
			{" synthetic description ", "synthetic description"},
			{`"tools"`, `"wrong_tools"`},
			{"tools[0]", "tools[1]"},
			{"is not a supported tool declaration", "must be an object"},
		} {
			run := geminiPrimitiveExpected04384(scenario)
			if len(geminiCompare(scenario, run, run)) != 0 {
				return false
			}
			wire := ""
			if !fixture.reject {
				wire = run.Upstream[0].RawBody
			}
			changed := strings.ReplaceAll(wire, mutation[0], mutation[1])
			body := strings.ReplaceAll(run.Client.Body, mutation[0], mutation[1])
			if changed == wire && body == run.Client.Body {
				continue
			}
			if !fixture.reject {
				run.Upstream[0].RawBody, run.Upstream[0].Body = changed, nil
				_ = json.Unmarshal([]byte(changed), &run.Upstream[0].Body)
			}
			run.Client.Body, run.Stdout = body, body+"\n"
			if len(geminiCompare(scenario, run, run)) == 0 {
				return false
			}
		}
	}
	return true
}
