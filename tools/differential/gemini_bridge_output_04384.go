package main

import "strings"

func geminiBridgeOutputSuffixes04384() []string {
	var suffixes []string
	for _, kind := range []string{"custom", "result", "mcp"} {
		for _, variant := range []string{"control", "null-id", "content-output", "result-output", "output-text", "null", "false", "empty-array", "object"} {
			suffixes = append(suffixes, "bridge-output-"+kind+"-"+variant)
		}
	}
	return suffixes
}

func geminiBridgeOutputFixture04384(suffix string) (requestPrimitiveFixture04384, bool) {
	kind, variant, _ := strings.Cut(strings.TrimPrefix(suffix, "bridge-output-"), "-")
	switch kind {
	case "custom":
		kind = "custom_tool_call_output"
	case "result":
		kind = "mcp_tool_result"
	case "mcp":
		kind = "mcp_call_output"
	default:
		return requestPrimitiveFixture04384{}, false
	}
	alias, output, wire := variant, "", ""
	switch variant {
	case "null":
		alias, output, wire = "control", "null", "null"
	case "false":
		alias, output, wire = "control", "false", "false"
	case "empty-array":
		alias, output = "control", "[]"
	case "object":
		alias, output, wire = "control", `{"b":1.00,"a":"<"}`, `{\"a\":\"<\",\"b\":1.0}`
	}
	fixture, known := geminiFunctionAliasFixture04384("function-alias-" + alias)
	fixture.body = strings.Replace(fixture.body, `"type":"function_call_output"`, `"type":"`+kind+`"`, 1)
	if output != "" {
		fixture.body = strings.Replace(fixture.body, `"output":"synthetic-result"`, `"output":`+output, 1)
		fixture.wireBody = strings.Replace(fixture.wireBody, `"content":"synthetic-result"`, `"content":"`+wire+`"`, 1)
	}
	return fixture, known
}
